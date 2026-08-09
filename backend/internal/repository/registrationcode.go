package repository

import (
	"context"

	domainregistrationcode "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/registrationcode"
)

type RegistrationCodeListFilter struct {
	Status string
}

type RegistrationCodeRepository interface {
	Create(ctx context.Context, item *domainregistrationcode.RegistrationCode) error
	List(ctx context.Context, filter RegistrationCodeListFilter, offset int, limit int) ([]domainregistrationcode.RegistrationCode, int64, error)
	GetByID(ctx context.Context, id uint) (*domainregistrationcode.RegistrationCode, error)
	DeleteUnused(ctx context.Context, id uint) error
}
