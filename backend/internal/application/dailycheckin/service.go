package dailycheckin

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	domaindailycheckin "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/dailycheckin"
)

type ConfigProvider interface {
	RuntimeValuesByNamespace(ctx context.Context, namespace string) (map[string]string, error)
}

type Repository interface {
	Get(ctx context.Context, userID uint, businessDate time.Time) (*domaindailycheckin.Claim, error)
	Claim(ctx context.Context, input domaindailycheckin.ClaimInput) (domaindailycheckin.Claim, bool, error)
}

type RandomSource interface {
	Intn(upperBound int) (int, error)
}

type cryptoRandom struct{}

func (cryptoRandom) Intn(upperBound int) (int, error) {
	if upperBound <= 0 {
		return 0, errors.New("random upper bound must be positive")
	}
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(upperBound)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

type Status struct {
	Enabled          bool
	BusinessDate     time.Time
	NextAvailableAt  time.Time
	Claimed          bool
	UnitPriceNanousd int64
	Prizes           []domaindailycheckin.Prize
	Claim            *domaindailycheckin.Claim
	StreakDays       int
}

type ClaimResult struct {
	ClaimedNow bool
	Claim      domaindailycheckin.Claim
}

type Service struct {
	repo     Repository
	settings ConfigProvider
	random   RandomSource
	now      func() time.Time
}

func NewService(repo Repository, settings ConfigProvider) *Service {
	return &Service{repo: repo, settings: settings, random: cryptoRandom{}, now: time.Now}
}

func (s *Service) Status(ctx context.Context, userID uint) (Status, error) {
	config, _, err := s.loadConfig(ctx)
	if err != nil {
		return Status{}, err
	}
	now := s.now()
	businessDate, nextAvailableAt, err := resolveBusinessDate(now, config.Timezone)
	if err != nil {
		return Status{}, err
	}
	claim, err := s.repo.Get(ctx, userID, businessDate)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Enabled:          config.Enabled,
		BusinessDate:     businessDate,
		NextAvailableAt:  nextAvailableAt,
		Claimed:          claim != nil,
		UnitPriceNanousd: config.UnitPriceNanousd,
		Prizes:           append([]domaindailycheckin.Prize(nil), config.Prizes...),
		Claim:            claim,
	}
	if claim != nil {
		status.UnitPriceNanousd = claim.UnitPriceNanousd
		status.StreakDays = claim.StreakDays
		if snapshotConfig, snapshotErr := domaindailycheckin.ParseConfig(claim.ConfigSnapshotJSON); snapshotErr == nil {
			status.Prizes = append([]domaindailycheckin.Prize(nil), snapshotConfig.Prizes...)
		}
	}
	return status, nil
}

func (s *Service) Claim(ctx context.Context, userID uint) (ClaimResult, error) {
	if userID == 0 {
		return ClaimResult{}, errors.New("user ID is required")
	}
	config, snapshot, err := s.loadConfig(ctx)
	if err != nil {
		return ClaimResult{}, err
	}
	now := s.now()
	businessDate, _, err := resolveBusinessDate(now, config.Timezone)
	if err != nil {
		return ClaimResult{}, err
	}
	existing, err := s.repo.Get(ctx, userID, businessDate)
	if err != nil {
		return ClaimResult{}, err
	}
	if existing != nil {
		return ClaimResult{ClaimedNow: false, Claim: *existing}, nil
	}
	if !config.Enabled {
		return ClaimResult{}, domaindailycheckin.ErrDisabled
	}
	draw, err := s.random.Intn(domaindailycheckin.WeightBasisPoints)
	if err != nil {
		return ClaimResult{}, fmt.Errorf("select daily check-in prize: %w", err)
	}
	prize, err := selectPrize(config.Prizes, draw)
	if err != nil {
		return ClaimResult{}, err
	}
	rewardNanousd := int64(prize.Calls) * config.UnitPriceNanousd
	claim, claimedNow, err := s.repo.Claim(ctx, domaindailycheckin.ClaimInput{
		UserID:             userID,
		BusinessDate:       businessDate,
		AwardedCalls:       prize.Calls,
		UnitPriceNanousd:   config.UnitPriceNanousd,
		RewardNanousd:      rewardNanousd,
		PrizeKey:           prize.Key,
		ConfigSnapshotJSON: snapshot,
		CreatedAt:          now.UTC(),
	})
	if err != nil {
		return ClaimResult{}, err
	}
	return ClaimResult{ClaimedNow: claimedNow, Claim: claim}, nil
}

func (s *Service) loadConfig(ctx context.Context) (domaindailycheckin.Config, string, error) {
	values, err := s.settings.RuntimeValuesByNamespace(ctx, domaindailycheckin.ConfigNamespace)
	if err != nil {
		return domaindailycheckin.Config{}, "", fmt.Errorf("load daily check-in config: %w", err)
	}
	value := strings.TrimSpace(values[domaindailycheckin.ConfigKey])
	if value == "" {
		return domaindailycheckin.Config{}, "", fmt.Errorf("%w: configuration is missing", domaindailycheckin.ErrInvalidConfig)
	}
	config, err := domaindailycheckin.ParseConfig(value)
	if err != nil {
		return domaindailycheckin.Config{}, "", err
	}
	snapshot, err := domaindailycheckin.CompactConfigJSON(value)
	if err != nil {
		return domaindailycheckin.Config{}, "", err
	}
	return config, snapshot, nil
}

func resolveBusinessDate(now time.Time, timezone string) (time.Time, time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("load daily check-in timezone: %w", err)
	}
	localNow := now.In(location)
	businessDate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
	nextLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, location)
	return businessDate, nextLocal.UTC(), nil
}

func selectPrize(prizes []domaindailycheckin.Prize, draw int) (domaindailycheckin.Prize, error) {
	if draw < 0 || draw >= domaindailycheckin.WeightBasisPoints {
		return domaindailycheckin.Prize{}, errors.New("daily check-in random draw is out of range")
	}
	cumulative := 0
	for _, prize := range prizes {
		cumulative += prize.WeightBps
		if draw < cumulative {
			return prize, nil
		}
	}
	return domaindailycheckin.Prize{}, errors.New("daily check-in prize weights are incomplete")
}
