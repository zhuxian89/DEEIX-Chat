package wechat

import (
	"context"
	"testing"

	domainwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechat"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

type fakeRepository struct {
	code  string
	calls int
}

func (r *fakeRepository) IssueRegistrationCode(_ context.Context, _ string, code string) (string, error) {
	r.calls++
	if r.code == "" {
		r.code = code
	}
	return r.code, nil
}

var _ repository.WeChatRegistrationRepository = (*fakeRepository)(nil)

func TestIsRegistrationKeyword(t *testing.T) {
	if !IsRegistrationKeyword(" 13004\n") {
		t.Fatal("expected keyword to be trimmed")
	}
	if IsRegistrationKeyword("13004abc") {
		t.Fatal("unexpected partial keyword match")
	}
}

func TestIssueRegistrationCodeRejectsEmptyOpenID(t *testing.T) {
	_, err := NewService(&fakeRepository{}).IssueRegistrationCode(context.Background(), " ")
	if err != repository.ErrInvalidInput {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func TestIssueRegistrationCodeReturnsRepositoryValue(t *testing.T) {
	repo := &fakeRepository{}
	code, err := NewService(repo).IssueRegistrationCode(context.Background(), "openid-1")
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	if code == "" || repo.calls != 1 {
		t.Fatalf("code=%q calls=%d", code, repo.calls)
	}
}

type detailedFakeRepository struct {
	result domainwechat.IssueResult
}

func (r *detailedFakeRepository) IssueRegistrationCode(_ context.Context, _ string, _ string) (string, error) {
	return r.result.Code, nil
}

func (r *detailedFakeRepository) IssueRegistrationCodeDetailed(_ context.Context, _ string, _ string) (domainwechat.IssueResult, error) {
	return r.result, nil
}

func TestHandleTextMessageUsesFixedMessagesForUsedCodeStates(t *testing.T) {
	tests := []struct {
		name    string
		result  domainwechat.IssueResult
		content string
	}{
		{name: "existing user", result: domainwechat.IssueResult{Code: "REG-USED", Used: true}, content: "你的专属注册码：REG-USED\n该注册码已使用。"},
		{name: "deleted user", result: domainwechat.IssueResult{Code: "REG-DELETED", Used: true, DeletedUser: true}, content: "该微信曾注册的账号已被注销，无法自动领取新注册码。\n如需重新注册，请添加管理员微信：zhuxian1005，并说明“申请新注册码”。"},
		{name: "never registered", result: domainwechat.IssueResult{Code: "REG-NEW", Created: true}, content: "你的专属注册码：REG-NEW"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewService(&detailedFakeRepository{result: tt.result}).HandleTextMessage(context.Background(), "openid", "13004")
			if err != nil {
				t.Fatalf("HandleTextMessage() error = %v", err)
			}
			if result.Content != tt.content {
				t.Fatalf("content = %q, want %q", result.Content, tt.content)
			}
		})
	}
}

func TestDeletedAccountMessageUsesConfiguredContact(t *testing.T) {
	got := deletedAccountMessage(" custom-admin ")
	want := "该微信曾注册的账号已被注销，无法自动领取新注册码。\n如需重新注册，请添加管理员微信：custom-admin，并说明“申请新注册码”。"
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}
