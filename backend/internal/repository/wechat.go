package repository

import "context"

// WeChatRegistrationRepository issues one registration code per official-account OpenID.
type WeChatRegistrationRepository interface {
	IssueRegistrationCode(ctx context.Context, openID, code string) (string, error)
}
