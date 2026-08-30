package wechat

import (
	"context"
	"errors"
	"net/url"
	"strings"

	appregistrationcode "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/registrationcode"
	domainwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechat"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

const keyword = "13004"

const usedRegistrationCodeMessage = "\n该注册码已使用。"

type Service struct {
	repo    repository.WeChatRegistrationRepository
	baseURL string
}

func NewService(repo repository.WeChatRegistrationRepository) *Service { return &Service{repo: repo} }

// NewServiceWithBaseURL 携带站点根地址，用于在回复中渲染一键注册链接；
// baseURL 为空时回退到纯注册码文案（与旧版行为一致）。
func NewServiceWithBaseURL(repo repository.WeChatRegistrationRepository, baseURL string) *Service {
	return &Service{repo: repo, baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/")}
}

// registerLink 生成直达注册页并预填注册码的链接；无 baseURL 时返回空串。
func (s *Service) registerLink(code string) string {
	if s.baseURL == "" {
		return ""
	}
	return s.baseURL + "/login?code=" + url.QueryEscape(code)
}

func (s *Service) IssueRegistrationCode(ctx context.Context, openID string) (string, error) {
	openID = strings.TrimSpace(openID)
	if openID == "" {
		return "", repository.ErrInvalidInput
	}
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateCode()
		if err != nil {
			return "", err
		}
		issued, err := s.issue(ctx, openID, code)
		if err == repository.ErrDuplicate {
			continue
		}
		return issued, err
	}
	return "", repository.ErrDuplicate
}

func IsRegistrationKeyword(content string) bool { return strings.TrimSpace(content) == keyword }

type MessageResult struct {
	Matched            bool
	Content            string
	Keyword            string
	Action             string
	TemplateID         uint
	RegistrationCodeID uint
	Outcome            string
}

func (s *Service) HandleTextMessage(ctx context.Context, openID, content string) (MessageResult, error) {
	openID = strings.TrimSpace(openID)
	if openID == "" {
		return MessageResult{}, repository.ErrInvalidInput
	}
	keywordValue := strings.TrimSpace(content)
	adminRepo, configured := s.repo.(repository.WeChatAdminRepository)
	if !configured {
		if keywordValue != keyword {
			return MessageResult{}, nil
		}
		issued, err := s.issueDetailed(ctx, openID)
		if err != nil {
			return MessageResult{}, err
		}
		if issued.DeletedUser {
			return MessageResult{Matched: true, Content: deletedAccountMessage(domainwechat.DefaultAdminContact), Keyword: keyword, Action: domainwechat.ActionIssueRegistrationCode, Outcome: domainwechat.ResultReplayed}, nil
		}
		content := "你的专属注册码：" + issued.Code
		if issued.Used {
			content += usedRegistrationCodeMessage
		} else if link := s.registerLink(issued.Code); link != "" {
			content += "\n点此直接注册：" + link
		}
		outcome := domainwechat.ResultIssued
		if issued.Used {
			outcome = domainwechat.ResultReplayed
		}
		return MessageResult{Matched: true, Content: content, Keyword: keyword, Action: domainwechat.ActionIssueRegistrationCode, Outcome: outcome}, nil
	}

	rule, err := adminRepo.GetKeywordRule(ctx, keywordValue)
	if err != nil {
		if err == repository.ErrNotFound {
			if keywordValue != keyword {
				return MessageResult{}, nil
			}
			result := MessageResult{Matched: true, Keyword: keyword, Action: domainwechat.ActionIssueRegistrationCode}
			return s.issueConfiguredMessage(ctx, adminRepo, openID, result, "你的专属注册码：{{CODE}}")
		}
		return MessageResult{}, err
	}
	if !rule.Enabled {
		return MessageResult{}, nil
	}
	result := MessageResult{Matched: true, Keyword: rule.Keyword, Action: rule.Action, TemplateID: rule.TemplateID}
	if rule.Action != domainwechat.ActionIssueRegistrationCode {
		return result, s.logFailure(ctx, adminRepo, openID, result, "unsupported_action", "unsupported WeChat action")
	}
	if !rule.TemplateEnabled || rule.TemplateType != domainwechat.ResponseTypeText || !isValidRegistrationTemplate(rule.TemplateContent) {
		return result, s.logFailure(ctx, adminRepo, openID, result, "invalid_template", "registration code template must be enabled text and contain {{CODE}}")
	}
	return s.issueConfiguredMessage(ctx, adminRepo, openID, result, rule.TemplateContent)
}

func (s *Service) issueConfiguredMessage(ctx context.Context, repo repository.WeChatAdminRepository, openID string, result MessageResult, templateContent string) (MessageResult, error) {
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateCode()
		if err != nil {
			return result, err
		}
		log := &domainwechat.InvocationLog{OpenID: openID, Keyword: result.Keyword, Action: result.Action, TemplateID: result.TemplateID}
		issue, err := repo.IssueRegistrationCodeWithInvocationLog(ctx, openID, code, log)
		if err == repository.ErrDuplicate {
			continue
		}
		if err != nil {
			result.Outcome = domainwechat.ResultFailed
			logErr := repo.CreateInvocationLog(ctx, &domainwechat.InvocationLog{OpenID: openID, Keyword: result.Keyword, Action: result.Action, TemplateID: result.TemplateID, Result: domainwechat.ResultFailed, ErrorCode: "issue_failed", ErrorMessage: "registration code issuance failed"})
			if logErr != nil {
				return result, errors.Join(err, logErr)
			}
			return result, err
		}
		result.RegistrationCodeID = issue.RegistrationCodeID
		result.Outcome = log.Result
		if issue.DeletedUser {
			contact, err := repo.GetAdminContact(ctx)
			if err != nil {
				return result, err
			}
			result.Content = deletedAccountMessage(contact)
			return result, nil
		}
		result.Content = strings.ReplaceAll(templateContent, "{{CODE}}", issue.Code)
		result.Content = strings.ReplaceAll(result.Content, "{{REGISTER_LINK}}", s.registerLink(issue.Code))
		if issue.Used {
			result.Content += usedRegistrationCodeMessage
		} else if link := s.registerLink(issue.Code); link != "" && !strings.Contains(templateContent, "{{REGISTER_LINK}}") {
			result.Content += "\n点此直接注册：" + link
		}
		return result, nil
	}
	return result, repository.ErrDuplicate
}

