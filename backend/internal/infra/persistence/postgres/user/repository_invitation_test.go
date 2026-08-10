package user

import (
	"context"
	"strings"
	"testing"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/invitation"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func invitationTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("SQLite repository test requires CGO")
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserCredential{},
		&model.Subscription{},
		&model.BillingAccount{},
		&model.BalanceTransaction{},
		&model.InvitationCode{},
		&model.InvitationRelationship{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newInvitationUser(repo *Repo, id int, username string) *domainuser.User {
	return &domainuser.User{ID: uint(id), PublicID: username + "_pid", Username: username, DisplayName: username, Email: username + "@example.com", Role: domainuser.RoleUser, Status: domainuser.StatusActive}
}

func TestCreateWithInvitationCodeGrantsBothRewards(t *testing.T) {
	db := invitationTestDB(t, "invitation_both_rewards")
	repo := NewRepo(db)
	ctx := context.Background()
	now := time.Now()

	// 邀请人：先建账号并为其生成邀请码。
	inviter := newInvitationUser(repo, 1, "inviter")
	if err := repo.CreateWithCredential(ctx, inviter, domainuser.Credential{PasswordHash: "x", PasswordAlgo: "bcrypt", PasswordEnabled: true, PasswordOrigin: domainuser.PasswordOriginLocalRegister}, 0, 0, nil, false); err != nil {
		t.Fatalf("create inviter: %v", err)
	}
	inviterCode, err := invitation.GenerateCode(invitation.DefaultCodeLength)
	if err != nil {
		t.Fatalf("generate inviter code: %v", err)
	}
	if err := db.Create(&model.InvitationCode{UserID: inviter.ID, Code: inviterCode}).Error; err != nil {
		t.Fatalf("seed inviter code: %v", err)
	}

	// 被邀请人通过邀请码注册。
	invitee := newInvitationUser(repo, 2, "invitee")
	invitee.ID = 0
	invitee.PublicID = "invitee_pid"
	input := invitation.ApplyInput{
		Code:                 inviterCode,
		Enabled:              true,
		InviteeRewardNanousd: 500_000_000,
		InviterRewardNanousd: 500_000_000,
		CodeLength:           invitation.DefaultCodeLength,
	}
	if err := repo.CreateWithCredentialAndInvitationCode(ctx, invitee, domainuser.Credential{PasswordHash: "x", PasswordAlgo: "bcrypt", PasswordEnabled: true, PasswordOrigin: domainuser.PasswordOriginLocalRegister}, 0, 0, nil, false, "", 0, now, input); err != nil {
		t.Fatalf("register invitee: %v", err)
	}

	// 被邀请人获得自己的邀请码。
	var inviteeCode model.InvitationCode
	if err := db.Where("user_id = ?", invitee.ID).First(&inviteeCode).Error; err != nil {
		t.Fatalf("invitee code not generated: %v", err)
	}
	if !strings.HasPrefix(inviteeCode.Code, invitation.CodePrefix) {
		t.Fatalf("invitee code missing prefix: %s", inviteeCode.Code)
	}

	// 邀请关系建立。
	var rel model.InvitationRelationship
	if err := db.Where("invited_user_id = ?", invitee.ID).First(&rel).Error; err != nil {
		t.Fatalf("invitation relationship not created: %v", err)
	}
	if rel.InviterUserID != inviter.ID {
		t.Fatalf("inviter mismatch: got %d want %d", rel.InviterUserID, inviter.ID)
	}

	// 双方余额到账。
	var inviteeAccount, inviterAccount model.BillingAccount
	if err := db.Where("user_id = ?", invitee.ID).First(&inviteeAccount).Error; err != nil {
		t.Fatalf("invitee account not created: %v", err)
	}
	if inviteeAccount.BalanceNanousd != 500_000_000 {
		t.Fatalf("invitee balance = %d, want 500000000", inviteeAccount.BalanceNanousd)
	}
	if err := db.Where("user_id = ?", inviter.ID).First(&inviterAccount).Error; err != nil {
		t.Fatalf("inviter account not created: %v", err)
	}
	if inviterAccount.BalanceNanousd != 500_000_000 {
		t.Fatalf("inviter balance = %d, want 500000000", inviterAccount.BalanceNanousd)
	}
}

func TestCreateWithInvitationCodeLenientOnInvalidCode(t *testing.T) {
	db := invitationTestDB(t, "invitation_invalid_code")
	repo := NewRepo(db)
	ctx := context.Background()
	now := time.Now()

	invitee := newInvitationUser(repo, 0, "invitee2")
	invitee.PublicID = "invitee2_pid"
	// 无效邀请码：宽松放行，正常建用户，不发奖。
	input := invitation.ApplyInput{
		Code:                 "INV-NOTEXIST",
		Enabled:              true,
		InviteeRewardNanousd: 500_000_000,
		InviterRewardNanousd: 500_000_000,
		CodeLength:           invitation.DefaultCodeLength,
	}
	if err := repo.CreateWithCredentialAndInvitationCode(ctx, invitee, domainuser.Credential{PasswordHash: "x", PasswordAlgo: "bcrypt", PasswordEnabled: true, PasswordOrigin: domainuser.PasswordOriginLocalRegister}, 0, 0, nil, false, "", 0, now, input); err != nil {
		t.Fatalf("register with invalid code should succeed (lenient): %v", err)
	}

	var count int64
	db.Model(&model.InvitationRelationship{}).Count(&count)
	if count != 0 {
		t.Fatalf("no relationship should be created for invalid code, got %d", count)
	}
	var accountCount int64
	db.Model(&model.BillingAccount{}).Count(&accountCount)
	if accountCount != 0 {
		t.Fatalf("no reward should be granted for invalid code, got %d accounts", accountCount)
	}
	// 被邀请人仍获得自己的邀请码。
	var codeCount int64
	db.Model(&model.InvitationCode{}).Where("user_id = ?", invitee.ID).Count(&codeCount)
	if codeCount != 1 {
		t.Fatalf("invitee own code should be generated even with invalid invitation code, got %d", codeCount)
	}
}

func TestCreateWithInvitationCodeDisabledSkipsReward(t *testing.T) {
	db := invitationTestDB(t, "invitation_disabled")
	repo := NewRepo(db)
	ctx := context.Background()
	now := time.Now()

	invitee := newInvitationUser(repo, 0, "invitee3")
	invitee.PublicID = "invitee3_pid"
	input := invitation.ApplyInput{
		Code:                 "INV-ANYTHING",
		Enabled:              false, // 功能关闭
		InviteeRewardNanousd: 500_000_000,
		InviterRewardNanousd: 500_000_000,
		CodeLength:           invitation.DefaultCodeLength,
	}
	if err := repo.CreateWithCredentialAndInvitationCode(ctx, invitee, domainuser.Credential{PasswordHash: "x", PasswordAlgo: "bcrypt", PasswordEnabled: true, PasswordOrigin: domainuser.PasswordOriginLocalRegister}, 0, 0, nil, false, "", 0, now, input); err != nil {
		t.Fatalf("register with disabled invitation should succeed: %v", err)
	}
	var count int64
	db.Model(&model.InvitationRelationship{}).Count(&count)
	if count != 0 {
		t.Fatalf("no relationship when disabled, got %d", count)
	}
}
