package wechat

import (
	"context"
	"regexp"
	"strings"

	domainwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechat"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

var (
	ErrInvalidInput = repository.ErrInvalidInput
	ErrDuplicate    = repository.ErrDuplicate
	ErrNotFound     = repository.ErrNotFound
)

type ActionOption struct {
	Key   string
	Label string
}

var actionOptions = []ActionOption{{Key: domainwechat.ActionIssueRegistrationCode, Label: "发放注册码"}}

var templatePlaceholderPattern = regexp.MustCompile(`\{\{[^{}]+\}\}`)

type AdminService struct {
	repo repository.WeChatAdminRepository
}

func NewAdminService(repo repository.WeChatAdminRepository) *AdminService {
	return &AdminService{repo: repo}
}

func (s *AdminService) Actions() []ActionOption { return append([]ActionOption(nil), actionOptions...) }

func (s *AdminService) ListRules(ctx context.Context, page, pageSize int) ([]domainwechat.KeywordRule, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	return s.repo.ListKeywordRules(ctx, (page-1)*pageSize, pageSize)
}

func (s *AdminService) SaveRule(ctx context.Context, item domainwechat.KeywordRule) error {
	item.Keyword = strings.TrimSpace(item.Keyword)
	item.Action = strings.TrimSpace(item.Action)
	if item.Keyword == "" || !isKnownAction(item.Action) || item.TemplateID == 0 {
		return repository.ErrInvalidInput
	}
	template, err := s.repo.GetReplyTemplate(ctx, item.TemplateID)
	if err != nil {
		return err
	}
	if item.Action == domainwechat.ActionIssueRegistrationCode &&
		(!template.Enabled || template.ResponseType != domainwechat.ResponseTypeText || !isValidRegistrationTemplate(template.Content)) {
		return repository.ErrInvalidInput
	}
	if item.ID == 0 {
		if !item.Enabled {
			item.Enabled = true
		}
		return s.repo.CreateKeywordRule(ctx, &item)
	}
	return s.repo.UpdateKeywordRule(ctx, item.ID, item.Keyword, item.Action, item.TemplateID)
}

func (s *AdminService) SetRuleEnabled(ctx context.Context, id uint, enabled bool) error {
	if id == 0 {
		return repository.ErrInvalidInput
	}
	return s.repo.SetKeywordRuleEnabled(ctx, id, enabled)
}

func (s *AdminService) ListTemplates(ctx context.Context, page, pageSize int) ([]domainwechat.ReplyTemplate, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	return s.repo.ListReplyTemplates(ctx, (page-1)*pageSize, pageSize)
}

func (s *AdminService) SaveTemplate(ctx context.Context, item domainwechat.ReplyTemplate) error {
	item.Name = strings.TrimSpace(item.Name)
	item.ResponseType = strings.TrimSpace(item.ResponseType)
	if item.Name == "" || item.ResponseType != domainwechat.ResponseTypeText || strings.TrimSpace(item.Content) == "" {
		return repository.ErrInvalidInput
	}
	if !isValidRegistrationTemplate(item.Content) {
		return repository.ErrInvalidInput
	}
	if item.ID == 0 {
		if !item.Enabled {
			item.Enabled = true
		}
		return s.repo.CreateReplyTemplate(ctx, &item)
	}
	return s.repo.UpdateReplyTemplate(ctx, item.ID, item.Name, item.ResponseType, item.Content)
}

func isValidRegistrationTemplate(content string) bool {
	placeholders := templatePlaceholderPattern.FindAllString(content, -1)
	for _, placeholder := range placeholders {
		if placeholder != "{{CODE}}" && placeholder != "{{REGISTER_LINK}}" {
			return false
		}
	}
	return true
}

func (s *AdminService) SetTemplateEnabled(ctx context.Context, id uint, enabled bool) error {
	if id == 0 {
		return repository.ErrInvalidInput
	}
	return s.repo.SetReplyTemplateEnabled(ctx, id, enabled)
}

func (s *AdminService) ListIssuances(ctx context.Context, page, pageSize int, query string) ([]domainwechat.IssuanceRecord, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	return s.repo.ListIssuanceRecords(ctx, (page-1)*pageSize, pageSize, query)
}

func (s *AdminService) ListLogs(ctx context.Context, page, pageSize int, result, action, query string) ([]domainwechat.InvocationLog, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	return s.repo.ListInvocationLogs(ctx, (page-1)*pageSize, pageSize, result, action, query)
}

func (s *AdminService) Stats(ctx context.Context) (domainwechat.Stats, error) {
	return s.repo.Stats(ctx)
}

func (s *AdminService) GetSettings(ctx context.Context) (domainwechat.Settings, error) {
	contact, err := s.repo.GetAdminContact(ctx)
	if err != nil {
		return domainwechat.Settings{}, err
	}
	return domainwechat.Settings{AdminContact: contact}, nil
}

func (s *AdminService) SaveSettings(ctx context.Context, settings domainwechat.Settings) error {
	settings.AdminContact = strings.TrimSpace(settings.AdminContact)
	if settings.AdminContact == "" || len(settings.AdminContact) > 128 {
		return repository.ErrInvalidInput
	}
	return s.repo.SetAdminContact(ctx, settings.AdminContact)
}

func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func isKnownAction(value string) bool {
	for _, item := range actionOptions {
		if item.Key == value {
			return true
		}
	}
	return false
}