func (s *Service) issue(ctx context.Context, openID, code string) (string, error) {
	if detailed, ok := s.repo.(repository.WeChatDetailedRegistrationRepository); ok {
		result, err := detailed.IssueRegistrationCodeDetailed(ctx, openID, code)
		return result.Code, err
	}
	return s.repo.IssueRegistrationCode(ctx, openID, code)
}

func (s *Service) issueDetailed(ctx context.Context, openID string) (domainwechat.IssueResult, error) {
	for attempt := 0; attempt < 5; attempt++ {
		code, err := generateCode()
		if err != nil {
			return domainwechat.IssueResult{}, err
		}
		if detailed, ok := s.repo.(repository.WeChatDetailedRegistrationRepository); ok {
			result, err := detailed.IssueRegistrationCodeDetailed(ctx, openID, code)
			if err == repository.ErrDuplicate {
				continue
			}
			return result, err
		}
		issued, err := s.repo.IssueRegistrationCode(ctx, openID, code)
		return domainwechat.IssueResult{Code: issued, Created: true}, err
	}
	return domainwechat.IssueResult{}, repository.ErrDuplicate
}

func (s *Service) logFailure(ctx context.Context, repo repository.WeChatAdminRepository, openID string, result MessageResult, code, message string) error {
	result.Outcome = domainwechat.ResultFailed
	if err := repo.CreateInvocationLog(ctx, &domainwechat.InvocationLog{OpenID: openID, Keyword: result.Keyword, Action: result.Action, TemplateID: result.TemplateID, Result: domainwechat.ResultFailed, ErrorCode: code, ErrorMessage: message}); err != nil {
		return err
	}
	return repository.ErrInvalidInput
}

func deletedAccountMessage(contact string) string {
	contact = strings.TrimSpace(contact)
	if contact == "" {
		contact = domainwechat.DefaultAdminContact
	}
	return "该微信曾注册的账号已被注销，无法自动领取新注册码。\n如需重新注册，请添加管理员微信：" + contact + "，并说明“申请新注册码”。"
}

// generateCode 复用 registrationcode 包的统一生成函数，保证注册码格式单一来源（REG- 前缀）。
func generateCode() (string, error) { return appregistrationcode.GenerateCode() }
