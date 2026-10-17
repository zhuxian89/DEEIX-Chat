package companion

import "time"

type contextPreferencesRecord struct {
	UserID        uint `gorm:"primaryKey;autoIncrement:false"`
	Enabled       bool
	ExcludedJSON  string `gorm:"type:text"`
	ForgetBefore  time.Time
	RefreshAfter  time.Time
	SuccessDigest string `gorm:"size:64"`
}

func (contextPreferencesRecord) TableName() string { return "companion_context_preferences" }

type followupRecord struct {
	ID                   string    `gorm:"primaryKey;size:64"`
	UserID               uint      `gorm:"index:companion_followup_due,priority:1"`
	Title                string    `gorm:"size:200"`
	DueAt                time.Time `gorm:"index:companion_followup_due,priority:3"`
	Status               string    `gorm:"size:20;index:companion_followup_due,priority:2"`
	SourceKind           string    `gorm:"size:20"`
	SourceID             string    `gorm:"size:100"`
	SourceConversationID string    `gorm:"size:64"`
	Evidence             string    `gorm:"type:text"`
	SourceOwnerKey       string    `gorm:"size:64"`
	SourceMessageID      uint
	SourceAt             time.Time
	Revision             uint
	UserManaged          bool
	DeliveredAt          time.Time
	NotificationStatus   string `gorm:"size:24"`
	UpdatedAt            time.Time
}

func (followupRecord) TableName() string { return "companion_followups" }

type decisionRecord struct {
	UserID uint   `gorm:"primaryKey;autoIncrement:false"`
	Kind   string `gorm:"primaryKey;size:20"`
	Reason string `gorm:"size:80"`
	At     time.Time
}

func (decisionRecord) TableName() string { return "companion_decisions" }

type notificationRecord struct {
	UserID        uint `gorm:"primaryKey;autoIncrement:false"`
	Enabled       bool
	Credits       int
	TemplateID    string `gorm:"size:128"`
	BoundAppID    string `gorm:"size:128"`
	BoundOpenID   string `gorm:"size:128"`
	LastConsentAt time.Time
}

func (notificationRecord) TableName() string { return "companion_notifications" }
