package wechatminiapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	appauth "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/auth"
	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	domainwechatminiapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechatminiapp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/conv"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/requestmeta"
	"github.com/google/uuid"
)

var (
	ErrDisabled           = errors.New("wechat mini program login is disabled")
	ErrInvalidCode        = errors.New("wechat mini program code is invalid")
	ErrAccountUnavailable = errors.New("wechat mini program account is unavailable")
	ErrIdentityConflict   = errors.New("wechat mini program identity conflict")
)

type CodeExchanger interface {
	Exchange(ctx context.Context, appID, appSecret, code string) (domainwechatminiapp.Identity, error)
}

type Repository interface {
	GetMiniAppBinding(ctx context.Context, appID, openID string) (*domainwechatminiapp.Binding, error)
	TouchMiniAppBinding(ctx context.Context, appID, openID, unionID string, now time.Time) (*domainwechatminiapp.Binding, error)
	CreateMiniAppUserAndBinding(ctx context.Context, user *domainuser.User, credential domainuser.Credential, binding *domainwechatminiapp.Binding, invitationCodeLength int) error
}

type LoginIssuer interface {
	IssueWeChatMiniAppLogin(ctx context.Context, userID uint, created bool, requestID string, auditCtx requestmeta.SessionAuditContext) (*appauth.LoginResult, error)
	RecordWeChatMiniAppIdentityConflict(ctx context.Context, userID uint, requestID string, auditCtx requestmeta.SessionAuditContext)
}

type LoginResult struct {
	Auth    *appauth.LoginResult
	Created bool
	Presets domainwechatminiapp.Presets
}

type Service struct {
	cfg       *config.Runtime
	repo      Repository
	exchanger CodeExchanger
	issuer    LoginIssuer
	now       func() time.Time
}

func NewService(cfg *config.Runtime, repo Repository, exchanger CodeExchanger, issuer LoginIssuer) *Service {
	return &Service{cfg: cfg, repo: repo, exchanger: exchanger, issuer: issuer, now: time.Now}
}

func (s *Service) Login(ctx context.Context, code, requestID string, auditCtx requestmeta.SessionAuditContext) (*LoginResult, error) {
	if s == nil || s.cfg == nil || s.repo == nil || s.exchanger == nil || s.issuer == nil {
		return nil, errors.New("wechat mini program login is unavailable")
	}
	cfg := s.cfg.Snapshot()
	if !cfg.WeChatMiniAppEnabled {
		return nil, ErrDisabled
	}
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 256 {
		return nil, ErrInvalidCode
	}
	identity, err := s.exchanger.Exchange(ctx, cfg.WeChatMiniAppAppID, cfg.WeChatMiniAppAppSecret, code)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCode, err)
	}
	identity.OpenID = strings.TrimSpace(identity.OpenID)
	identity.UnionID = strings.TrimSpace(identity.UnionID)
	if identity.OpenID == "" || len(identity.OpenID) > 128 || len(identity.UnionID) > 128 {
		return nil, ErrInvalidCode
	}

	now := s.now()
	binding, err := s.repo.TouchMiniAppBinding(ctx, cfg.WeChatMiniAppAppID, identity.OpenID, identity.UnionID, now)
	created := false
	if errors.Is(err, repository.ErrNotFound) {
		created = true
		binding, err = s.createUserAndBinding(ctx, cfg, identity, now)
		if errors.Is(err, repository.ErrDuplicate) || errors.Is(err, repository.ErrDuplicateUsername) {
			created = false
			binding, err = s.repo.TouchMiniAppBinding(ctx, cfg.WeChatMiniAppAppID, identity.OpenID, identity.UnionID, now)
		}
	}
	if errors.Is(err, repository.ErrConflict) {
		if existing, lookupErr := s.repo.GetMiniAppBinding(ctx, cfg.WeChatMiniAppAppID, identity.OpenID); lookupErr == nil && existing != nil && existing.UserID != 0 {
			s.issuer.RecordWeChatMiniAppIdentityConflict(ctx, existing.UserID, requestID, auditCtx)
		}
		return nil, ErrIdentityConflict
	}
	if err != nil {
		return nil, err
	}
	if binding == nil || binding.UserID == 0 || binding.RevokedAt != nil {
		return nil, ErrAccountUnavailable
	}

	authResult, err := s.issuer.IssueWeChatMiniAppLogin(ctx, binding.UserID, created, requestID, auditCtx)
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, appauth.ErrInvalidCredentials) {
		return nil, ErrAccountUnavailable
	}
	if err != nil {
		return nil, err
	}
	return &LoginResult{
		Auth:    authResult,
		Created: created,
		Presets: domainwechatminiapp.Presets{
			ChatModel:  strings.TrimSpace(cfg.WeChatMiniAppDefaultChatModel),
			ImageModel: strings.TrimSpace(cfg.WeChatMiniAppDefaultImageModel),
		},
	}, nil
}

func (s *Service) createUserAndBinding(ctx context.Context, cfg config.Config, identity domainwechatminiapp.Identity, now time.Time) (*domainwechatminiapp.Binding, error) {
	digest := sha256.Sum256([]byte(cfg.WeChatMiniAppAppID + "\x00" + identity.OpenID))
	suffix := hex.EncodeToString(digest[:])
	baseUsername := "wx-" + suffix[:12]
	for attempt := 0; attempt < 20; attempt++ {
		username := baseUsername
		if attempt > 0 {
			username = fmt.Sprintf("%s-%d", baseUsername, attempt+1)
		}
		user := &domainuser.User{
			PublicID:    conv.NormalizePublicID(uuid.NewString()),
			Username:    username,
			DisplayName: "微信用户 " + suffix[:6],
			Email:       "",
			Role:        domainuser.RoleUser,
			Status:      domainuser.StatusActive,
			Timezone:    "Asia/Shanghai",
			Locale:      "zh-CN",
		}
		credential := domainuser.Credential{
			PasswordHash:    "",
			PasswordAlgo:    "bcrypt",
			PasswordEnabled: false,
			PasswordOrigin:  domainuser.PasswordOriginSSOPlaceholder,
		}
		binding := &domainwechatminiapp.Binding{
			AppID:       cfg.WeChatMiniAppAppID,
			OpenID:      identity.OpenID,
			UnionID:     identity.UnionID,
			LastLoginAt: now,
		}
		if identity.UnionID != "" {
			observedAt := now
			binding.UnionIDObservedAt = &observedAt
		}
		err := s.repo.CreateMiniAppUserAndBinding(ctx, user, credential, binding, cfg.InvitationCodeLength)
		if errors.Is(err, repository.ErrDuplicateUsername) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return binding, nil
	}
	return nil, appauth.ErrUsernameTaken
}
