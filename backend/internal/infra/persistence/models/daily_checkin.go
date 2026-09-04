package model

import "time"

// DailyCheckinClaim is the immutable, server-authored result for one business day.
type DailyCheckinClaim struct {
	ControlPlaneModel
	UserID               uint      `gorm:"not null;uniqueIndex:idx_daily_checkin_user_date,priority:1;index:idx_daily_checkin_user;comment:领取用户ID"`
	BusinessDate         time.Time `gorm:"type:date;not null;uniqueIndex:idx_daily_checkin_user_date,priority:2;index:idx_daily_checkin_business_date;comment:业务日期"`
	AwardedCalls         int       `gorm:"not null;comment:中奖标准对话次数"`
	UnitPriceNanousd     int64     `gorm:"not null;comment:单次折算价格快照(纳美元)"`
	RewardNanousd        int64     `gorm:"not null;comment:实际入账金额(纳美元)"`
	PrizeKey             string    `gorm:"size:64;not null;comment:中奖档位键"`
	ConfigSnapshotJSON   string    `gorm:"type:text;not null;comment:中奖时奖池配置快照"`
	StreakDays           int       `gorm:"not null;default:1;comment:连续签到天数"`
	BalanceTransactionID uint      `gorm:"not null;default:0;index:idx_daily_checkin_balance_tx;comment:余额流水ID"`
}

func (DailyCheckinClaim) TableName() string { return "daily_checkin_claims" }
