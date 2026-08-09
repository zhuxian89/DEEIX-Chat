package model

import "time"

// RegistrationCode stores one-time account registration codes independently from user data.
type RegistrationCode struct {
	ControlPlaneModel
	Code            string     `gorm:"size:128;not null;uniqueIndex:idx_registration_codes_code;comment:注册码明文"`
	CodeHint        string     `gorm:"size:16;not null;default:'';comment:注册码末尾提示"`
	Status          string     `gorm:"size:16;not null;default:'active';index:idx_registration_codes_status;comment:状态"`
	UsedByUserID    uint       `gorm:"not null;default:0;index:idx_registration_codes_used_user;comment:使用者用户ID"`
	UsedAt          *time.Time `gorm:"index:idx_registration_codes_used_at;comment:使用时间"`
	CreatedByUserID uint       `gorm:"not null;default:0;index:idx_registration_codes_created_by;comment:创建者用户ID"`
}

func (RegistrationCode) TableName() string { return "registration_codes" }
