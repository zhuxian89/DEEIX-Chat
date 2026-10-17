package companion

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"
)

// ContextDigest ignores TODO records, which are synchronized without inference.
func ContextDigest(sources []ContextSource) string {
	items := append([]ContextSource(nil), sources...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	h := sha256.New()
	for _, s := range items {
		if s.Kind == "conversation" {
			fmt.Fprintf(h, "%q|%q|%q|%q|%s\n", s.ID, s.ConversationID, s.Title, s.Text, s.At.UTC().Format(time.RFC3339Nano))
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

type ContextPreferences struct {
	Enabled      bool
	Excluded     []string
	ForgetBefore time.Time
}

type ContextSource struct {
	ID, Kind, Title, Text, ConversationID string
	MessageID                             uint
	At                                    time.Time
}

type ContextConversation struct {
	ID, Title string
	Excluded  bool
}

type Followup struct {
	ID                                                   string
	UserID                                               uint
	Title                                                string
	DueAt                                                time.Time
	Status                                               string
	SourceKind, SourceID, SourceConversationID, Evidence string
	SourceOwnerKey                                       string
	SourceMessageID                                      uint
	SourceAt                                             time.Time
	Revision                                             uint
	UserManaged                                          bool
	DeliveredAt                                          time.Time
	NotificationStatus                                   string
	UpdatedAt                                            time.Time
}

type Decision struct {
	Kind, Reason string
	At           time.Time
}

type NotificationSettings struct {
	Enabled    bool
	Credits    int
	TemplateID string
	AppID      string // Internal consent binding; never supplied by the client.
}

type NotificationDelivery struct {
	ID                               string
	UserID                           uint
	AppID, OpenID, TemplateID, Title string
	DueAt                            time.Time
}
