package user

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	domaininvitation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/invitation"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestCreateWithInvitationCodeDoesNotAbortPostgresTransaction verifies that a
// duplicate normalized email skips rewards without aborting the registration transaction.
func TestCreateWithInvitationCodeDoesNotAbortPostgresTransaction(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("DEEIX_TEST_DATABASE_DSN"))
	if dsn == "" {
		t.Skip("set DEEIX_TEST_DATABASE_DSN to run PostgreSQL invitation integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("resolve postgres db: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	defer sqlDB.Close()

	if err = db.AutoMigrate(
		&model.User{},
		&model.UserCredential{},
		&model.Subscription{},
		&model.BillingAccount{},
		&model.BalanceTransaction{},
		&model.InvitationCode{},
		&model.InvitationRelationship{},
	); err != nil {
		t.Fatalf("migrate invitation tables: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	prefix := "pg_invitation_" + suffix
	repo := NewRepo(db)
	ctx := context.Background()
	now := time.Now()
	defer func() {
		var users []model.User
		if cleanupErr := db.Unscoped().Where("username LIKE ?", prefix+"%").Find(&users).Error; cleanupErr != nil {
			t.Errorf("find test users for cleanup: %v", cleanupErr)
			return
		}
		ids := make([]uint, 0, len(users))
		for _, item := range users {
			ids = append(ids, item.ID)
		}
		if len(ids) == 0 {
			return
		}
		_ = db.Where("inviter_user_id IN ? OR invited_user_id IN ?", ids, ids).Delete(&model.InvitationRelationship{}).Error
		_ = db.Where("user_id IN ?", ids).Delete(&model.BalanceTransaction{}).Error
		_ = db.Where("user_id IN ?", ids).Delete(&model.BillingAccount{}).Error
		_ = db.Where("user_id IN ?", ids).Delete(&model.Subscription{}).Error
		_ = db.Where("user_id IN ?", ids).Delete(&model.UserCredential{}).Error
		_ = db.Where("user_id IN ?", ids).Delete(&model.InvitationCode{}).Error
		_ = db.Unscoped().Where("id IN ?", ids).Delete(&model.User{}).Error
	}()

	inviter := newInvitationUser(repo, 0, prefix+"_inviter")
	if err = repo.CreateWithCredential(ctx, inviter, domainuser.Credential{
		PasswordHash:    "x",
		PasswordAlgo:    "bcrypt",
		PasswordEnabled: true,
		PasswordOrigin:  domainuser.PasswordOriginLocalRegister,
	}, 0, 0, nil, false); err != nil {
		t.Fatalf("create inviter: %v", err)
	}
	inviterCode, err := domaininvitation.GenerateCode(domaininvitation.DefaultCodeLength)
	if err != nil {
		t.Fatalf("generate inviter code: %v", err)
	}
	if err = db.Create(&model.InvitationCode{UserID: inviter.ID, Code: inviterCode}).Error; err != nil {
		t.Fatalf("seed inviter code: %v", err)
	}

	input := domaininvitation.ApplyInput{
		Code:                 inviterCode,
		Enabled:              true,
		InviteeRewardNanousd: 500_000_000,
		InviterRewardNanousd: 500_000_000,
		CodeLength:           domaininvitation.DefaultCodeLength,
	}
	first := newInvitationUser(repo, 0, prefix+"_first")
	first.Email = "Repeat@" + prefix + ".example.com"
	if err = repo.CreateWithCredentialAndInvitationCode(ctx, first, domainuser.Credential{
		PasswordHash:    "x",
		PasswordAlgo:    "bcrypt",
		PasswordEnabled: true,
		PasswordOrigin:  domainuser.PasswordOriginLocalRegister,
	}, 0, 0, nil, false, "", 0, now, input); err != nil {
		t.Fatalf("register first invitee: %v", err)
	}

	second := newInvitationUser(repo, 0, prefix+"_second")
	second.Email = " repeat@" + prefix + ".EXAMPLE.com "
	if err = repo.CreateWithCredentialAndInvitationCode(ctx, second, domainuser.Credential{
		PasswordHash:    "x",
		PasswordAlgo:    "bcrypt",
		PasswordEnabled: true,
		PasswordOrigin:  domainuser.PasswordOriginLocalRegister,
	}, 0, 0, nil, false, "", 0, now, input); err != nil {
		t.Fatalf("register duplicate-email invitee: %v", err)
	}

	var relationshipCount int64
	if err = db.Model(&model.InvitationRelationship{}).Where("inviter_user_id = ?", inviter.ID).Count(&relationshipCount).Error; err != nil {
		t.Fatalf("count invitation relationships: %v", err)
	}
	if relationshipCount != 1 {
		t.Fatalf("relationship count = %d, want 1", relationshipCount)
	}

	var account model.BillingAccount
	if err = db.Where("user_id = ?", inviter.ID).First(&account).Error; err != nil {
		t.Fatalf("load inviter account: %v", err)
	}
	if account.BalanceNanousd != 500_000_000 {
		t.Fatalf("inviter balance = %d, want 500000000", account.BalanceNanousd)
	}

}
