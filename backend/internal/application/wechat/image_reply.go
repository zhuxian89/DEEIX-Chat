package wechat

import (
	"context"
	"regexp"
	"strings"

	domainwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechat"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

var imageMediaIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,512}$`)

func validImageTemplate(responseType, content string) bool {
	return responseType == domainwechat.ResponseTypeImage && imageMediaIDPattern.MatchString(content)
}

func (s *Service) replyImage(ctx context.Context, repo repository.WeChatAdminRepository, openID string, result MessageResult, rule *domainwechat.KeywordRule) (MessageResult, error) {
	mediaID := strings.TrimSpace(rule.TemplateContent)
	if !rule.TemplateEnabled || !validImageTemplate(rule.TemplateType, mediaID) {
		return result, s.logFailure(ctx, repo, openID, result, "invalid_template", "image reply template must be enabled and contain a media ID")
	}
	if err := repo.CreateInvocationLog(ctx, &domainwechat.InvocationLog{
		OpenID: openID, Keyword: result.Keyword, Action: result.Action,
		TemplateID: result.TemplateID, Result: domainwechat.ResultHandled,
	}); err != nil {
		return result, err
	}
	result.ResponseType = domainwechat.ResponseTypeImage
	result.MediaID = mediaID
	result.Content = "[图片] " + rule.TemplateName
	result.Outcome = domainwechat.ResultHandled
	return result, nil
}
