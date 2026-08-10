package wechat

import (
	"context"
	"strings"
	"testing"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestIssueRegistrationCodeIsIdempotentByOpenID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:wechat_issuance_idempotent?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("SQLite repository test requires CGO")
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.RegistrationCode{}, &model.WeChatRegistrationIssuance{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewRepo(db)
	first, err := repo.IssueRegistrationCode(context.Background(), "openid-1", "AAAA-BBBB-CCCC-DDDD")
	if err != nil {
		t.Fatalf("first issue: %v", err)
	}
	second, err := repo.IssueRegistrationCode(context.Background(), "openid-1", "EEEE-FFFF-GGGG-HHHH")
	if err != nil {
		t.Fatalf("second issue: %v", err)
	}
	if first != "AAAA-BBBB-CCCC-DDDD" || second != first {
		t.Fatalf("codes = %q, %q; want the first code returned twice", first, second)
	}

	var issuanceCount, codeCount int64
	if err := db.Model(&model.WeChatRegistrationIssuance{}).Count(&issuanceCount).Error; err != nil {
		t.Fatalf("count issuances: %v", err)
	}
	if err := db.Model(&model.RegistrationCode{}).Count(&codeCount).Error; err != nil {
		t.Fatalf("count codes: %v", err)
	}
	if issuanceCount != 1 || codeCount != 1 {
		t.Fatalf("issuances=%d codes=%d; want 1 and 1", issuanceCount, codeCount)
	}
}
