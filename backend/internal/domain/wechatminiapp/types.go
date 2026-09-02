package wechatminiapp

import "time"

// Identity is the stable identity returned by WeChat code2Session.
// SessionKey is intentionally excluded because DEEIX does not need to persist it.
type Identity struct {
	OpenID  string
	UnionID string
}

// Binding maps one Mini Program identity to the canonical DEEIX user.
type Binding struct {
	ID                uint
	UserID            uint
	AppID             string
	OpenID            string
	UnionID           string
	UnionIDObservedAt *time.Time
	LastLoginAt       time.Time
	RevokedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Presets contains task-oriented models presented by the Mini Program.
type Presets struct {
	ChatModel  string
	ImageModel string
}
