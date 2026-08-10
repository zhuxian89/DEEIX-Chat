package repository

import (
	"context"

	domaininvitation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/invitation"
)

// InvitationListFilter 邀请关系列表过滤条件。
type InvitationListFilter struct {
	InviterUserID uint
	InvitedUserID uint
}

// InvitationRepository 提供邀请码与邀请关系的持久化能力（非注册事务路径）。
// 注册事务内的原子写入（建邀请码 + 邀请关系 + 发奖）由 user repo 的事务辅助函数完成，
// 它们共享同一 *gorm.DB 事务句柄，不经过此接口。
type InvitationRepository interface {
	// GetInvitationCodeByUserID 按 user_id 查询邀请码。
	GetInvitationCodeByUserID(ctx context.Context, userID uint) (*domaininvitation.InvitationCode, error)
	// GetInvitationCodeByCode 按码字符串查询邀请码（用于解析邀请人）。
	GetInvitationCodeByCode(ctx context.Context, code string) (*domaininvitation.InvitationCode, error)
	// GetOrCreateInvitationCode 返回用户邀请码，不存在则创建（幂等）。
	GetOrCreateInvitationCode(ctx context.Context, userID uint) (*domaininvitation.InvitationCode, error)
	// GetRelationshipByInvited 按被邀请人查询邀请关系。
	GetRelationshipByInvited(ctx context.Context, invitedUserID uint) (*domaininvitation.Relationship, error)
	// ListRelationshipsByInviter 查询某邀请人邀请的用户列表（分页，脱敏）。
	ListRelationshipsByInviter(ctx context.Context, inviterUserID uint, offset, limit int) ([]domaininvitation.InvitedUser, int64, error)
	// ListRelationships 邀请关系列表（管理端，分页）。
	ListRelationships(ctx context.Context, filter InvitationListFilter, offset, limit int) ([]domaininvitation.Relationship, int64, error)
}
