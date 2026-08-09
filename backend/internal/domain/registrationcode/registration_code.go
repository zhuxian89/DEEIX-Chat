package registrationcode

import "time"

const (
	StatusActive = "active"
	StatusUsed   = "used"
)

type RegistrationCode struct {
	ID              uint
	Code            string
	CodeHint        string
	Status          string
	UsedByUserID    uint
	UsedAt          *time.Time
	CreatedByUserID uint
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
