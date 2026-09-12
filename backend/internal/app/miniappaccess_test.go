package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniappaccess"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/token"
	httpx "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	access "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/miniappaccess"
	"github.com/gin-gonic/gin"
)

type sessionSourceReader struct{}

func (sessionSourceReader) Access(_ context.Context, _ uint, sid string) (domain.Decision, error) {
	return domain.Decision{MiniApp: sid == "mini"}, nil
}

func TestAccessGateCoversRoutesRegisteredAfterEngineConstruction(t *testing.T) {
	cfg := config.NewRuntime(config.Config{JWTSecret: "integration-secret", Env: "test"})
	r, err := httpx.NewEngine(cfg, nil, httpx.Modules{SessionAccess: access.Middleware("integration-secret", sessionSourceReader{})}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Fork-owned speech/companion routes are registered after NewEngine returns.
	g := r.Group("/api/v1", middleware.AuthMiddleware("integration-secret", nil))
	for _, path := range []string{"/speech/transcriptions", "/companion/open", "/conversations", "/files", "/future-protected-route"} {
		g.POST(path, func(c *gin.Context) { c.Status(204) })
	}
	for _, sid := range []string{"mini", "web"} {
		for _, jti := range []string{"before-refresh", "after-refresh"} {
			tok, err := token.GenerateWithClaims("integration-secret", 7, "user", "user", sid, jti, "access", time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/speech/transcriptions", "/companion/open", "/conversations", "/files", "/future-protected-route"} {
				req := httptest.NewRequest(http.MethodPost, "/api/v1"+path, nil)
				req.Header.Set("Authorization", "Bearer "+tok)
				req.Header.Set("User-Agent", "DEEIX-WeChat-MiniApp")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				want := 204
				if sid == "mini" {
					want = 403
				}
				if w.Code != want {
					t.Fatalf("%s %s %s: %d %s", sid, jti, path, w.Code, w.Body.String())
				}
			}
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", nil)
	req.Header.Set("Authorization", "Bearer forged-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("existing authentication bypassed")
	}
}
