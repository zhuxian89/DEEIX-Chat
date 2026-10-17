package companion

import (
	"context"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"time"
)

// Optional extension keeps older companion implementations compatible.
type AssistantStore interface {
	ContextPreferences(context.Context, uint) (domain.ContextPreferences, error)
	SetContextPreferences(context.Context, uint, domain.ContextPreferences) error
	ContextConversations(context.Context, uint, string, int) ([]domain.ContextConversation, error)
	ContextSources(context.Context, uint, string) ([]domain.ContextSource, error)
	Followups(context.Context, uint) ([]domain.Followup, error)
	ContextFollowups(context.Context, uint) ([]domain.Followup, error)
	SaveFollowup(context.Context, domain.Followup) (domain.Followup, error)
	SyncTodoFollowups(context.Context, uint, time.Time) error
	RecordDecision(context.Context, uint, domain.Decision) error
	Decisions(context.Context, uint) ([]domain.Decision, error)
	NotificationSettings(context.Context, uint) (domain.NotificationSettings, error)
	SetNotificationSettings(context.Context, uint, domain.NotificationSettings, bool) error
	GrantNotification(context.Context, uint, domain.NotificationSettings, domain.NotificationConsent) error
	AssistantUsers(context.Context, uint, int) ([]uint, error)
	ClaimFollowupRefresh(context.Context, uint, time.Time, string) (bool, error)
	CompleteFollowupRefresh(context.Context, uint, string) error
	FollowupRefreshCurrent(context.Context, uint, string) (bool, error)
	PublishDueFollowup(context.Context, uint, time.Time, string) (*domain.NotificationDelivery, error)
	FinishNotification(context.Context, string, string) error
}

type NotificationSender interface {
	Send(context.Context, domain.NotificationDelivery) error
}
