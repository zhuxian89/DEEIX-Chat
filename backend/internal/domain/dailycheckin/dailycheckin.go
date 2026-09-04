package dailycheckin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

const (
	// ConfigNamespace and ConfigKey identify the atomic daily check-in setting.
	ConfigNamespace = "daily_checkin"
	ConfigKey       = "config"

	BalanceRefType      = "daily_checkin_claim"
	WeightBasisPoints   = 10_000
	maxPrizeCount       = 12
	maxCallsPerPrize    = 1_000_000
	maxUnitPriceNanousd = int64(1_000_000_000_000)
	defaultConfigJSON   = `{"enabled":true,"unitPriceUsd":0.00167,"timezone":"Asia/Shanghai","prizes":[{"calls":10,"weightBps":3500},{"calls":20,"weightBps":3000},{"calls":30,"weightBps":2000},{"calls":50,"weightBps":1000},{"calls":100,"weightBps":400},{"calls":200,"weightBps":100}]}`
)

var (
	ErrDisabled       = errors.New("daily check-in is disabled")
	ErrInvalidConfig  = errors.New("invalid daily check-in config")
	decimalUSDPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,9})?$`)
)

// Prize is one configured wheel segment. WeightBps uses integer basis points.
type Prize struct {
	Key       string
	Calls     int
	WeightBps int
}

// Config is the validated runtime configuration.
type Config struct {
	Enabled          bool
	UnitPriceNanousd int64
	Timezone         string
	Prizes           []Prize
}

// Claim is the immutable result of one user's business-day check-in.
type Claim struct {
	ID                   uint
	UserID               uint
	BusinessDate         time.Time
	AwardedCalls         int
	UnitPriceNanousd     int64
	RewardNanousd        int64
	PrizeKey             string
	ConfigSnapshotJSON   string
	StreakDays           int
	BalanceTransactionID uint
	CreatedAt            time.Time
}

// ClaimInput is the server-authoritative award passed into the transaction.
type ClaimInput struct {
	UserID             uint
	BusinessDate       time.Time
	AwardedCalls       int
	UnitPriceNanousd   int64
	RewardNanousd      int64
	PrizeKey           string
	ConfigSnapshotJSON string
	CreatedAt          time.Time
}

// DefaultConfigJSON returns the production-safe initial wheel configuration.
func DefaultConfigJSON() string { return defaultConfigJSON }

// ParseConfig validates and converts the single atomic JSON setting.
func ParseConfig(value string) (Config, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(value)))
	var document map[string]json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return Config{}, fmt.Errorf("%w: malformed JSON: %v", ErrInvalidConfig, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%w: trailing JSON value", ErrInvalidConfig)
	}
	if err := requireExactKeys(document, "enabled", "unitPriceUsd", "timezone", "prizes"); err != nil {
		return Config{}, err
	}
	var enabled bool
	if err := json.Unmarshal(document["enabled"], &enabled); err != nil {
		return Config{}, fmt.Errorf("%w: enabled must be bool", ErrInvalidConfig)
	}
	unitPriceNumber, err := decodeJSONNumber(document["unitPriceUsd"])
	if err != nil {
		return Config{}, fmt.Errorf("%w: unitPriceUsd must be a number", ErrInvalidConfig)
	}
	unitPriceNanousd, err := parseUSDNanousd(unitPriceNumber.String())
	if err != nil || unitPriceNanousd <= 0 || unitPriceNanousd > maxUnitPriceNanousd {
		return Config{}, fmt.Errorf("%w: unitPriceUsd must be greater than 0, have at most 9 decimal places, and not exceed 1000", ErrInvalidConfig)
	}
	var timezoneValue string
	if err := json.Unmarshal(document["timezone"], &timezoneValue); err != nil {
		return Config{}, fmt.Errorf("%w: timezone must be a string", ErrInvalidConfig)
	}
	timezone := strings.TrimSpace(timezoneValue)
	if timezone == "" {
		return Config{}, fmt.Errorf("%w: timezone is required", ErrInvalidConfig)
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return Config{}, fmt.Errorf("%w: timezone is not a valid IANA timezone", ErrInvalidConfig)
	}
	var prizeDocuments []map[string]json.RawMessage
	if err := json.Unmarshal(document["prizes"], &prizeDocuments); err != nil {
		return Config{}, fmt.Errorf("%w: prizes must be an array", ErrInvalidConfig)
	}
	if len(prizeDocuments) < 1 || len(prizeDocuments) > maxPrizeCount {
		return Config{}, fmt.Errorf("%w: prizes must contain between 1 and %d items", ErrInvalidConfig, maxPrizeCount)
	}
	seenCalls := make(map[int]struct{}, len(prizeDocuments))
	prizes := make([]Prize, 0, len(prizeDocuments))
	totalWeight := 0
	for _, item := range prizeDocuments {
		if err := requireExactKeys(item, "calls", "weightBps"); err != nil {
			return Config{}, err
		}
		var calls, weightBps int
		if err := json.Unmarshal(item["calls"], &calls); err != nil {
			return Config{}, fmt.Errorf("%w: prize calls must be an integer", ErrInvalidConfig)
		}
		if err := json.Unmarshal(item["weightBps"], &weightBps); err != nil {
			return Config{}, fmt.Errorf("%w: prize weightBps must be an integer", ErrInvalidConfig)
		}
		if calls <= 0 || calls > maxCallsPerPrize {
			return Config{}, fmt.Errorf("%w: prize calls must be a positive integer no greater than %d", ErrInvalidConfig, maxCallsPerPrize)
		}
		if _, exists := seenCalls[calls]; exists {
			return Config{}, fmt.Errorf("%w: prize calls must not repeat", ErrInvalidConfig)
		}
		if weightBps <= 0 || weightBps > WeightBasisPoints {
			return Config{}, fmt.Errorf("%w: prize weightBps must be between 1 and %d", ErrInvalidConfig, WeightBasisPoints)
		}
		seenCalls[calls] = struct{}{}
		totalWeight += weightBps
		prizes = append(prizes, Prize{
			Key:       fmt.Sprintf("calls_%d", calls),
			Calls:     calls,
			WeightBps: weightBps,
		})
	}
	if totalWeight != WeightBasisPoints {
		return Config{}, fmt.Errorf("%w: prize weightBps must total exactly %d", ErrInvalidConfig, WeightBasisPoints)
	}
	return Config{
		Enabled:          enabled,
		UnitPriceNanousd: unitPriceNanousd,
		Timezone:         timezone,
		Prizes:           prizes,
	}, nil
}

func requireExactKeys(document map[string]json.RawMessage, keys ...string) error {
	if len(document) != len(keys) {
		return fmt.Errorf("%w: configuration contains missing or unknown fields", ErrInvalidConfig)
	}
	for _, key := range keys {
		if _, exists := document[key]; !exists {
			return fmt.Errorf("%w: %s is required", ErrInvalidConfig, key)
		}
	}
	return nil
}

func decodeJSONNumber(raw json.RawMessage) (json.Number, error) {
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return "", err
	}
	return number, nil
}

// CompactConfigJSON validates and removes insignificant whitespace for snapshots.
func CompactConfigJSON(value string) (string, error) {
	if _, err := ParseConfig(value); err != nil {
		return "", err
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return "", err
	}
	compacted, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	return string(compacted), nil
}

// USD converts nanodollars into the API's numeric USD representation.
func USD(nanousd int64) float64 { return float64(nanousd) / 1_000_000_000 }

func parseUSDNanousd(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if !decimalUSDPattern.MatchString(value) {
		return 0, ErrInvalidConfig
	}
	parts := strings.SplitN(value, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > maxUnitPriceNanousd/1_000_000_000 {
		return 0, ErrInvalidConfig
	}
	fraction := "000000000"
	if len(parts) == 2 {
		fraction = parts[1] + strings.Repeat("0", 9-len(parts[1]))
	}
	fractionValue, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, ErrInvalidConfig
	}
	return whole*1_000_000_000 + fractionValue, nil
}
