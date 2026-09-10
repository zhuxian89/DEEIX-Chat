package miniapptodo

import "time"

type Ownership struct {
	AppID       string `gorm:"primaryKey;size:64"`
	OwnerOpenID string `gorm:"primaryKey;size:128"`
}
type unlockRecord struct {
	AppID      string    `gorm:"primaryKey;size:64"`
	OpenID     string    `gorm:"primaryKey;size:128"`
	UnlockedAt time.Time `gorm:"not null"`
}

func (unlockRecord) TableName() string { return "miniapp_entry_unlocks" }

type listRecord struct {
	Ownership `gorm:"embedded"`
	ID        string `gorm:"primaryKey;size:36"`
	Name      string `gorm:"size:100;not null"`
	Position  int
	Version   int64 `gorm:"not null"`
	DeletedAt *time.Time
}

func (listRecord) TableName() string { return "miniapp_todo_lists" }

type taskRecord struct {
	Ownership         `gorm:"embedded"`
	ID                string `gorm:"primaryKey;size:36"`
	ListID            string `gorm:"size:36;index"`
	ParentID          string `gorm:"size:36;index"`
	Title             string `gorm:"size:200"`
	Notes             string
	Version           int64 `gorm:"not null"`
	GenerationVersion int64
	Data              string `gorm:"not null"`
	UpdatedAt         time.Time
	CompletedAt       *time.Time
	DeletedAt         *time.Time
}

func (taskRecord) TableName() string { return "miniapp_todo_tasks" }

type operationRecord struct {
	Ownership `gorm:"embedded"`
	ID        string `gorm:"primaryKey;size:36"`
	Digest    string `gorm:"size:64;not null"`
	Result    string `gorm:"not null"`
	CreatedAt time.Time
}

func (operationRecord) TableName() string { return "miniapp_todo_operations" }

type feedbackRecord struct {
	Ownership `gorm:"embedded"`
	ID        string `gorm:"primaryKey;size:36"`
	Content   string `gorm:"not null"`
	Status    string `gorm:"size:20;not null"`
	CreatedAt time.Time
}

func (feedbackRecord) TableName() string { return "miniapp_todo_feedback" }
