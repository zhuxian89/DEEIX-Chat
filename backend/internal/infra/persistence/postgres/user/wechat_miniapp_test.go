package user

import (
	"context"
	"errors"
	"testing"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	domainwechatminiapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechatminiapp"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openMiniAppTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&model.User{}, &model.UserCredential{}, &model.InvitationCode{}, &model.UserSession{}, &model.WeChatMiniAppBinding{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCreateMiniAppUserAndBindingIsAtomicAndPasswordless(t *testing.T) {
	db := openMiniAppTestDB(t)
	repo := NewRepo(db)
	now := time.Now().UTC()
	user := &domainuser.User{PublicID: "user-1", Username: "wx-user-1", DisplayName: "微信用户", Role: domainuser.RoleUser, Status: domainuser.StatusActive, Locale: "zh-CN", Timezone: "Asia/Shanghai"}
	binding := &domainwechatminiapp.Binding{AppID: "wx-app", OpenID: "openid-1", UnionID: "union-1", LastLoginAt: now}
	err := repo.CreateMiniAppUserAndBinding(context.Background(), user, domainuser.Credential{PasswordEnabled: false, PasswordOrigin: domainuser.PasswordOriginSSOPlaceholder}, binding, 8)
	if err != nil {
		t.Fatalf("CreateMiniAppUserAndBinding() error = %v", err)
	}
	if user.ID == 0 || binding.UserID != user.ID {
		t.Fatalf("user=%+v binding=%+v", user, binding)
	}
	var credential model.UserCredential
	if err = db.Where("user_id = ?", user.ID).First(&credential).Error; err != nil {
		t.Fatalf("load credential: %v", err)
	}
	if credential.PasswordEnabled {
		t.Fatal("password credential unexpectedly enabled")
	}
	var invitationCount int64
	if err = db.Model(&model.InvitationCode{}).Where("user_id = ?", user.ID).Count(&invitationCount).Error; err != nil || invitationCount != 1 {
		t.Fatalf("invitation count=%d error=%v", invitationCount, err)
	}
}

func TestDuplicateOpenIDRollsBackSecondUser(t *testing.T) {
	db := openMiniAppTestDB(t)
	repo := NewRepo(db)
	create := func(publicID, username string) error {
		user := &domainuser.User{PublicID: publicID, Username: username, DisplayName: username, Role: domainuser.RoleUser, Status: domainuser.StatusActive}
		binding := &domainwechatminiapp.Binding{AppID: "wx-app", OpenID: "same-openid", LastLoginAt: time.Now()}
		return repo.CreateMiniAppUserAndBinding(context.Background(), user, domainuser.Credential{PasswordEnabled: false}, binding, 8)
	}
	if err := create("user-1", "wx-user-1"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := create("user-2", "wx-user-2"); !errors.Is(err, repository.ErrDuplicate) {
		t.Fatalf("second create error=%v, want duplicate", err)
	}
	var count int64
	if err := db.Model(&model.User{}).Where("username IN ?", []string{"wx-user-1", "wx-user-2"}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("user count=%d error=%v", count, err)
	}
}

func TestTouchMiniAppBindingRejectsUnionIDChangeAndSurvivesUserDelete(t *testing.T) {
	db := openMiniAppTestDB(t)
	repo := NewRepo(db)
	now := time.Now().UTC()
	user := &domainuser.User{PublicID: "user-1", Username: "wx-user-1", DisplayName: "微信用户", Role: domainuser.RoleUser, Status: domainuser.StatusActive}
	binding := &domainwechatminiapp.Binding{AppID: "wx-app", OpenID: "openid-1", UnionID: "union-1", LastLoginAt: now}
	if err := repo.CreateMiniAppUserAndBinding(context.Background(), user, domainuser.Credential{PasswordEnabled: false}, binding, 8); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repo.TouchMiniAppBinding(context.Background(), "wx-app", "openid-1", "changed-union", now.Add(time.Minute)); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("TouchMiniAppBinding() error=%v, want conflict", err)
	}
	if err := db.Unscoped().Delete(&model.User{}, user.ID).Error; err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := repo.GetMiniAppBinding(context.Background(), "wx-app", "openid-1"); err != nil {
		t.Fatalf("binding should survive user deletion: %v", err)
	}
}

func TestRevokeActiveMiniAppSessionsDoesNotTouchWebSessions(t *testing.T) {
	db := openMiniAppTestDB(t)
	repo := NewRepo(db)
	now := time.Now().UTC()
	sessions := []model.UserSession{
		{SessionID: "mini", UserID: 9, UserAgent: "DEEIX-WeChat-MiniApp device", IssuedAt: now, ExpiresAt: now.Add(time.Hour)},
		{SessionID: "web", UserID: 9, UserAgent: "Mozilla/5.0", IssuedAt: now, ExpiresAt: now.Add(time.Hour)},
	}
	if err := db.Create(&sessions).Error; err != nil {
		t.Fatalf("seed sessions: %v", err)
	}
	if err := repo.RevokeActiveMiniAppSessions(context.Background(), 9, "DEEIX-WeChat-MiniApp", now.Add(time.Minute)); err != nil {
		t.Fatalf("RevokeActiveMiniAppSessions() error = %v", err)
	}
	var mini, web model.UserSession
	if err := db.Where("session_id = ?", "mini").First(&mini).Error; err != nil {
		t.Fatalf("load mini session: %v", err)
	}
	if err := db.Where("session_id = ?", "web").First(&web).Error; err != nil {
		t.Fatalf("load web session: %v", err)
	}
	if mini.RevokedAt == nil || web.RevokedAt != nil {
		t.Fatalf("mini revoked=%v web revoked=%v", mini.RevokedAt, web.RevokedAt)
	}
}
