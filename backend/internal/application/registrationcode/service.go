package registrationcode

import (
	"context"
	"crypto/rand"
	"fmt"

	domainregistrationcode "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/registrationcode"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

const (
	defaultQuantity = 1
	maxQuantity     = 100
	codeLength      = 16
)

type Service struct {
	repo repository.RegistrationCodeRepository
}

func NewService(repo repository.RegistrationCodeRepository) *Service { return &Service{repo: repo} }

type CodeView struct {
	domainregistrationcode.RegistrationCode
}

func (s *Service) Create(ctx context.Context, actorUserID uint, quantity int) ([]CodeView, error) {
	if quantity == 0 {
		quantity = defaultQuantity
	}
	if actorUserID == 0 || quantity < 1 || quantity > maxQuantity {
		return nil, repository.ErrInvalidInput
	}
	result := make([]CodeView, 0, quantity)
	for i := 0; i < quantity; i++ {
		created := false
		for attempt := 0; attempt < 5; attempt++ {
			code, err := generateCode()
			if err != nil {
				return nil, err
			}
			item := &domainregistrationcode.RegistrationCode{Code: code, CodeHint: code[len(code)-4:], Status: domainregistrationcode.StatusActive, CreatedByUserID: actorUserID}
			if err = s.repo.Create(ctx, item); err != nil {
				if err == repository.ErrDuplicate {
					continue
				}
				return nil, err
			}
			result = append(result, CodeView{RegistrationCode: *item})
			created = true
			break
		}
		if !created {
			return nil, repository.ErrDuplicate
		}
	}
	return result, nil
}

func (s *Service) List(ctx context.Context, page, pageSize int, status string) ([]CodeView, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	items, total, err := s.repo.List(ctx, repository.RegistrationCodeListFilter{Status: status}, (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]CodeView, 0, len(items))
	for _, item := range items {
		result = append(result, CodeView{RegistrationCode: item})
	}
	return result, total, nil
}

func (s *Service) Delete(ctx context.Context, id uint) error { return s.repo.DeleteUnused(ctx, id) }

// registrationCodePrefix 是注册码的固定前缀，用于与邀请码（INV-）区分。
const registrationCodePrefix = "REG-"

func generateCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, codeLength)
	raw := make([]byte, codeLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return fmt.Sprintf("%s%s", registrationCodePrefix, string(buf)), nil
}
