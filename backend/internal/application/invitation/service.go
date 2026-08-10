package invitation

import (
	"context"
	"strings"

	domaininvitation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/invitation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

// Service 提供邀请码查询与面板能力。注册事务内的发奖由 user repo 编排，不在此处。
type Service struct {
	repo repository.InvitationRepository
	cfg  *config.Runtime
}

func NewService(repo repository.InvitationRepository, cfg *config.Runtime) *Service {
	return &Service{repo: repo, cfg: cfg}
}

// GetPanel 返回用户中心的邀请面板视图（邀请码、邀请链接、已邀请人数）。
func (s *Service) GetPanel(ctx context.Context, userID uint) (*domaininvitation.InvitationPanel, error) {
	code, err := s.repo.GetOrCreateInvitationCode(ctx, userID)
	if err != nil {
		return nil, err
	}
	_, total, err := s.repo.ListRelationshipsByInviter(ctx, userID, 0, 1)
	if err != nil {
		return nil, err
	}
	return &domaininvitation.InvitationPanel{
		InvitationCode: code.Code,
		InviteLink:     s.buildInviteLink(code.Code),
		InviteCount:    total,
	}, nil
}

// ListInvitedUsers 返回某邀请人邀请的用户列表（脱敏，不含邮箱）。
func (s *Service) ListInvitedUsers(ctx context.Context, inviterUserID uint, page, pageSize int) ([]domaininvitation.InvitedUser, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.ListRelationshipsByInviter(ctx, inviterUserID, (page-1)*pageSize, pageSize)
}

// ListRelationships 管理端邀请关系列表。
func (s *Service) ListRelationships(ctx context.Context, page, pageSize int, inviterUserID, invitedUserID uint) ([]domaininvitation.Relationship, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.repo.ListRelationships(ctx, repository.InvitationListFilter{InviterUserID: inviterUserID, InvitedUserID: invitedUserID}, (page-1)*pageSize, pageSize)
}

func (s *Service) buildInviteLink(code string) string {
	base := ""
	if s.cfg != nil {
		base = s.cfg.Snapshot().PublicWebBaseURL
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return "/register?invite=" + code
	}
	return base + "/register?invite=" + code
}
