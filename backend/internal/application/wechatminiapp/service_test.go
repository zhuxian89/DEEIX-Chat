package wechatminiapp

import (
	"context"
	"errors"
	"testing"
	"time"

	appauth "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/auth"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	domainwechatminiapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechatminiapp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
)

type fakeExchanger struct {
	identity domainwechatminiapp.Identity
	calls    int
}

func (f *fakeExchanger) Exchange(context.Context, string, string, string) (domainwechatminiapp.Identity, error) {
	f.calls++
	return f.identity, nil
}

type fakeRepo struct {
	binding        *domainwechatminiapp.Binding
	touchErr       error
	createdUser    *domainuser.User
	credential     domainuser.Credential
	createdBinding *domainwechatminiapp.Binding
}

func (f *fakeRepo) GetMiniAppBinding(context.Context, string, string) (*domainwechatminiapp.Binding, error) {
	if f.binding == nil {
		return nil, repository.ErrNotFound
	}
	return f.binding, nil
}

func (f *fakeRepo) TouchMiniAppBinding(context.Context, string, string, string, time.Time) (*domainwechatminiapp.Binding, error) {
	if f.touchErr != nil {
		return nil, f.touchErr
	}
	if f.binding == nil {
		return nil, repository.ErrNotFound
	}
	return f.binding, nil
}

func (f *fakeRepo) CreateMiniAppUserAndBinding(_ context.Context, user *domainuser.User, credential domainuser.Credential, binding *domainwechatminiapp.Binding, _ int) error {
	f.createdUser = user
	f.credential = credential
	binding.UserID = 42
	f.createdBinding = binding
	f.binding = binding
	return nil
}

type fakeIssuer struct {
	userID         uint
	created        bool
	conflictUserID uint
}

func (f *fakeIssuer) RecordWeChatMiniAppIdentityConflict(_ context.Context, userID uint, _ string, _ requestmeta.SessionAuditContext) {
	f.conflictUserID = userID
}

func (f *fakeIssuer) IssueWeChatMiniAppLogin(_ context.Context, userID uint, created bool, _ string, _ requestmeta.SessionAuditContext) (*appauth.LoginResult, error) {
	f.userID, f.created = userID, created
	return &appauth.LoginResult{AccessToken: "access"}, nil
}

func enabledRuntime() *config.Runtime {
	return config.NewRuntime(config.Config{
		WeChatMiniAppEnabled:           true,
		WeChatMiniAppAppID:             "wx-app",
		WeChatMiniAppAppSecret:         "secret",
		WeChatMiniAppDefaultChatModel:  "chat-model",
		WeChatMiniAppDefaultImageModel: "image-model",
		InvitationCodeLength:           8,
	})
}

func TestLoginCreatesPasswordlessCanonicalUser(t *testing.T) {
	repo := &fakeRepo{}
	exchanger := &fakeExchanger{identity: domainwechatminiapp.Identity{OpenID: "openid-1", UnionID: "union-1"}}
	issuer := &fakeIssuer{}
	service := NewService(enabledRuntime(), repo, exchanger, issuer)
	fixedNow := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }

	result, err := service.Login(context.Background(), "temporary-code", "request-1", requestmeta.SessionAuditContext{})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !result.Created || issuer.userID != 42 || !issuer.created {
		t.Fatalf("result=%+v issuer=%+v", result, issuer)
	}
	if repo.createdUser == nil || repo.createdUser.Email != "" || repo.createdUser.Role != domainuser.RoleUser {
		t.Fatalf("created user = %+v", repo.createdUser)
	}
	if repo.credential.PasswordEnabled || repo.credential.PasswordOrigin != domainuser.PasswordOriginSSOPlaceholder {
		t.Fatalf("credential = %+v", repo.credential)
	}
	if repo.createdBinding.UnionID != "union-1" || repo.createdBinding.UnionIDObservedAt == nil {
		t.Fatalf("binding = %+v", repo.createdBinding)
	}
	if result.Presets.ChatModel != "chat-model" || result.Presets.ImageModel != "image-model" {
		t.Fatalf("presets = %+v", result.Presets)
	}
}

func TestLoginUsesExistingOpenIDBinding(t *testing.T) {
	repo := &fakeRepo{binding: &domainwechatminiapp.Binding{UserID: 7, AppID: "wx-app", OpenID: "openid-1"}}
	issuer := &fakeIssuer{}
	service := NewService(enabledRuntime(), repo, &fakeExchanger{identity: domainwechatminiapp.Identity{OpenID: "openid-1"}}, issuer)

	result, err := service.Login(context.Background(), "temporary-code", "request-1", requestmeta.SessionAuditContext{})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.Created || issuer.userID != 7 || issuer.created || repo.createdUser != nil {
		t.Fatalf("result=%+v issuer=%+v createdUser=%+v", result, issuer, repo.createdUser)
	}
}

func TestLoginRejectsUnionIDConflict(t *testing.T) {
	repo := &fakeRepo{binding: &domainwechatminiapp.Binding{UserID: 7, AppID: "wx-app", OpenID: "openid-1", UnionID: "original"}, touchErr: repository.ErrConflict}
	issuer := &fakeIssuer{}
	service := NewService(enabledRuntime(), repo, &fakeExchanger{identity: domainwechatminiapp.Identity{OpenID: "openid-1", UnionID: "changed"}}, issuer)
	_, err := service.Login(context.Background(), "temporary-code", "request-1", requestmeta.SessionAuditContext{})
	if !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("Login() error = %v, want ErrIdentityConflict", err)
	}
	if issuer.conflictUserID != 7 {
		t.Fatalf("conflict audit user id = %d, want 7", issuer.conflictUserID)
	}
}

func TestLoginDisabledStopsBeforeWeChat(t *testing.T) {
	exchanger := &fakeExchanger{}
	service := NewService(config.NewRuntime(config.Config{}), &fakeRepo{}, exchanger, &fakeIssuer{})
	_, err := service.Login(context.Background(), "temporary-code", "request-1", requestmeta.SessionAuditContext{})
	if !errors.Is(err, ErrDisabled) || exchanger.calls != 0 {
		t.Fatalf("error=%v exchange calls=%d", err, exchanger.calls)
	}
}
