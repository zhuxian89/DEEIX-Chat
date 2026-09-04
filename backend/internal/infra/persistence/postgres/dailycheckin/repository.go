package dailycheckin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domainbilling "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/billing"
	domaindailycheckin "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/dailycheckin"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Claim inserts the unique daily row first, then credits the existing billing ledger in the same transaction.
func (r *Repo) Claim(ctx context.Context, input domaindailycheckin.ClaimInput) (domaindailycheckin.Claim, bool, error) {
	if input.UserID == 0 || input.BusinessDate.IsZero() || input.AwardedCalls <= 0 ||
		input.UnitPriceNanousd <= 0 || input.RewardNanousd <= 0 || strings.TrimSpace(input.PrizeKey) == "" {
		return domaindailycheckin.Claim{}, false, errors.New("invalid daily check-in claim")
	}
	var output domaindailycheckin.Claim
	claimedNow := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		item := model.DailyCheckinClaim{
			ControlPlaneModel:  model.ControlPlaneModel{CreatedAt: input.CreatedAt, UpdatedAt: input.CreatedAt},
			UserID:             input.UserID,
			BusinessDate:       input.BusinessDate,
			AwardedCalls:       input.AwardedCalls,
			UnitPriceNanousd:   input.UnitPriceNanousd,
			RewardNanousd:      input.RewardNanousd,
			PrizeKey:           input.PrizeKey,
			ConfigSnapshotJSON: input.ConfigSnapshotJSON,
			StreakDays:         1,
		}
		created := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "business_date"}},
			DoNothing: true,
		}).Create(&item)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			if err := tx.Where("user_id = ? AND business_date = ?", input.UserID, input.BusinessDate).First(&item).Error; err != nil {
				return err
			}
			output = toDomain(item)
			return nil
		}

		var previous model.DailyCheckinClaim
		if err := tx.Where("user_id = ? AND business_date < ?", input.UserID, input.BusinessDate).
			Order("business_date DESC").First(&previous).Error; err == nil {
			if previous.BusinessDate.Equal(input.BusinessDate.AddDate(0, 0, -1)) {
				item.StreakDays = previous.StreakDays + 1
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		account, err := getOrCreateBillingAccountForUpdate(tx, input.UserID)
		if err != nil {
			return err
		}
		if err := tx.Model(account).Updates(map[string]interface{}{
			"balance_nanousd": gorm.Expr("balance_nanousd + ?", input.RewardNanousd),
			"currency":        "USD",
			"status":          "active",
		}).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", account.ID).First(account).Error; err != nil {
			return err
		}
		transaction := model.BalanceTransaction{
			AccountID:           account.ID,
			UserID:              input.UserID,
			Type:                domainbilling.BalanceTransactionTypeDailyCheckin,
			AmountNanousd:       input.RewardNanousd,
			BalanceAfterNanousd: account.BalanceNanousd,
			RefType:             domaindailycheckin.BalanceRefType,
			RefID:               item.ID,
			RefNo:               fmt.Sprintf("daily-checkin:%d:%s", input.UserID, input.BusinessDate.Format("2006-01-02")),
			Description:         fmt.Sprintf("每日签到奖励：%d 次标准对话", input.AwardedCalls),
		}
		if err := tx.Create(&transaction).Error; err != nil {
			return err
		}
		item.BalanceTransactionID = transaction.ID
		if err := tx.Model(&item).Updates(map[string]interface{}{
			"streak_days":            item.StreakDays,
			"balance_transaction_id": item.BalanceTransactionID,
		}).Error; err != nil {
			return err
		}
		claimedNow = true
		output = toDomain(item)
		return nil
	})
	return output, claimedNow, err
}

func (r *Repo) Get(ctx context.Context, userID uint, businessDate time.Time) (*domaindailycheckin.Claim, error) {
	if userID == 0 || businessDate.IsZero() {
		return nil, errors.New("invalid daily check-in lookup")
	}
	var item model.DailyCheckinClaim
	if err := r.db.WithContext(ctx).Where("user_id = ? AND business_date = ?", userID, businessDate).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	result := toDomain(item)
	return &result, nil
}

func getOrCreateBillingAccountForUpdate(tx *gorm.DB, userID uint) (*model.BillingAccount, error) {
	seed := model.BillingAccount{UserID: userID, Currency: "USD", Status: "active"}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoNothing: true,
	}).Create(&seed).Error; err != nil {
		return nil, err
	}
	var account model.BillingAccount
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func toDomain(item model.DailyCheckinClaim) domaindailycheckin.Claim {
	return domaindailycheckin.Claim{
		ID:                   item.ID,
		UserID:               item.UserID,
		BusinessDate:         item.BusinessDate,
		AwardedCalls:         item.AwardedCalls,
		UnitPriceNanousd:     item.UnitPriceNanousd,
		RewardNanousd:        item.RewardNanousd,
		PrizeKey:             strings.TrimSpace(item.PrizeKey),
		ConfigSnapshotJSON:   item.ConfigSnapshotJSON,
		StreakDays:           item.StreakDays,
		BalanceTransactionID: item.BalanceTransactionID,
		CreatedAt:            item.CreatedAt,
	}
}
