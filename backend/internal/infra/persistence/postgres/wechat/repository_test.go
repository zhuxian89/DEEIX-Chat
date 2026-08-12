package wechat

import (
	"context"
	"strings"
	"testing"
	"time"

	domainregistrationcode "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/registrationcode"
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

func TestIssueRegistrationCodeClassifiesUsedByExistingOrDeletedUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:wechat_issuance_user_state?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("SQLite repository test requires CGO")
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.RegistrationCode{}, &model.WeChatRegistrationIssuance{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	activeUser := model.User{PublicID: "wechat_active", Username: "wechat-active", Email: "wechat-active@example.com"}
	if err := db.Create(&activeUser).Error; err != nil {
		t.Fatalf("create active user: %v", err)
	}
	activeCode := model.RegistrationCode{Code: "REG-ACTIVE", CodeHint: "TIVE", Status: domainregistrationcode.StatusUsed, UsedByUserID: activeUser.ID, UsedAt: &now}
	if err := db.Create(&activeCode).Error; err != nil {
		t.Fatalf("create active code: %v", err)
	}
	if err := db.Create(&model.WeChatRegistrationIssuance{OpenID: "openid-active", RegistrationCodeID: activeCode.ID}).Error; err != nil {
		t.Fatalf("create active issuance: %v", err)
	}

	deletedCode := model.RegistrationCode{Code: "REG-DELETED", CodeHint: "ETED", Status: domainregistrationcode.StatusUsed, UsedByUserID: 999999, UsedAt: &now}
	if err := db.Create(&deletedCode).Error; err != nil {
		t.Fatalf("create deleted code: %v", err)
	}
	if err := db.Create(&model.WeChatRegistrationIssuance{OpenID: "openid-deleted", RegistrationCodeID: deletedCode.ID}).Error; err != nil {
		t.Fatalf("create deleted issuance: %v", err)
	}

	repo := NewRepo(db)
	active, err := repo.IssueRegistrationCodeDetailed(context.Background(), "openid-active", "REG-IGNORED")
	if err != nil {
		t.Fatalf("classify active user: %v", err)
	}
	if !active.Used || active.DeletedUser {
		t.Fatalf("active result = used:%v deleted:%v, want used true/deleted false", active.Used, active.DeletedUser)
	}

	deleted, err := repo.IssueRegistrationCodeDetailed(context.Background(), "openid-deleted", "REG-IGNORED")
	if err != nil {
		t.Fatalf("classify deleted user: %v", err)
	}
	if !deleted.Used || !deleted.DeletedUser {
		t.Fatalf("deleted result = used:%v deleted:%v, want used true/deleted true", deleted.Used, deleted.DeletedUser)
	}
}
