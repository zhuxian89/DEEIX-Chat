package model

import "time"

// WeChatMiniAppBinding preserves the Mini Program identity even when its user
// is hard-deleted. This deliberately has no cascading user foreign key.
type WeChatMiniAppBinding struct {
	ControlPlaneModel
	UserID            uint       `gorm:"not null;uniqueIndex:uk_wechat_miniapp_app_user,priority:2;index:idx_wechat_miniapp_user_id;comment:DEEIX 用户ID"`
	AppID             string     `gorm:"size:64;not null;uniqueIndex:uk_wechat_miniapp_app_open,priority:1;uniqueIndex:uk_wechat_miniapp_app_user,priority:1;index:idx_wechat_miniapp_app_union,priority:1;comment:微信小程序 AppID"`
	OpenID            string     `gorm:"size:128;not null;uniqueIndex:uk_wechat_miniapp_app_open,priority:2;comment:微信小程序 OpenID"`
	UnionID           string     `gorm:"size:128;not null;default:'';index:idx_wechat_miniapp_app_union,priority:2;comment:微信开放平台 UnionID（仅留档）"`
	UnionIDObservedAt *time.Time `gorm:"comment:首次观察到 UnionID 的时间"`
	LastLoginAt       time.Time  `gorm:"not null;index:idx_wechat_miniapp_last_login_at;comment:最近登录时间"`
	RevokedAt         *time.Time `gorm:"index:idx_wechat_miniapp_revoked_at;comment:绑定撤销时间"`
}

func (WeChatMiniAppBinding) TableName() string { return "wechat_miniapp_bindings" }
