package wechat

import "time"

const (
	ActionIssueRegistrationCode = "issue_registration_code"
	ResponseTypeText            = "text"
	ResultIssued                = "issued"
	ResultReplayed              = "replayed"
	ResultHandled               = "handled"
	ResultFailed                = "failed"
)

type ReplyTemplate struct {
	ID           uint
	Name         string
	ResponseType string
	Content      string
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type KeywordRule struct {
	ID              uint
	Keyword         string
	Action          string
	TemplateID      uint
	TemplateName    string
	TemplateType    string
	TemplateContent string
	TemplateEnabled bool
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type InvocationLog struct {
	ID                 uint
	OpenID             string
	Keyword            string
	Action             string
	TemplateID         uint
	RegistrationCodeID uint
	Result             string
	ErrorCode          string
	ErrorMessage       string
	CreatedAt          time.Time
}

type IssuanceRecord struct {
	ID                 uint
	OpenID             string
	RegistrationCodeID uint
	Code               string
	Status             string
	UsedByUserID       uint
	UsedAt             *time.Time
	CreatedAt          time.Time
}

type Stats struct {
	IssuanceCount int64
	SuccessCount  int64
	FailureCount  int64
}

type IssueResult struct {
	Code               string
	RegistrationCodeID uint
	Created            bool
}
