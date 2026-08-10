package wechat

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

const keyword = "13003"

type Service struct {
	repo repository.WeChatRegistrationRepository
}

func NewService(repo repository.WeChatRegistrationRepository) *Service { return &Service{repo: repo} }

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
		issued, err := s.repo.IssueRegistrationCode(ctx, openID, code)
		if err == repository.ErrDuplicate {
			continue
		}
		return issued, err
	}
	return "", repository.ErrDuplicate
}

func IsRegistrationKeyword(content string) bool { return strings.TrimSpace(content) == keyword }

func generateCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	buf := make([]byte, len(raw))
	for i := range raw {
		buf[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return fmt.Sprintf("%s-%s-%s-%s", string(buf[:4]), string(buf[4:8]), string(buf[8:12]), string(buf[12:])), nil
}
