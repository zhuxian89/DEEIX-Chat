package dailycheckin

import (
	"context"
	"testing"
	"time"

	domaindailycheckin "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/dailycheckin"
)

type fakeConfigProvider struct{ value string }

func (f fakeConfigProvider) RuntimeValuesByNamespace(context.Context, string) (map[string]string, error) {
	return map[string]string{domaindailycheckin.ConfigKey: f.value}, nil
}

type fakeRepository struct{ claim *domaindailycheckin.Claim }

func (f *fakeRepository) Get(context.Context, uint, time.Time) (*domaindailycheckin.Claim, error) {
	return f.claim, nil
}

func (f *fakeRepository) Claim(_ context.Context, input domaindailycheckin.ClaimInput) (domaindailycheckin.Claim, bool, error) {
	result := domaindailycheckin.Claim{
		ID: 1, UserID: input.UserID, BusinessDate: input.BusinessDate,
		AwardedCalls: input.AwardedCalls, UnitPriceNanousd: input.UnitPriceNanousd,
		RewardNanousd: input.RewardNanousd, PrizeKey: input.PrizeKey,
		ConfigSnapshotJSON: input.ConfigSnapshotJSON, StreakDays: 1, CreatedAt: input.CreatedAt,
	}
	f.claim = &result
	return result, true, nil
}

type fixedRandom struct{ value int }

func (f fixedRandom) Intn(int) (int, error) { return f.value, nil }

func TestClaimUsesServerBusinessDateAndConfiguredWeights(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, fakeConfigProvider{value: domaindailycheckin.DefaultConfigJSON()})
	service.now = func() time.Time {
		return time.Date(2026, time.September, 2, 16, 30, 0, 0, time.UTC)
	}
	service.random = fixedRandom{value: 8_500}

	result, err := service.Claim(context.Background(), 9)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if !result.ClaimedNow || result.Claim.AwardedCalls != 50 {
		t.Fatalf("Claim() = %#v", result)
	}
	if got := result.Claim.BusinessDate.Format("2006-01-02"); got != "2026-09-03" {
		t.Fatalf("business date = %s, want 2026-09-03", got)
	}
	if result.Claim.RewardNanousd != 83_500_000 {
		t.Fatalf("reward = %d, want 83500000", result.Claim.RewardNanousd)
	}
}

func TestStatusReturnsOriginalClaim(t *testing.T) {
	date := time.Date(2026, time.September, 3, 0, 0, 0, 0, time.UTC)
	repo := &fakeRepository{claim: &domaindailycheckin.Claim{
		ID: 4, UserID: 9, BusinessDate: date, AwardedCalls: 100,
		UnitPriceNanousd: 1_670_000, RewardNanousd: 167_000_000, PrizeKey: "calls_100", StreakDays: 3,
	}}
	service := NewService(repo, fakeConfigProvider{value: domaindailycheckin.DefaultConfigJSON()})
	service.now = func() time.Time {
		return time.Date(2026, time.September, 3, 9, 0, 0, 0, time.FixedZone("test", 8*60*60))
	}

	status, err := service.Status(context.Background(), 9)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Claimed || status.Claim == nil || status.Claim.ID != 4 || status.StreakDays != 3 {
		t.Fatalf("Status() = %#v", status)
	}
}
