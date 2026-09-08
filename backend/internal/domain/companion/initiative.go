package companion

import "time"

// InitiativeState and Initiative belong only to the companion feature. Candidates
// are invisible until a foreground client accepts them against the same chat turn.
type InitiativeState struct {
	UserID             uint
	Mode               string
	LastAttemptAt      time.Time
	IdleAttemptAt      time.Time
	LastHomeAt         time.Time
	HomeDay            string
	HomeCount          uint
	UnansweredHomes    uint
	HomeAfterUserID    uint
	IdleVisitID        string
	IdleAfterMessageID uint
	LastIdleAt         time.Time
}

type Initiative struct {
	ID             string
	UserID         uint
	ConversationID uint
	Kind           string
	VisitID        string
	AfterMessageID uint
	AfterUserID    uint
	Revision       uint
	Text           string
	TopicJSON      string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	AcceptedAt     time.Time
}
