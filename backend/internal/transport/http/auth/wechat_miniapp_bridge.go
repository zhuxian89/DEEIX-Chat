package auth

import (
	"net/http"
	"time"

	appauth "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/auth"
	"github.com/gin-gonic/gin"
)

// WriteRefreshTokenCookie writes the same HttpOnly rotating cookie used by the
// password login transport. Mini Program clients bridge it in memory only.
func WriteRefreshTokenCookie(c *gin.Context, service *appauth.Service, result *appauth.LoginResult) {
	if c == nil || result == nil || result.RefreshToken == "" || result.RefreshExpiresAt.IsZero() {
		return
	}
	maxAge := int(time.Until(result.RefreshExpiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	secure := isHTTPSRequest(c) || (service != nil && service.ShouldUseSecureCookies())
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    result.RefreshToken,
		Path:     "/api/v1/auth",
		Expires:  result.RefreshExpiresAt,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
