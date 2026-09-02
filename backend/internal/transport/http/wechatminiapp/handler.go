package wechatminiapp

import (
	"errors"
	"net/http"
	"strings"

	appauth "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/auth"
	appwechatminiapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/wechatminiapp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	authhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/auth"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service     *appwechatminiapp.Service
	authService *appauth.Service
}

func NewHandler(service *appwechatminiapp.Service, authService *appauth.Service) *Handler {
	return &Handler{service: service, authService: authService}
}

// Login godoc
// @Summary 微信小程序一键登录
// @Description 使用 wx.login 一次性 code 登录；首次使用时快捷创建标准 DEEIX 用户。UnionID 仅留档，不用于账号融合。
// @Tags auth
// @Accept json
// @Produce json
// @Param body body LoginRequest true "微信小程序登录参数"
// @Success 200 {object} LoginResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 401 {object} ErrorDoc
// @Failure 403 {object} ErrorDoc
// @Failure 429 {object} ErrorDoc
// @Failure 503 {object} ErrorDoc
// @Router /auth/wechat-miniapp/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	auditCtx := middleware.ResolveSessionAuditContext(c)
	originalUserAgent := strings.TrimSpace(auditCtx.UserAgent)
	auditCtx.UserAgent = appauth.WeChatMiniAppUserAgentPrefix
	if originalUserAgent != "" {
		auditCtx.UserAgent += " " + originalUserAgent
	}
	result, err := h.service.Login(c.Request.Context(), req.Code, middleware.MustRequestID(c), auditCtx)
	if err != nil {
		switch {
		case errors.Is(err, appwechatminiapp.ErrDisabled):
			response.ErrorWithCode(c, http.StatusServiceUnavailable, "auth.wechat_miniapp_disabled", "wechat mini program login is unavailable")
		case errors.Is(err, appwechatminiapp.ErrInvalidCode):
			response.ErrorWithCode(c, http.StatusUnauthorized, "auth.wechat_miniapp_invalid_code", "wechat login expired, please retry")
		case errors.Is(err, appwechatminiapp.ErrAccountUnavailable):
			response.ErrorWithCode(c, http.StatusForbidden, "auth.wechat_miniapp_account_unavailable", "account is unavailable")
		case errors.Is(err, appwechatminiapp.ErrIdentityConflict):
			response.ErrorWithCode(c, http.StatusForbidden, "auth.wechat_miniapp_identity_conflict", "wechat identity verification failed")
		default:
			response.ErrorWithCode(c, http.StatusInternalServerError, "auth.wechat_miniapp_login_failed", "wechat mini program login failed")
		}
		return
	}
	authhttp.WriteRefreshTokenCookie(c, h.authService, result.Auth)
	response.Success(c, LoginResponse{
		Auth: MiniAppAuthResponse{
			AccessToken:      result.Auth.AccessToken,
			SessionID:        result.Auth.SessionID,
			ExpiresAt:        result.Auth.ExpiresAt,
			RefreshExpiresAt: result.Auth.RefreshExpiresAt,
			User: MiniAppUserResponse{
				ID:                   result.Auth.User.ID,
				PublicID:             result.Auth.User.PublicID,
				Username:             result.Auth.User.Username,
				DisplayName:          result.Auth.User.DisplayName,
				AvatarURL:            result.Auth.User.AvatarURL,
				SubscriptionTier:     result.Auth.User.SubscriptionTier,
				SubscriptionPlanName: result.Auth.User.SubscriptionPlanName,
				SubscriptionStatus:   result.Auth.User.SubscriptionStatus,
			},
		},
		Created: result.Created,
		Presets: MiniAppPresetsResponse{ChatModel: result.Presets.ChatModel, ImageModel: result.Presets.ImageModel},
	})
}
