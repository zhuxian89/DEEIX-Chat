package dailycheckin

type DailyCheckinPrizeResponse struct {
	PrizeKey  string `json:"prizeKey"`
	Calls     int    `json:"calls"`
	WeightBps int    `json:"weightBps"`
}

type DailyCheckinStatusResponse struct {
	Enabled         bool                        `json:"enabled"`
	BusinessDate    string                      `json:"businessDate"`
	NextAvailableAt string                      `json:"nextAvailableAt"`
	Claimed         bool                        `json:"claimed"`
	AwardedCalls    int                         `json:"awardedCalls"`
	UnitPriceUsd    float64                     `json:"unitPriceUsd"`
	RewardUsd       float64                     `json:"rewardUsd"`
	PrizeKey        string                      `json:"prizeKey"`
	StreakDays      int                         `json:"streakDays"`
	Prizes          []DailyCheckinPrizeResponse `json:"prizes"`
}

type DailyCheckinClaimResponse struct {
	ClaimedNow     bool    `json:"claimedNow"`
	AlreadyClaimed bool    `json:"alreadyClaimed"`
	BusinessDate   string  `json:"businessDate"`
	AwardedCalls   int     `json:"awardedCalls"`
	UnitPriceUsd   float64 `json:"unitPriceUsd"`
	RewardUsd      float64 `json:"rewardUsd"`
	PrizeKey       string  `json:"prizeKey"`
	StreakDays     int     `json:"streakDays"`
}
