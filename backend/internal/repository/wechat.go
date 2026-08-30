package repository

import (
	"context"

	domainwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechat"
)

// WeChatRegistrationRepository issues one registration code per official-account OpenID.
type WeChatRegistrationRepository interface {
	IssueRegistrationCode(ctx context.Context, openID, code string) (string, error)
}

type WeChatDetailedRegistrationRepository interface {
	IssueRegistrationCodeDetailed(ctx context.Context, openID, code string) (domainwechat.IssueResult, error)
}

type WeChatAdminRepository interface {
	WeChatRegistrationRepository
	GetKeywordRule(ctx context.Context, keyword string) (*domainwechat.KeywordRule, error)
	IssueRegistrationCodeWithInvocationLog(ctx context.Context, openID, code string, log *domainwechat.InvocationLog) (domainwechat.IssueResult, error)
	ListKeywordRules(ctx context.Context, offset, limit int) ([]domainwechat.KeywordRule, int64, error)
	CreateKeywordRule(ctx context.Context, item *domainwechat.KeywordRule) error
	UpdateKeywordRule(ctx context.Context, id uint, keyword, action string, templateID uint) error
	SetKeywordRuleEnabled(ctx context.Context, id uint, enabled bool) error
	ListReplyTemplates(ctx context.Context, offset, limit int) ([]domainwechat.ReplyTemplate, int64, error)
	GetReplyTemplate(ctx context.Context, id uint) (*domainwechat.ReplyTemplate, error)
	CreateReplyTemplate(ctx context.Context, item *domainwechat.ReplyTemplate) error
	UpdateReplyTemplate(ctx context.Context, id uint, name, responseType, content string) error
	SetReplyTemplateEnabled(ctx context.Context, id uint, enabled bool) error
	ListIssuanceRecords(ctx context.Context, offset, limit int, query string) ([]domainwechat.IssuanceRecord, int64, error)
	ListInvocationLogs(ctx context.Context, offset, limit int, result, action, query string) ([]domainwechat.InvocationLog, int64, error)
	CreateInvocationLog(ctx context.Context, item *domainwechat.InvocationLog) error
	Stats(ctx context.Context) (domainwechat.Stats, error)
	GetAdminContact(ctx context.Context) (string, error)
	SetAdminContact(ctx context.Context, contact string) error
}
