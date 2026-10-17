package companion

import (
	"context"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"strings"
	"time"
)

func (s *Service) SetContextPreferences(ctx context.Context, user uint, enabled bool, excluded []string) error {
	store, ok := s.Store.(AssistantStore)
	if !ok {
		return ErrNotFound
	}
	if _, err := s.Store.Profile(ctx, user); err != nil {
		return err
	}
	return store.SetContextPreferences(ctx, user, domain.ContextPreferences{Enabled: enabled, Excluded: excluded})
}
func (s *Service) ContextConversations(ctx context.Context, user uint, query string, page int) ([]domain.ContextConversation, error) {
	store, ok := s.Store.(AssistantStore)
	if !ok {
		return nil, ErrNotFound
	}
	if len([]rune(query)) > 100 || page < 1 || page > 10000 {
		return nil, ErrInvalid
	}
	if _, err := s.Store.Profile(ctx, user); err != nil {
		return nil, err
	}
	return store.ContextConversations(ctx, user, query, page)
}
func (s *Service) SaveFollowup(ctx context.Context, user uint, id, title, status string, due time.Time, revision uint) (domain.Followup, error) {
	store, ok := s.Store.(AssistantStore)
	if !ok {
		return domain.Followup{}, ErrNotFound
	}
	if _, err := s.Store.Profile(ctx, user); err != nil {
		return domain.Followup{}, err
	}
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 120 || due.IsZero() || due.After(time.Now().Add(366*24*time.Hour)) {
		return domain.Followup{}, ErrInvalid
	}
	f := domain.Followup{UserID: user, SourceKind: "manual", SourceAt: time.Now()}
	if id != "" {
		items, err := store.Followups(ctx, user)
		if err != nil {
			return f, err
		}
		found := false
		for _, item := range items {
			if item.ID == id {
				f = item
				found = true
				break
			}
		}
		if !found {
			return f, ErrNotFound
		}
		if f.Revision != revision {
			return f, ErrBusy
		}
	} else if revision != 0 {
		return f, ErrInvalid
	}
	if status == "pending" && due.Before(time.Now()) && (f.ID == "" || !due.Equal(f.DueAt)) {
		return f, ErrInvalid
	}
	f.Title, f.Status, f.DueAt, f.UserManaged = title, status, due, true
	return store.SaveFollowup(ctx, f)
}
func (s *Service) SetNotifications(ctx context.Context, user uint, enabled, accepted bool, template string, consents ...domain.NotificationConsent) error {
	store, ok := s.Store.(AssistantStore)
	if !ok {
		return ErrNotFound
	}
	if _, err := s.Store.Profile(ctx, user); err != nil {
		return err
	}
	notification := s.notificationRuntime()
	if enabled && (notification.Sender == nil || notification.TemplateID == "" || template != notification.TemplateID || !accepted) {
		return ErrInvalid
	}
	settings := domain.NotificationSettings{Enabled: enabled, TemplateID: notification.TemplateID, AppID: notification.AppID}
	if len(consents) > 1 {
		return ErrInvalid
	}
	if len(consents) == 1 {
		consent := consents[0]
		if consent.RequestID != "" {
			if !enabled || len(consent.RequestID) < 8 || len(consent.RequestID) > 100 || strings.TrimSpace(consent.RequestID) != consent.RequestID {
				return ErrInvalid
			}
			return store.GrantNotification(ctx, user, settings, consent)
		}
		if consent.Automatic {
			return ErrInvalid
		}
	}
	return store.SetNotificationSettings(ctx, user, settings, enabled && accepted)
}
