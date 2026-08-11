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

// TestDeleteAccountHardReleasesRegistrationCode 验证删除账号时，该用户用过的注册码恢复成 active 可重用，
// 清空 used_by_user_id / used_at，且不影响别的用户的码。
func TestDeleteAccountHardReleasesRegistrationCode(t *testing.T) {
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

	if err = NewRepo(db).DeleteAccountHard(context.Background(), user.ID); err != nil {
		t.Fatalf("DeleteAccountHard() error = %v", err)
	}

	// 被删用户的码：恢复 active，使用者标记清空。
	var released model.RegistrationCode
	if err = db.Where("id = ?", usedCode.ID).First(&released).Error; err != nil {
		t.Fatalf("released code not found: %v", err)
	}
	if released.Status != domainregistrationcode.StatusActive {
		t.Fatalf("released code status = %q, want active", released.Status)
	}
	if released.UsedByUserID != 0 {
		t.Fatalf("released code used_by_user_id = %d, want 0", released.UsedByUserID)
	}
	if released.UsedAt != nil {
		t.Fatalf("released code used_at = %v, want nil", released.UsedAt)
	}

	// 别人的码：保持 used 不变。
	var untouched model.RegistrationCode
	if err = db.Where("id = ?", otherCode.ID).First(&untouched).Error; err != nil {
		t.Fatalf("other code not found: %v", err)
	}
	if untouched.Status != domainregistrationcode.StatusUsed || untouched.UsedByUserID != other.ID {
		t.Fatalf("other code should remain used by other user, got status=%q used_by=%d", untouched.Status, untouched.UsedByUserID)
	}
}
