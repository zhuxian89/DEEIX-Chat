package registrationcode

import (
	"context"
	"strings"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/registrationcode"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDeleteUnusedRejectsWeChatIssuedCode(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:registration_code_wechat_delete?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("SQLite repository test requires CGO")
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.RegistrationCode{}, &model.WeChatRegistrationIssuance{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	item := model.RegistrationCode{Code: "REG-AAAABBBBCCCCDDDD", CodeHint: "DDDD", Status: registrationcode.StatusActive}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("create code: %v", err)
	}
	if err := db.Create(&model.WeChatRegistrationIssuance{OpenID: "openid-1", RegistrationCodeID: item.ID}).Error; err != nil {
		t.Fatalf("create issuance: %v", err)
	}

	err = NewRepo(db).DeleteUnused(context.Background(), item.ID)
	if err != repository.ErrConflict {
		t.Fatalf("DeleteUnused() error = %v, want ErrConflict", err)
	}
	var remaining model.RegistrationCode
	if err := db.First(&remaining, item.ID).Error; err != nil {
		t.Fatalf("load code after rejected delete: %v", err)
	}
}
