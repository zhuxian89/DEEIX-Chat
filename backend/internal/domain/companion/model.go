// Package companion defines protocol-independent companion data.
package companion

import (
	"errors"
	"time"
)

var (
	ErrBusy     = errors.New("resource conflict")
	ErrNotFound = errors.New("resource not found")
	ErrInvalid  = errors.New("invalid request")
)

type Profile struct {
	UserID               uint
	ConversationID       uint
	ConversationPublicID string
	Model                string
	Quiet                bool
	Summary              string
	SummaryAt            time.Time
	ThroughID            uint
	ForgetThroughID      uint
	Revision             uint
	LeaseToken           string
	LeaseUntil           int64
	RefreshToken         string
	RefreshAfter         int64
	LastGreeting         string
	GreetingID           string
	GreetingAt           time.Time
	GreetingAfterUserID  uint
	ReadThroughID        uint
	LastTopicJSON        string
	LastTopicAt          time.Time
	SeenTopicsJSON       string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Memory struct {
	ID              string
	UserID          uint
	Key             string
	Value           string
	Evidence        string
	SourceMessageID uint
	SourceAt        time.Time
	ExpiresAt       time.Time
	UpdatedAt       time.Time
}

type Topic struct {
	Category    string
	Title       string
	URL         string
	Opener      string
	PublishedAt time.Time
	FetchedAt   time.Time
}

type TopicCache struct {
	Category     string
	TopicsJSON   string
	RefreshToken string
	RefreshAfter int64
	FetchedAt    time.Time
}

type GreetingUpdate struct {
	Text           string
	ID             string
	At             time.Time
	AfterUserID    uint
	TopicJSON      string
	TopicAt        time.Time
	SeenTopicsJSON string
}

type Extraction struct {
	UserID    uint
	Revision  uint
	Token     string
	ThroughID uint
	Summary   string
	Memories  []Memory
	At        time.Time
}
