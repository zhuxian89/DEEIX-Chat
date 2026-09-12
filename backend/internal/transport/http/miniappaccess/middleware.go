package miniappaccess

import (
	"net/http"
	"strings"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniappaccess"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/token"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// Middleware must be installed before any routes, including fork-owned routes.
// Existing route authentication still verifies session validity and ownership.
func Middleware(secret string, reader domain.Reader) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
			c.Next()
			return
		}
		refresh := c.Request.Method == http.MethodPost && c.Request.URL.Path == "/api/v1/auth/refresh"
		var raw string
		if refresh {
			raw, _ = c.Cookie("deeix_chat_refresh_token")
		} else {
			header := c.GetHeader("Authorization")
			if strings.HasPrefix(header, "Bearer ") {
				raw = strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			}
		}
		if raw == "" {
			c.Next()
			return
		}
		claims, err := token.Parse(secret, raw)
		// Invalid credentials remain the responsibility of the existing auth routes.
		if err != nil || claims.SessionID == "" || claims.UserID == 0 {
			c.Next()
			return
		}
		if (refresh && claims.TokenType != "refresh") || (!refresh && claims.TokenType != "access" && claims.TokenType != "") {
			c.Next()
			return
		}
		decision, err := reader.Access(c.Request.Context(), claims.UserID, claims.SessionID)
		if err != nil {
			response.ErrorWithCode(c, 503, "miniapp.access_unavailable", "access check unavailable")
			c.Abort()
			return
		}
		if decision.Reauthenticate {
			if refresh {
				c.SetCookie("deeix_chat_refresh_token", "", -1, "/api/v1/auth", "", c.Request.TLS != nil, true)
			}
			response.ErrorWithCode(c, 401, "miniapp.reauthentication_required", "please sign in again")
			c.Abort()
			return
		}
		if decision.MiniApp && !decision.Unlocked && !refresh && !todoAllowed(c.Request.Method, c.Request.URL.Path) {
			response.ErrorWithCode(c, 403, "miniapp.access_locked", "mini program access is locked")
			c.Abort()
			return
		}
		c.Next()
	}
}

func todoAllowed(method, path string) bool {
	switch method + " " + path {
	case "GET /api/v1/miniapp-entry/status", "GET /api/v1/miniapp-todo/snapshot", "GET /api/v1/miniapp-todo/tasks", "GET /api/v1/miniapp-todo/export", "POST /api/v1/miniapp-todo/sync", "POST /api/v1/miniapp-todo/feedback":
		return true
	}
	return false
}
