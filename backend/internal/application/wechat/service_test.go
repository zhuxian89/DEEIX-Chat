package wechat

import (
	"context"
	"testing"

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
	if !IsRegistrationKeyword(" 13003\n") {
		t.Fatal("expected keyword to be trimmed")
	}
	if IsRegistrationKeyword("13003abc") {
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
