package registrationcode

import (
	"context"
	"strings"

	domainregistrationcode "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/registrationcode"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func translateError(err error) error {
	if err == nil {
		return nil
	}
	if dberror.IsRecordNotFound(err) {
		return repository.ErrNotFound
	}
	if dberror.IsUniqueConstraint(err) {
		return repository.ErrDuplicate
	}
	return err
}

func toDomain(item model.RegistrationCode) domainregistrationcode.RegistrationCode {
	return domainregistrationcode.RegistrationCode{ID: item.ID, Code: item.Code, CodeHint: item.CodeHint, Status: item.Status, UsedByUserID: item.UsedByUserID, UsedAt: item.UsedAt, CreatedByUserID: item.CreatedByUserID, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func (r *Repo) Create(ctx context.Context, item *domainregistrationcode.RegistrationCode) error {
	dbItem := &model.RegistrationCode{Code: strings.TrimSpace(item.Code), CodeHint: item.CodeHint, Status: item.Status, UsedByUserID: item.UsedByUserID, UsedAt: item.UsedAt, CreatedByUserID: item.CreatedByUserID}
	if err := r.db.WithContext(ctx).Create(dbItem).Error; err != nil {
		return translateError(err)
	}
	item.ID, item.CreatedAt, item.UpdatedAt = dbItem.ID, dbItem.CreatedAt, dbItem.UpdatedAt
	return nil
}

func (r *Repo) List(ctx context.Context, filter repository.RegistrationCodeListFilter, offset int, limit int) ([]domainregistrationcode.RegistrationCode, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.RegistrationCode{})
	if status := strings.TrimSpace(filter.Status); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, translateError(err)
	}
	items := make([]model.RegistrationCode, 0)
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, translateError(err)
	}
	result := make([]domainregistrationcode.RegistrationCode, 0, len(items))
	for _, item := range items {
		result = append(result, toDomain(item))
	}
	return result, total, nil
}

func (r *Repo) GetByID(ctx context.Context, id uint) (*domainregistrationcode.RegistrationCode, error) {
	var item model.RegistrationCode
	if err := r.db.WithContext(ctx).First(&item, id).Error; err != nil {
		return nil, translateError(err)
	}
	result := toDomain(item)
	return &result, nil
}

func (r *Repo) DeleteUnused(ctx context.Context, id uint) error {
	var issuanceCount int64
	if err := r.db.WithContext(ctx).Model(&model.WeChatRegistrationIssuance{}).
		Where("registration_code_id = ?", id).Count(&issuanceCount).Error; err != nil {
		return translateError(err)
	}
	if issuanceCount > 0 {
		return repository.ErrConflict
	}
	result := r.db.WithContext(ctx).Where("id = ? AND status = ?", id, domainregistrationcode.StatusActive).Delete(&model.RegistrationCode{})
	if result.Error != nil {
		return translateError(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrConflict
	}
	return nil
}

var _ repository.RegistrationCodeRepository = (*Repo)(nil)
