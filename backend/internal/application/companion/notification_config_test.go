package companion

import (
	"context"
	"testing"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
)

type switchingNotificationStore struct {
	*dispatchFixture
	switchConfig    func()
	claimedTemplate string
}

func (f *switchingNotificationStore) ClaimReminderNotification(ctx context.Context, now time.Time, template string) (*domain.NotificationDelivery, bool, error) {
	d, more, err := f.dispatchFixture.ClaimReminderNotification(ctx, now, template)
	if d != nil {
		f.claimedTemplate = template
		d.TemplateID = template
		f.switchConfig()
	}
	return d, more, err
}

func TestNotificationDispatchKeepsClaimedSnapshotAndStopsNewClaims(t *testing.T) {
	oldSender, newSender := &followupSender{}, &followupSender{}
	current := NotificationRuntime{Sender: oldSender, TemplateID: "old"}
	f := &switchingNotificationStore{dispatchFixture: &dispatchFixture{remaining: 2, finished: map[string]string{}}}
	f.switchConfig = func() { current = NotificationRuntime{} }
	service := &Service{Store: f, NotificationProvider: func() NotificationRuntime { return current }}
	if err := service.dispatchReminders(t.Context(), 1, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if oldSender.count != 1 || f.remaining != 1 || f.claimedTemplate != "old" {
		t.Fatal("config update altered claimed delivery or allowed claims while disabled")
	}
	current = NotificationRuntime{Sender: newSender, TemplateID: "new"}
	if err := service.dispatchReminders(t.Context(), 1, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if newSender.count != 1 || f.remaining != 0 || f.claimedTemplate != "new" {
		t.Fatal("newly enabled sender/template not used on next claim")
	}
}

func TestNotificationTemplateChangeRequiresNewConsent(t *testing.T) {
	store := testStore(t)
	if _, err := store.Ensure(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Exec("CREATE TABLE wechat_miniapp_bindings(id integer PRIMARY KEY,user_id integer,app_id text,open_id text,revoked_at datetime,created_at datetime,updated_at datetime,deleted_at datetime)").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','owner')").Error; err != nil {
		t.Fatal(err)
	}
	store.SetAppIDProvider(func() string { return "app" })
	current := NotificationRuntime{Sender: &followupSender{}, TemplateID: "old", AppID: "app"}
	service := &Service{Store: store, NotificationProvider: func() NotificationRuntime { return current }}
	if err := service.SetNotifications(t.Context(), 1, true, true, "old"); err != nil {
		t.Fatal(err)
	}
	state, err := service.AssistantState(t.Context(), 1)
	if err != nil || !state.NotificationReady || !state.Notifications.Enabled || state.Notifications.Credits != 1 {
		t.Fatalf("consent not usable: %+v %v", state, err)
	}
	current.TemplateID = "new"
	state, err = service.AssistantState(t.Context(), 1)
	if err != nil || !state.NotificationReady || state.Notifications.Enabled || state.Notifications.Credits != 0 || state.Notifications.TemplateID != "new" {
		t.Fatalf("old consent survived template change: %+v %v", state, err)
	}
	if err := service.SetNotifications(t.Context(), 1, true, true, "old"); err == nil {
		t.Fatal("outdated client consent accepted")
	}
	if err := service.SetNotifications(t.Context(), 1, true, true, "new"); err != nil {
		t.Fatal(err)
	}
	current = NotificationRuntime{}
	state, err = service.AssistantState(t.Context(), 1)
	if err != nil || state.NotificationReady || state.Notifications.Enabled {
		t.Fatalf("disabled config reported ready: %+v %v", state, err)
	}
}
