package dailycheckin

import (
	"context"
	"strings"
	"testing"
	"time"

	domaindailycheckin "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/dailycheckin"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestClaimCreditsBalanceExactlyOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:daily_checkin_claim?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("SQLite repository test requires CGO")
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.DailyCheckinClaim{}, &model.BillingAccount{}, &model.BalanceTransaction{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewRepo(db)
	businessDate := time.Date(2026, time.September, 3, 0, 0, 0, 0, time.UTC)
	input := domaindailycheckin.ClaimInput{
		UserID:             42,
		BusinessDate:       businessDate,
		AwardedCalls:       50,
		UnitPriceNanousd:   1_670_000,
		RewardNanousd:      83_500_000,
		PrizeKey:           "calls_50",
		ConfigSnapshotJSON: `{"version":1}`,
		CreatedAt:          time.Date(2026, time.September, 3, 1, 2, 3, 0, time.UTC),
	}

	first, claimedNow, err := repo.Claim(context.Background(), input)
	if err != nil {
		t.Fatalf("first Claim() error = %v", err)
	}
	if !claimedNow || first.BalanceTransactionID == 0 {
		t.Fatalf("first Claim() = %#v, claimedNow = %v", first, claimedNow)
	}

	secondInput := input
	secondInput.AwardedCalls = 200
	secondInput.RewardNanousd = 334_000_000
	secondInput.PrizeKey = "calls_200"
	second, claimedNow, err := repo.Claim(context.Background(), secondInput)
	if err != nil {
		t.Fatalf("second Claim() error = %v", err)
	}
	if claimedNow {
		t.Fatal("second Claim() claimedNow = true, want false")
	}
	if second.ID != first.ID || second.AwardedCalls != 50 || second.RewardNanousd != 83_500_000 {
		t.Fatalf("second Claim() = %#v, want original %#v", second, first)
	}

	var account model.BillingAccount
	if err := db.Where("user_id = ?", input.UserID).First(&account).Error; err != nil {
		t.Fatalf("load billing account: %v", err)
	}
	if account.BalanceNanousd != input.RewardNanousd {
		t.Fatalf("balance = %d, want %d", account.BalanceNanousd, input.RewardNanousd)
	}
	var transactionCount int64
	if err := db.Model(&model.BalanceTransaction{}).
		Where("user_id = ? AND type = ?", input.UserID, "daily_checkin_reward").
		Count(&transactionCount).Error; err != nil {
		t.Fatalf("count balance transactions: %v", err)
	}
	if transactionCount != 1 {
		t.Fatalf("transaction count = %d, want 1", transactionCount)
	}
}
