package wechat

import (
	"context"
	"errors"
	"testing"

	domainwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechat"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

type imageReplyRepo struct {
	repository.WeChatAdminRepository
	rule           domainwechat.KeywordRule
	template       domainwechat.ReplyTemplate
	logs           []domainwechat.InvocationLog
	logErr         error
	codeCalls      int
	templateWrites int
	ruleWrites     int
}

func (r *imageReplyRepo) GetKeywordRule(_ context.Context, keyword string) (*domainwechat.KeywordRule, error) {
	if keyword != r.rule.Keyword {
		return nil, repository.ErrNotFound
	}
	return &r.rule, nil
}

func (r *imageReplyRepo) CreateInvocationLog(_ context.Context, log *domainwechat.InvocationLog) error {
	if r.logErr != nil {
		return r.logErr
	}
	r.logs = append(r.logs, *log)
	return nil
}

func (r *imageReplyRepo) IssueRegistrationCodeWithInvocationLog(_ context.Context, _ string, _ string, log *domainwechat.InvocationLog) (domainwechat.IssueResult, error) {
	r.codeCalls++
	log.Result = domainwechat.ResultIssued
	return domainwechat.IssueResult{Code: "REG-EXISTING", Created: true}, nil
}

func (r *imageReplyRepo) GetReplyTemplate(context.Context, uint) (*domainwechat.ReplyTemplate, error) {
	return &r.template, nil
}

func (r *imageReplyRepo) CreateReplyTemplate(_ context.Context, item *domainwechat.ReplyTemplate) error {
	r.templateWrites++
	r.template = *item
	return nil
}

func (r *imageReplyRepo) UpdateReplyTemplate(_ context.Context, id uint, name, responseType, content string) error {
	r.templateWrites++
	r.template = domainwechat.ReplyTemplate{ID: id, Name: name, ResponseType: responseType, Content: content, Enabled: r.template.Enabled}
	return nil
}

func (r *imageReplyRepo) CreateKeywordRule(_ context.Context, item *domainwechat.KeywordRule) error {
	r.ruleWrites++
	r.rule = *item
	return nil
}

func imageRule() domainwechat.KeywordRule {
	return domainwechat.KeywordRule{ID: 2, Keyword: "小程序", Action: domainwechat.ActionReplyImage, TemplateID: 3,
		TemplateName: "小程序码", TemplateType: domainwechat.ResponseTypeImage, TemplateContent: "permanent-image_123",
		TemplateEnabled: true, Enabled: true}
}

func TestImageReplyDoesNotIssueRegistrationCode(t *testing.T) {
	repo := &imageReplyRepo{rule: imageRule()}
	service := NewServiceWithBaseURL(repo, "https://example.com")
	for i := 0; i < 2; i++ {
		result, err := service.HandleTextMessage(t.Context(), "openid", " 小程序\n")
		if err != nil {
			t.Fatal(err)
		}
		if !result.Matched || result.ResponseType != "image" || result.MediaID != "permanent-image_123" || result.Content != "[图片] 小程序码" || result.Outcome != domainwechat.ResultHandled {
			t.Fatalf("unexpected reply: %+v", result)
		}
	}
	if repo.codeCalls != 0 || len(repo.logs) != 2 {
		t.Fatalf("codes=%d logs=%d", repo.codeCalls, len(repo.logs))
	}
	for _, log := range repo.logs {
		if log.RegistrationCodeID != 0 || log.Result != domainwechat.ResultHandled || log.Action != domainwechat.ActionReplyImage || log.OpenID != "openid" {
			t.Fatalf("unexpected image invocation: %+v", log)
		}
	}
	result, err := service.HandleTextMessage(t.Context(), "openid", "13004")
	if err != nil || !result.Matched || result.MediaID != "" || repo.codeCalls != 1 {
		t.Fatalf("registration fallback broken: result=%+v err=%v calls=%d", result, err, repo.codeCalls)
	}
}

func TestImageReplyRequiresExactEnabledKeywordAndValidTemplate(t *testing.T) {
	for _, name := range []string{"unmatched", "disabled rule", "disabled template", "wrong type", "empty media", "url instead of media", "log failure"} {
		t.Run(name, func(t *testing.T) {
			repo := &imageReplyRepo{rule: imageRule()}
			keyword := "小程序"
			wantErr := true
			switch name {
			case "unmatched":
				keyword = "打开小程序"
				wantErr = false
			case "disabled rule":
				repo.rule.Enabled = false
				wantErr = false
			case "disabled template":
				repo.rule.TemplateEnabled = false
			case "wrong type":
				repo.rule.TemplateType = "text"
			case "empty media":
				repo.rule.TemplateContent = " "
			case "url instead of media":
				repo.rule.TemplateContent = "https://example.com/code.png"
			case "log failure":
				repo.logErr = errors.New("database unavailable")
			}
			result, err := NewService(repo).HandleTextMessage(t.Context(), "openid", keyword)
			if (err != nil) != wantErr || result.MediaID != "" || repo.codeCalls != 0 {
				t.Fatalf("unexpected result=%+v err=%v code calls=%d", result, err, repo.codeCalls)
			}
			if !wantErr && result.Matched {
				t.Fatal("unmatched or disabled keyword was handled")
			}
		})
	}
}

func TestAdminImageTemplateAndRuleValidation(t *testing.T) {
	repo := &imageReplyRepo{}
	admin := NewAdminService(repo)
	if err := admin.SaveTemplate(t.Context(), domainwechat.ReplyTemplate{Name: "小程序码", ResponseType: "image", Content: " permanent-image_123\n"}); err != nil {
		t.Fatal(err)
	}
	if repo.template.Content != "permanent-image_123" || !repo.template.Enabled {
		t.Fatalf("unexpected template: %+v", repo.template)
	}
	if err := admin.SaveRule(t.Context(), domainwechat.KeywordRule{Keyword: "小程序", Action: domainwechat.ActionReplyImage, TemplateID: 3}); err != nil {
		t.Fatal(err)
	}
	if repo.ruleWrites != 1 || repo.rule.Action != domainwechat.ActionReplyImage {
		t.Fatalf("unexpected rule: %+v", repo.rule)
	}
	if err := admin.SaveRule(t.Context(), domainwechat.KeywordRule{Keyword: "13004", Action: domainwechat.ActionIssueRegistrationCode, TemplateID: 3}); !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("registration action accepted image template: %v", err)
	}
	for _, content := range []string{"", "https://example.com/code.png", "{{CODE}}", "<Image>bad</Image>", "media id"} {
		if err := admin.SaveTemplate(t.Context(), domainwechat.ReplyTemplate{Name: "bad", ResponseType: "image", Content: content}); !errors.Is(err, repository.ErrInvalidInput) {
			t.Fatalf("invalid image content %q accepted: %v", content, err)
		}
	}
	if repo.templateWrites != 1 {
		t.Fatalf("invalid saves wrote templates: %d", repo.templateWrites)
	}
	repo.template = domainwechat.ReplyTemplate{ID: 1, Name: "注册码", ResponseType: "text", Content: "{{CODE}}", Enabled: true}
	if err := admin.SaveTemplate(t.Context(), domainwechat.ReplyTemplate{ID: 1, Name: "改成图片", ResponseType: "image", Content: "permanent-image_123"}); !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("existing template type changed: %v", err)
	}
	if err := admin.SaveTemplate(t.Context(), domainwechat.ReplyTemplate{ID: 1, Name: "注册码", ResponseType: "text", Content: "你的注册码：{{CODE}}"}); err != nil {
		t.Fatal(err)
	}
	if err := admin.SaveRule(t.Context(), domainwechat.KeywordRule{Keyword: "小程序", Action: domainwechat.ActionReplyImage, TemplateID: 1}); !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("image action accepted text template: %v", err)
	}
}
