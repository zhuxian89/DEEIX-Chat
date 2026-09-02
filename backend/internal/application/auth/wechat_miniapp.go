package auth

import (
	"context"
	"strings"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
)

const WeChatMiniAppUserAgentPrefix = "DEEIX-WeChat-MiniApp"

type miniAppSessionRepository interface {
	RevokeActiveMiniAppSessions(ctx context.Context, userID uint, userAgentPrefix string, now time.Time) error
}

// IssueWeChatMiniAppLogin signs a normal DEEIX session after the Mini Program
// service has authoritatively resolved its OpenID binding.
func (s *Service) IssueWeChatMiniAppLogin(
	ctx context.Context,
	userID uint,
	created bool,
	requestID string,
	auditCtx requestmeta.SessionAuditContext,
) (*LoginResult, error) {
	item, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if item.Status != domainuser.StatusActive {
		return nil, ErrInvalidCredentials
	}
	now := time.Now()
	if repo, ok := s.repo.(miniAppSessionRepository); ok {
		if err = repo.RevokeActiveMiniAppSessions(ctx, userID, WeChatMiniAppUserAgentPrefix, now); err != nil {
			return nil, err
		}
	}
	normalizedAuditCtx := s.resolveSessionAuditContext(ctx, auditCtx)
	if !strings.HasPrefix(normalizedAuditCtx.UserAgent, WeChatMiniAppUserAgentPrefix) {
		normalizedAuditCtx.UserAgent = WeChatMiniAppUserAgentPrefix
	}
	result, err := s.issueLoginResult(ctx, item, normalizedAuditCtx, now)
	if err != nil {
		return nil, err
	}
	eventType := "wechat_miniapp_login"
	if created {
		eventType = "wechat_miniapp_register"
	}
	s.RecordAuthEvent(
		ctx,
		userID,
		requestID,
		eventType,
		"success",
		"",
		normalizedAuditCtx.ClientIP,
		normalizedAuditCtx.UserAgent,
		marshalAuthEventDetail(map[string]interface{}{"session_id": result.SessionID}),
	)
	if created && s.telegramNotifier != nil {
		s.telegramNotifier.NotifyRegistration(userID, result.User.Username, "")
	}
	return result, nil
}

// RecordWeChatMiniAppIdentityConflict records a security-relevant mismatch
// without retaining OpenID, UnionID, the one-time code, or other WeChat secrets.
func (s *Service) RecordWeChatMiniAppIdentityConflict(
	ctx context.Context,
	userID uint,
	requestID string,
	auditCtx requestmeta.SessionAuditContext,
) {
	if s == nil || userID == 0 {
		return
	}
	normalizedAuditCtx := s.resolveSessionAuditContext(ctx, auditCtx)
	if !strings.HasPrefix(normalizedAuditCtx.UserAgent, WeChatMiniAppUserAgentPrefix) {
		normalizedAuditCtx.UserAgent = WeChatMiniAppUserAgentPrefix
	}
	s.RecordAuthEvent(
		ctx,
		userID,
		requestID,
		"wechat_miniapp_identity_conflict",
		"failed",
		"identity_conflict",
		normalizedAuditCtx.ClientIP,
		normalizedAuditCtx.UserAgent,
		"{}",
	)
}
