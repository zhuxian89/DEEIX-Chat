package model

// WeChatRegistrationIssuance records the one registration code issued to a WeChat OpenID.
// It intentionally lives outside the existing user tables so upstream syncs remain low-conflict.
type WeChatRegistrationIssuance struct {
	ControlPlaneModel
	OpenID             string `gorm:"size:128;not null;uniqueIndex:idx_wechat_issuances_open_id;comment:微信公众号 OpenID"`
	RegistrationCodeID uint   `gorm:"not null;uniqueIndex:idx_wechat_issuances_registration_code;comment:注册码 ID"`
}

func (WeChatRegistrationIssuance) TableName() string { return "wechat_registration_issuances" }
