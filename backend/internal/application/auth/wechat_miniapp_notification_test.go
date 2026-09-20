package auth

import (
	"testing"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
)

type miniAppNotificationRecorder struct {
	webCalls     int
	miniAppCalls int
	userID       uint
	username     string
}

func (n *miniAppNotificationRecorder) NotifyRegistration(uint, string, string) {
	n.webCalls++
}

func (n *miniAppNotificationRecorder) NotifyMiniAppRegistration(userID uint, username string) {
	n.miniAppCalls++
	n.userID, n.username = userID, username
}

func TestMiniAppRegistrationNotificationOnlyOnFirstSuccessfulLogin(t *testing.T) {
	user := &domainuser.User{ID: 7, Username: "wechat-user", Status: domainuser.StatusActive, Role: domainuser.RoleUser}
	repo := &providerLoginRepo{usersByID: map[uint]*domainuser.User{7: user}}
	service := newTestService(config.Config{JWTSecret: "test-secret"}, repo, nil)
	notifier := &miniAppNotificationRecorder{}
	service.SetTelegramNotifier(notifier)

	if _, err := service.IssueWeChatMiniAppLogin(t.Context(), 7, true, "first", requestmeta.SessionAuditContext{}); err != nil {
		t.Fatal(err)
	}
	if notifier.miniAppCalls != 1 || notifier.webCalls != 0 || notifier.userID != 7 || notifier.username != "wechat-user" {
		t.Fatalf("registration was not routed exclusively to miniapp notification: %+v", notifier)
	}
	if _, err := service.IssueWeChatMiniAppLogin(t.Context(), 7, false, "repeat", requestmeta.SessionAuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.IssueWeChatMiniAppLogin(t.Context(), 999, true, "missing", requestmeta.SessionAuditContext{}); err == nil {
		t.Fatal("expected missing user login to fail")
	}
	if notifier.miniAppCalls != 1 || notifier.webCalls != 0 {
		t.Fatalf("repeat or failed login produced a registration notification: %+v", notifier)
	}
}
