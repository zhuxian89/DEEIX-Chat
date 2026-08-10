package wechat

import (
	"context"
	"errors"

	domainregistrationcode "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/registrationcode"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// IssueRegistrationCode is idempotent for an OpenID and atomic for first issuance.
func (r *Repo) IssueRegistrationCode(ctx context.Context, openID, code string) (string, error) {
	var issued string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.WeChatRegistrationIssuance
		if err := tx.Where("open_id = ?", openID).First(&existing).Error; err == nil {
			var registration model.RegistrationCode
			if err := tx.First(&registration, existing.RegistrationCodeID).Error; err != nil {
				return err
			}
			issued = registration.Code
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		registration := model.RegistrationCode{
			Code:     code,
			CodeHint: code[len(code)-4:],
			Status:   domainregistrationcode.StatusActive,
		}
		if err := tx.Create(&registration).Error; err != nil {
			if dberror.IsUniqueConstraint(err) {
				return repository.ErrDuplicate
			}
			return err
		}
		issuance := model.WeChatRegistrationIssuance{OpenID: openID, RegistrationCodeID: registration.ID}
		if err := tx.Create(&issuance).Error; err != nil {
			if dberror.IsUniqueConstraint(err) {
				return repository.ErrDuplicate
			}
			return err
		}
		issued = registration.Code
		return nil
	})
	return issued, err
}

var _ repository.WeChatRegistrationRepository = (*Repo)(nil)
