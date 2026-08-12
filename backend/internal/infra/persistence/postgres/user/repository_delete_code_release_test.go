package user

import (
	"context"
	"strings"
	"testing"
	"time"

	domainregistrationcode "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/registrationcode"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/schema"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestDeleteAccountHardKeepsRegistrationCodeUsed 验证删除账号后，注册码仍保持 used，不影响别人的注册码。
func TestDeleteAccountHardKeepsRegistrationCodeUsed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:delete_account_release_code?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("SQLite repository test requires CGO")
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(schema.Models()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 待删除的用户：已用一个注册码注册（码处于 used 状态）。
	user := model.User{PublicID: "u_release", Username: "release-user", DisplayName: "Release", Email: "release@example.com", Role: domainuser.RoleUser, Status: domainuser.StatusActive}
	if err = db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	now := time.Now()
	usedCode := model.RegistrationCode{Code: "REG-USEDBYRELEASE", CodeHint: "EASE", Status: domainregistrationcode.StatusUsed, UsedByUserID: user.ID, UsedAt: &now}
	if err = db.Create(&usedCode).Error; err != nil {
		t.Fatalf("seed used code: %v", err)
	}
	// 另一个用户的码，删除时不应被误改。
	other := model.User{PublicID: "u_other", Username: "other-user", DisplayName: "Other", Email: "other@example.com", Role: domainuser.RoleUser, Status: domainuser.StatusActive}
	if err = db.Create(&other).Error; err != nil {
		t.Fatalf("seed other user: %v", err)
	}
	otherCode := model.RegistrationCode{Code: "REG-OTHER USED", CodeHint: "USED", Status: domainregistrationcode.StatusUsed, UsedByUserID: other.ID, UsedAt: &now}
	if err = db.Create(&otherCode).Error; err != nil {
		t.Fatalf("seed other code: %v", err)
	}
	// 已产生邀请关系的用户：注册码不能因删号重新变成可触发奖励的凭证。
	rewarded := model.User{PublicID: "u_rewarded", Username: "rewarded-user", DisplayName: "Rewarded", Email: "rewarded@example.com", Role: domainuser.RoleUser, Status: domainuser.StatusActive}
	if err = db.Create(&rewarded).Error; err != nil {
		t.Fatalf("seed rewarded user: %v", err)
	}
	rewardedCode := model.RegistrationCode{Code: "REG-REWARDED", CodeHint: "ARDED", Status: domainregistrationcode.StatusUsed, UsedByUserID: rewarded.ID, UsedAt: &now}
	if err = db.Create(&rewardedCode).Error; err != nil {
		t.Fatalf("seed rewarded code: %v", err)
	}
	if err = db.Create(&model.InvitationRelationship{
		InviterUserID:        other.ID,
		InvitedUserID:        rewarded.ID,
		InviteeEmail:         rewarded.Email,
		InvitationCode:       "INV-REWARDED",
		InviteeRewardNanousd: 500_000_000,
		InviterRewardNanousd: 500_000_000,
	}).Error; err != nil {
		t.Fatalf("seed invitation relationship: %v", err)
	}

	if err = NewRepo(db).DeleteAccountHard(context.Background(), user.ID); err != nil {
		t.Fatalf("DeleteAccountHard() error = %v", err)
	}

	// 被删用户的码：永久保持 used，避免通过删号重复注册。
	var consumed model.RegistrationCode
	if err = db.Where("id = ?", usedCode.ID).First(&consumed).Error; err != nil {
		t.Fatalf("consumed code not found: %v", err)
	}
	if consumed.Status != domainregistrationcode.StatusUsed {
		t.Fatalf("consumed code status = %q, want used", consumed.Status)
	}
	if consumed.UsedByUserID != user.ID {
		t.Fatalf("consumed code used_by_user_id = %d, want %d", consumed.UsedByUserID, user.ID)
	}
	if consumed.UsedAt == nil {
		t.Fatal("consumed code used_at must remain set")
	}

	// 别人的码：保持 used 不变。
	var untouched model.RegistrationCode
	if err = db.Where("id = ?", otherCode.ID).First(&untouched).Error; err != nil {
		t.Fatalf("other code not found: %v", err)
	}
	if untouched.Status != domainregistrationcode.StatusUsed || untouched.UsedByUserID != other.ID {
		t.Fatalf("other code should remain used by other user, got status=%q used_by=%d", untouched.Status, untouched.UsedByUserID)
	}

	if err = NewRepo(db).DeleteAccountHard(context.Background(), rewarded.ID); err != nil {
		t.Fatalf("DeleteAccountHard() rewarded user error = %v", err)
	}
	var protected model.RegistrationCode
	if err = db.Where("id = ?", rewardedCode.ID).First(&protected).Error; err != nil {
		t.Fatalf("protected code not found: %v", err)
	}
	if protected.Status != domainregistrationcode.StatusUsed || protected.UsedByUserID != rewarded.ID || protected.UsedAt == nil {
		t.Fatalf("rewarded code should remain used, got status=%q used_by=%d used_at=%v", protected.Status, protected.UsedByUserID, protected.UsedAt)
	}
}
