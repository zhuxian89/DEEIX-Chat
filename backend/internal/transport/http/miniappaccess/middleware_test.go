package miniappaccess

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniappaccess"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/token"
	"github.com/gin-gonic/gin"
)

type readerFunc func(context.Context, uint, string) (domain.Decision, error)

func (f readerFunc) Access(ctx context.Context, uid uint, sid string) (domain.Decision, error) {
	return f(ctx, uid, sid)
}

func TestSessionSourcePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, path, sid, tokenType string
		decision                           domain.Decision
		failure                            bool
		want                               int
	}{
		{"web chat", "POST", "/api/v1/conversations/a/messages/stream", "web", "access", domain.Decision{}, false, 204},
		{"web files", "GET", "/api/v1/files/a/content", "web", "access", domain.Decision{}, false, 204},
		{"locked chat", "POST", "/api/v1/conversations/a/messages/stream", "mini", "access", domain.Decision{MiniApp: true}, false, 403},
		{"locked new API", "POST", "/api/v1/future-capability", "mini", "access", domain.Decision{MiniApp: true}, false, 403},
		{"locked speech", "POST", "/api/v1/speech/transcriptions", "mini", "access", domain.Decision{MiniApp: true}, false, 403},
		{"locked companion", "POST", "/api/v1/companion/open", "mini", "access", domain.Decision{MiniApp: true}, false, 403},
		{"todo", "POST", "/api/v1/miniapp-todo/sync", "mini", "access", domain.Decision{MiniApp: true}, false, 204},
		{"feedback", "POST", "/api/v1/miniapp-todo/feedback", "mini", "access", domain.Decision{MiniApp: true}, false, 204},
		{"no prefix exemption", "GET", "/api/v1/miniapp-todo/unknown", "mini", "access", domain.Decision{MiniApp: true}, false, 403},
		{"unlocked", "POST", "/api/v1/conversations", "mini", "access", domain.Decision{MiniApp: true, Unlocked: true}, false, 204},
		{"legacy access", "GET", "/api/v1/miniapp-entry/status", "old", "access", domain.Decision{Reauthenticate: true}, false, 401},
		{"legacy refresh", "POST", "/api/v1/auth/refresh", "old", "refresh", domain.Decision{Reauthenticate: true}, false, 401},
		{"web refresh", "POST", "/api/v1/auth/refresh", "web", "refresh", domain.Decision{}, false, 204},
		{"mini refresh", "POST", "/api/v1/auth/refresh", "mini", "refresh", domain.Decision{MiniApp: true}, false, 204},
		{"failure closed", "POST", "/api/v1/conversations", "mini", "access", domain.Decision{}, true, 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(Middleware("test-secret", readerFunc(func(_ context.Context, uid uint, sid string) (domain.Decision, error) {
				if uid != 7 || sid != tc.sid {
					t.Fatalf("untrusted identity: %d %s", uid, sid)
				}
				if tc.failure {
					return domain.Decision{}, errors.New("database unavailable")
				}
				return tc.decision, nil
			})))
			r.Handle(tc.method, tc.path, func(c *gin.Context) { c.Status(204) })
			tok, err := token.GenerateWithClaims("test-secret", 7, "user", "user", tc.sid, "jti", tc.tokenType, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(tc.method, tc.path, nil)
			// A spoofed Web header cannot turn a miniapp session into Web (or vice versa).
			req.Header.Set("User-Agent", "Mozilla/5.0 DEEIX-WeChat-MiniApp")
			req.Header.Set("X-Client-Type", "web")
			if tc.tokenType == "refresh" {
				req.AddCookie(&http.Cookie{Name: "deeix_chat_refresh_token", Value: tok})
			} else {
				req.Header.Set("Authorization", "Bearer "+tok)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
