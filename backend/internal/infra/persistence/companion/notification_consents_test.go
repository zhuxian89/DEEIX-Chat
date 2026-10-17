package companion

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"github.com/google/uuid"
)

func TestNotificationReceiptsAccumulateAndSurviveConsumptionAndRestart(t *testing.T) {
	s, db := assistantFixture(t)
	if err := db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','owner')").Error; err != nil {
		t.Fatal(err)
	}
	n := domain.NotificationSettings{Enabled: true, TemplateID: "template", AppID: "app"}
	first := domain.NotificationConsent{RequestID: uuid.NewString()}
	if err := s.GrantNotification(t.Context(), 1, n, first); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Each of three independently accepted clicks is reported four times.
			if err := s.GrantNotification(t.Context(), 1, n, domain.NotificationConsent{RequestID: fmt.Sprintf("accepted-click-%d", i%3), Automatic: true}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	settings, err := s.NotificationSettings(t.Context(), 1)
	if err != nil || settings.Credits != 4 {
		t.Fatalf("grants were lost or duplicated: %+v %v", settings, err)
	}
	// A legacy client must not overwrite accumulated grants with one credit.
	if err := s.SetNotificationSettings(t.Context(), 1, n, true); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		_, err := s.ExecuteReminder(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Action: "create", Title: "拿快递", Schedule: domain.ReminderSchedule{Kind: "once", StartAt: now.Add(time.Minute)}}, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.TickReminders(t.Context(), now.Add(time.Minute), "template"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		delivery, _, err := s.ClaimReminderNotification(t.Context(), now.Add(time.Minute), "template")
		if err != nil || delivery == nil {
			t.Fatalf("accepted credit did not queue a notification: %v", err)
		}
		if err := s.FinishReminderNotification(t.Context(), delivery.ID, "sent"); err != nil {
			t.Fatal(err)
		}
	}
	s, err = NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	s.SetAppIDProvider(func() string { return "app" })
	if err := s.GrantNotification(t.Context(), 1, n, first); err != nil {
		t.Fatal(err)
	}
	settings, err = s.NotificationSettings(t.Context(), 1)
	if err != nil || settings.Credits != 2 {
		t.Fatalf("retry restored spent credits: %+v %v", settings, err)
	}
	var receipts int64
	db.Model(&notificationConsentRecord{}).Count(&receipts)
	if receipts != 4 {
		t.Fatal("receipt count differs from accepted clicks", receipts)
	}
}

func TestNotificationReceiptsRespectDisablingTemplatesAndBindings(t *testing.T) {
	s, db := assistantFixture(t)
	if err := db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','owner')").Error; err != nil {
		t.Fatal(err)
	}
	n := domain.NotificationSettings{Enabled: true, TemplateID: "template", AppID: "app"}
	consent := domain.NotificationConsent{RequestID: uuid.NewString()}
	if err := s.GrantNotification(t.Context(), 1, n, consent); err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotificationSettings(t.Context(), 1, domain.NotificationSettings{}, false); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantNotification(t.Context(), 1, n, consent); err != nil {
		t.Fatal(err)
	}
	settings, _ := s.NotificationSettings(t.Context(), 1)
	if settings.Enabled || settings.Credits != 0 {
		t.Fatal("old receipt re-enabled notifications", settings)
	}
	if err := s.GrantNotification(t.Context(), 1, n, domain.NotificationConsent{RequestID: uuid.NewString(), Automatic: true}); !errors.Is(err, ErrBusy) {
		t.Fatal("automatic grant re-enabled notifications", err)
	}
	if err := s.GrantNotification(t.Context(), 1, n, domain.NotificationConsent{RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	n.TemplateID = "another-template"
	if err := s.GrantNotification(t.Context(), 1, n, consent); !errors.Is(err, ErrInvalid) {
		t.Fatal("same receipt accepted for another template", err)
	}
	if err := s.GrantNotification(t.Context(), 1, n, domain.NotificationConsent{RequestID: uuid.NewString(), Automatic: true}); !errors.Is(err, ErrBusy) {
		t.Fatal("old authorization silently followed template change", err)
	}
	if err := s.GrantNotification(t.Context(), 1, n, domain.NotificationConsent{RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	settings, _ = s.NotificationSettings(t.Context(), 1)
	if settings.Credits != 1 || settings.TemplateID != n.TemplateID {
		t.Fatal("template grants were mixed", settings)
	}
	if err := db.Exec("UPDATE wechat_miniapp_bindings SET open_id='another-owner' WHERE user_id=1").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.GrantNotification(t.Context(), 1, n, domain.NotificationConsent{RequestID: uuid.NewString(), Automatic: true}); !errors.Is(err, ErrBusy) {
		t.Fatal("old authorization followed account rebind", err)
	}
	if err := s.GrantNotification(t.Context(), 1, n, domain.NotificationConsent{RequestID: uuid.NewString()}); err != nil {
		t.Fatal(err)
	}
	settings, _ = s.NotificationSettings(t.Context(), 1)
	if settings.Credits != 1 {
		t.Fatal("old binding credits were retained", settings)
	}
}
