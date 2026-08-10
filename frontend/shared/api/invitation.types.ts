export interface InvitationPanel {
  invitationCode: string;
  inviteLink: string;
  inviteCount: number;
}

export interface InvitedUser {
  relationshipId: number;
  invitedUserId: number;
  invitedDisplayName: string;
  invitedUsername: string;
  invitedAt: string;
  inviterRewardNanousd: number;
}

export interface InvitationRelationship {
  id: number;
  inviterUserId: number;
  invitedUserId: number;
  invitationCode: string;
  inviteeRewardNanousd: number;
  inviterRewardNanousd: number;
  inviteeRewardedAt: string;
  inviterRewardedAt: string;
  createdAt: string;
}
