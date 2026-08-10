package model

import "time"

// InvitationCode 存储每个用户的固定邀请码，与 identity_users 解耦以便上游同步零冲突。
type InvitationCode struct {
	BaseModel
	UserID uint   `gorm:"not null;uniqueIndex:uk_invitation_codes_user_id;comment:所属用户ID"`
	Code   string `gorm:"size:32;not null;uniqueIndex:uk_invitation_codes_code;comment:带前缀的邀请码"`
}

func (InvitationCode) TableName() string { return "invitation_codes" }

// InvitationRelationship 记录一次邀请事件及双方奖励发放状态，硬删除语义保留不可变审计记录。
type InvitationRelationship struct {
	ControlPlaneModel
	InviterUserID        uint       `gorm:"not null;index:idx_invitation_relationships_inviter;comment:邀请人ID"`
	InvitedUserID        uint       `gorm:"not null;uniqueIndex:uk_invitation_relationships_invited;comment:被邀请人ID"`
	InvitationCode       string     `gorm:"size:32;not null;default:'';comment:使用的邀请码"`
	InviteeRewardNanousd int64      `gorm:"not null;default:0;comment:被邀请人实发奖励"`
	InviterRewardNanousd int64      `gorm:"not null;default:0;comment:邀请人实发奖励"`
	InviteeRewardedAt    *time.Time `gorm:"index:idx_invitation_relationships_invitee_rewarded;comment:被邀请人奖励发放时间"`
	InviterRewardedAt    *time.Time `gorm:"index:idx_invitation_relationships_inviter_rewarded;comment:邀请人奖励发放时间"`
}

func (InvitationRelationship) TableName() string { return "invitation_relationships" }
