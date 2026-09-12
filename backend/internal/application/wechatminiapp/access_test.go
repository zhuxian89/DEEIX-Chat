package wechatminiapp

import (
	"context"
	"errors"
	"testing"

	appauth "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/auth"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechatminiapp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
)

type accessIssuer struct{ fakeIssuer }

func (*accessIssuer) IssueWeChatMiniAppLogin(context.Context, uint, bool, string, requestmeta.SessionAuditContext) (*appauth.LoginResult, error) {
	return &appauth.LoginResult{SessionID: "signed-session", AccessToken: "secret"}, nil
}

type registrarFunc func(context.Context, uint, string, string, string) error

func (f registrarFunc) Register(ctx context.Context, uid uint, sid, app, open string) error {
	return f(ctx, uid, sid, app, open)
}

func TestLoginRegistersBeforeReturningCredentials(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "registration fails"}[fail], func(t *testing.T) {
			s := NewService(enabledRuntime(), &fakeRepo{binding: &domain.Binding{UserID: 7, AppID: "wx-app", OpenID: "openid-1"}}, &fakeExchanger{identity: domain.Identity{OpenID: "openid-1"}}, &accessIssuer{})
			called := false
			s.SetSessionRegistrar(registrarFunc(func(_ context.Context, uid uint, sid, app, open string) error {
				called = true
				if uid != 7 || sid != "signed-session" || app != "wx-app" || open != "openid-1" {
					t.Fatal("wrong authoritative identity")
				}
				if fail {
					return errors.New("registration failed")
				}
				return nil
			}))
			result, err := s.Login(t.Context(), "code", "request", requestmeta.SessionAuditContext{})
			if !called {
				t.Fatal("credentials returned without registration")
			}
			if fail {
				if err == nil || result != nil {
					t.Fatal("credentials leaked on failure")
				}
			} else if err != nil || result.Auth.SessionID != "signed-session" {
				t.Fatalf("result %+v %v", result, err)
			}
		})
	}
}
