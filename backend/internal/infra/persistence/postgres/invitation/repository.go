package invitation

import (
	"context"
	"errors"
	"strings"

	domaininvitation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/invitation"
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

func toDomainCode(item model.InvitationCode) domaininvitation.InvitationCode {
	return domaininvitation.InvitationCode{
		ID:        item.ID,
		UserID:    item.UserID,
		Code:      item.Code,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}

func toDomainRelationship(item model.InvitationRelationship) domaininvitation.Relationship {
	return domaininvitation.Relationship{
		ID:                   item.ID,
		InviterUserID:        item.InviterUserID,
		InvitedUserID:        item.InvitedUserID,
		InvitationCode:       item.InvitationCode,
		InviteeRewardNanousd: item.InviteeRewardNanousd,
		InviterRewardNanousd: item.InviterRewardNanousd,
		InviteeRewardedAt:    item.InviteeRewardedAt,
		InviterRewardedAt:    item.InviterRewardedAt,
		CreatedAt:            item.CreatedAt,
		UpdatedAt:            item.UpdatedAt,
	}
}

func (r *Repo) GetInvitationCodeByUserID(ctx context.Context, userID uint) (*domaininvitation.InvitationCode, error) {
	var item model.InvitationCode
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&item).Error; err != nil {
		return nil, translateError(err)
	}
	result := toDomainCode(item)
	return &result, nil
}

func (r *Repo) GetInvitationCodeByCode(ctx context.Context, code string) (*domaininvitation.InvitationCode, error) {
	trimmed := strings.ToUpper(strings.TrimSpace(code))
	if trimmed == "" {
		return nil, repository.ErrInvalidInput
	}
	var item model.InvitationCode
	if err := r.db.WithContext(ctx).Where("code = ?", trimmed).First(&item).Error; err != nil {
		return nil, translateError(err)
	}
	result := toDomainCode(item)
	return &result, nil
}

func (r *Repo) GetOrCreateInvitationCode(ctx context.Context, userID uint) (*domaininvitation.InvitationCode, error) {
	if userID == 0 {
		return nil, repository.ErrInvalidInput
	}
	if existing, err := r.GetInvitationCodeByUserID(ctx, userID); err == nil {
		return existing, nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	code, err := domaininvitation.GenerateCode(domaininvitation.DefaultCodeLength)
	if err != nil {
		return nil, err
	}
	item := model.InvitationCode{UserID: userID, Code: code}
	if err := r.db.WithContext(ctx).Create(&item).Error; err != nil {
		// 并发或碰撞时退化为读取已有记录。
		if existing, findErr := r.GetInvitationCodeByUserID(ctx, userID); findErr == nil {
			return existing, nil
		}
		return nil, translateError(err)
	}
	result := toDomainCode(item)
	return &result, nil
}

func (r *Repo) GetRelationshipByInvited(ctx context.Context, invitedUserID uint) (*domaininvitation.Relationship, error) {
	var item model.InvitationRelationship
	if err := r.db.WithContext(ctx).Where("invited_user_id = ?", invitedUserID).First(&item).Error; err != nil {
		return nil, translateError(err)
	}
	result := toDomainRelationship(item)
	return &result, nil
}

func (r *Repo) ListRelationshipsByInviter(ctx context.Context, inviterUserID uint, offset, limit int) ([]domaininvitation.InvitedUser, int64, error) {
	// LEFT JOIN 并显式过滤软删除用户：手写 Joins 不会自动套用软删除作用域，
	// 必须带上 identity_users.deleted_at IS NULL，否则软删除的被邀请人仍会泄露到列表中。
	query := r.db.WithContext(ctx).
		Model(&model.InvitationRelationship{}).
		Where("inviter_user_id = ?", inviterUserID).
		Joins("LEFT JOIN identity_users ON identity_users.id = invitation_relationships.invited_user_id AND identity_users.deleted_at IS NULL")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, translateError(err)
	}
	type joined struct {
		model.InvitationRelationship
		InvitedDisplayName string
		InvitedUsername    string
	}
	items := make([]joined, 0)
	if err := query.
		Select("invitation_relationships.*, identity_users.display_name AS invited_display_name, identity_users.username AS invited_username").
		Order("invitation_relationships.id DESC").
		Offset(offset).Limit(limit).
		Scan(&items).Error; err != nil {
		return nil, 0, translateError(err)
	}
	result := make([]domaininvitation.InvitedUser, 0, len(items))
	for _, item := range items {
		result = append(result, domaininvitation.InvitedUser{
			RelationshipID:       item.ID,
			InvitedUserID:        item.InvitedUserID,
			InvitedDisplayName:   item.InvitedDisplayName,
			InvitedUsername:      item.InvitedUsername,
			InvitedAt:            item.CreatedAt,
			InviterRewardNanousd: item.InviterRewardNanousd,
		})
	}
	return result, total, nil
}

func (r *Repo) ListRelationships(ctx context.Context, filter repository.InvitationListFilter, offset, limit int) ([]domaininvitation.Relationship, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.InvitationRelationship{})
	if filter.InviterUserID != 0 {
		query = query.Where("inviter_user_id = ?", filter.InviterUserID)
	}
	if filter.InvitedUserID != 0 {
		query = query.Where("invited_user_id = ?", filter.InvitedUserID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, translateError(err)
	}
	items := make([]model.InvitationRelationship, 0)
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, translateError(err)
	}
	result := make([]domaininvitation.Relationship, 0, len(items))
	for _, item := range items {
		result = append(result, toDomainRelationship(item))
	}
	return result, total, nil
}

var _ repository.InvitationRepository = (*Repo)(nil)
