package invitation

// InvitationPanelResponse 用户中心邀请面板视图。
type InvitationPanelResponse struct {
	InvitationCode string `json:"invitationCode"`
	InviteLink     string `json:"inviteLink"`
	InviteCount    int64  `json:"inviteCount"`
}

// InvitedUserResponse 被邀请人脱敏信息。
type InvitedUserResponse struct {
	RelationshipID       uint   `json:"relationshipId"`
	InvitedUserID        uint   `json:"invitedUserId"`
	InvitedDisplayName   string `json:"invitedDisplayName"`
	InvitedUsername      string `json:"invitedUsername"`
	InvitedAt            string `json:"invitedAt"`
	InviterRewardNanousd int64  `json:"inviterRewardNanousd"`
}

// InvitationRelationshipResponse 管理端邀请关系。
type InvitationRelationshipResponse struct {
	ID                   uint   `json:"id"`
	InviterUserID        uint   `json:"inviterUserId"`
	InvitedUserID        uint   `json:"invitedUserId"`
	InvitationCode       string `json:"invitationCode"`
	InviteeRewardNanousd int64  `json:"inviteeRewardNanousd"`
	InviterRewardNanousd int64  `json:"inviterRewardNanousd"`
	InviteeRewardedAt    string `json:"inviteeRewardedAt"`
	InviterRewardedAt    string `json:"inviterRewardedAt"`
	CreatedAt            string `json:"createdAt"`
}

// InvitedUserListDoc 被邀请用户分页响应文档。
type InvitedUserListDoc struct {
	ErrorMsg string `json:"errorMsg"`
	Data     struct {
		Total   int64               `json:"total"`
		Results []InvitedUserResponse `json:"results"`
	} `json:"data"`
}

// InvitationRelationshipListDoc 邀请关系分页响应文档。
type InvitationRelationshipListDoc struct {
	ErrorMsg string `json:"errorMsg"`
	Data     struct {
		Total   int64                        `json:"total"`
		Results []InvitationRelationshipResponse `json:"results"`
	} `json:"data"`
}
