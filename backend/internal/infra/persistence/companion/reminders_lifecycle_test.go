package companion

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
	chat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	domainchat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/google/uuid"
)

func reminderDeliveryFixture(t *testing.T) *Store {
	t.Helper()
	s, db := assistantFixture(t)
	if err := db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','owner')").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotificationSettings(t.Context(), 1, domain.NotificationSettings{Enabled: true, TemplateID: "template"}, true); err != nil {
		t.Fatal(err)
	}
	return s
}

func reopenReminderStore(t *testing.T, s *Store) *Store {
	t.Helper()
	reopened, err := NewStore(s.db)
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetAppIDProvider(func() string { return "app" })
	return reopened
}

func prepareReminderAtPhase(t *testing.T, s *Store, now time.Time, phase string) (domain.Reminder, time.Time) {
	t.Helper()
	created := reminderCommand(t, s, domain.ReminderCommand{Action: "create", Title: "为了测试", Schedule: domain.ReminderSchedule{Kind: "once", StartAt: now.Add(2 * time.Minute)}}, now)
	p := created.Snapshot.Plans[0]
	editAt := now.Add(30 * time.Second)
	if phase == "pending" {
		result := reminderCommand(t, s, domain.ReminderCommand{Action: "snooze", ID: p.ID, Revision: p.Revision, DelayMinutes: 1}, editAt)
		p = result.Snapshot.Plans[0]
	}
	if phase == "queued" {
		editAt = p.NextAt
		if err := s.TickReminders(t.Context(), editAt, "template"); err != nil {
			t.Fatal(err)
		}
	}
	return p, editAt
}

func TestReminderRescheduleCancelsOldTimeAndSurvivesReopen(t *testing.T) {
	for _, phase := range []string{"scheduled", "pending", "queued"} {
		t.Run(phase, func(t *testing.T) {
			s := reminderDeliveryFixture(t)
			now := time.Date(2026, 10, 17, 16, 22, 0, 0, domain.ReminderZone)
			p, editAt := prepareReminderAtPhase(t, s, now, phase)
			oldDue := p.NextAt
			newDue := oldDue.Add(3 * time.Minute)
			reminderCommand(t, s, domain.ReminderCommand{Action: "update", ID: p.ID, Revision: p.Revision, Schedule: domain.ReminderSchedule{Kind: "once", StartAt: newDue}}, editAt)
			s = reopenReminderStore(t, s)
			state, err := s.ReminderSnapshot(t.Context(), 1, "companion", editAt)
			if err != nil || len(state.Plans) != 1 || state.Plans[0].ID != p.ID || !state.Plans[0].NextAt.Equal(newDue) || state.Plans[0].Status != "active" {
				t.Fatalf("edited reminder was not restored: %+v, %v", state, err)
			}
			if err := s.TickReminders(t.Context(), oldDue, "template"); err != nil {
				t.Fatal(err)
			}
			if delivery, _, err := s.ClaimReminderNotification(t.Context(), oldDue, "template"); err != nil || delivery != nil {
				t.Fatalf("old time still delivered: %+v, %v", delivery, err)
			}
			for i := 0; i < 2; i++ {
				if err := s.TickReminders(t.Context(), newDue, "template"); err != nil {
					t.Fatal(err)
				}
			}
			state, err = s.ReminderSnapshot(t.Context(), 1, "companion", newDue)
			if err != nil || len(state.Occurrences) != 1 || state.Occurrences[0].Status != "due" || !state.Occurrences[0].DueAt.Equal(newDue) {
				t.Fatalf("wrong in-app reminder after reschedule: %+v, %v", state, err)
			}
			delivery, _, err := s.ClaimReminderNotification(t.Context(), newDue, "template")
			if err != nil || delivery == nil || !delivery.DueAt.Equal(newDue) || delivery.Title != p.Title {
				t.Fatalf("new time did not deliver: %+v, %v", delivery, err)
			}
		})
	}
}

func TestReminderPauseAndDeleteSuppressPendingDelivery(t *testing.T) {
	for _, action := range []string{"pause", "delete"} {
		for _, phase := range []string{"scheduled", "pending", "queued"} {
			t.Run(action+"/"+phase, func(t *testing.T) {
				s := reminderDeliveryFixture(t)
				now := time.Date(2026, 10, 17, 16, 22, 0, 0, domain.ReminderZone)
				p, editAt := prepareReminderAtPhase(t, s, now, phase)
				reminderCommand(t, s, domain.ReminderCommand{Action: action, ID: p.ID, Revision: p.Revision}, editAt)
				s = reopenReminderStore(t, s)
				checkAt := p.NextAt.Add(time.Minute)
				if err := s.TickReminders(t.Context(), checkAt, "template"); err != nil {
					t.Fatal(err)
				}
				state, err := s.ReminderSnapshot(t.Context(), 1, "companion", checkAt)
				if err != nil {
					t.Fatal(err)
				}
				if action == "delete" && len(state.Plans) != 0 || action == "pause" && (len(state.Plans) != 1 || state.Plans[0].Status != "paused" || !state.Plans[0].NextAt.IsZero()) {
					t.Fatalf("state was not preserved: %+v", state)
				}
				for _, event := range state.Occurrences {
					if event.Status == "pending" || event.Status == "due" || event.NotifiedAt.After(editAt) {
						t.Fatalf("stopped reminder reappeared: %+v", event)
					}
				}
				if delivery, _, err := s.ClaimReminderNotification(t.Context(), checkAt, "template"); err != nil || delivery != nil {
					t.Fatalf("stopped reminder delivered: %+v, %v", delivery, err)
				}
				settings, err := s.NotificationSettings(t.Context(), 1)
				if err != nil || settings.Credits != 1 {
					t.Fatalf("stopped reminder consumed consent: %+v, %v", settings, err)
				}
			})
		}
	}
}

func TestReminderDailyCompletionPreservesNextDayAcrossReopen(t *testing.T) {
	for _, beforeDue := range []bool{true, false} {
		name := "after_due"
		if beforeDue {
			name = "before_due"
		}
		t.Run(name, func(t *testing.T) {
			s, _ := assistantFixture(t)
			now := time.Date(2026, 10, 17, 19, 0, 0, 0, domain.ReminderZone)
			due := now.Add(time.Hour)
			created := reminderCommand(t, s, domain.ReminderCommand{Action: "create", Title: "陪孩子写作业", Schedule: domain.ReminderSchedule{Kind: "daily", StartAt: now, Times: []int{20 * 60}}}, now)
			p := created.Snapshot.Plans[0]
			completedAt := due.Add(-time.Minute)
			if !beforeDue {
				completedAt = due
				if err := s.TickReminders(t.Context(), due, ""); err != nil {
					t.Fatal(err)
				}
			}
			reminderCommand(t, s, domain.ReminderCommand{Action: "complete", ID: p.ID, Revision: p.Revision}, completedAt)
			s = reopenReminderStore(t, s)
			state, err := s.ReminderSnapshot(t.Context(), 1, "companion", completedAt)
			if err != nil || len(state.Plans) != 1 || state.Plans[0].Status != "active" || !state.Plans[0].NextAt.Equal(due.AddDate(0, 0, 1)) {
				t.Fatalf("next day lost after completion: %+v, %v", state, err)
			}
			if err := s.TickReminders(t.Context(), due.AddDate(0, 0, 1), ""); err != nil {
				t.Fatal(err)
			}
			state, err = s.ReminderSnapshot(t.Context(), 1, "companion", due.AddDate(0, 0, 1))
			if err != nil || len(state.Occurrences) != 2 {
				t.Fatalf("next day did not trigger: %+v, %v", state, err)
			}
			for _, event := range state.Occurrences {
				if event.DueAt.Equal(due) && event.Status != "completed" || event.DueAt.Equal(due.AddDate(0, 0, 1)) && event.Status != "due" {
					t.Fatalf("wrong daily occurrence: %+v", event)
				}
			}
		})
	}
}

type interpretedReminderModel struct{ input, title string }

func (m interpretedReminderModel) CompanionMessages(context.Context, uint, uint, int) ([]domainchat.Message, error) {
	return nil, nil
}

func (m interpretedReminderModel) StreamTemporaryChat(context.Context, chat.TemporaryChatInput, func(string) error) (*chat.SendMessageResult, error) {
	raw, _ := json.Marshal(map[string]interface{}{"action": "create", "evidence": m.input, "title": m.title, "schedule": map[string]interface{}{"kind": "once", "delay_seconds": 30}})
	return &chat.SendMessageResult{AssistantMessage: domainchat.Message{Content: string(raw)}}, nil
}

func TestInterpretedSecondsReminderPersistsAndQueuesWhileChatBusy(t *testing.T) {
	for _, tt := range []struct{ input, title string }{{"30秒之后提醒我 测试", "测试"}, {"30秒后通知我写作业", "写作业"}} {
		t.Run(tt.input, func(t *testing.T) {
			s := reminderDeliveryFixture(t)
			if err := s.db.Exec("UPDATE companion_profiles SET model='configured-model' WHERE user_id=1").Error; err != nil {
				t.Fatal(err)
			}
			service := app.Service{Store: s, ReminderModel: interpretedReminderModel{input: tt.input, title: tt.title}}
			before := time.Now()
			created, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: tt.input})
			if err != nil || !created.Handled || len(created.Snapshot.Plans) != 1 {
				t.Fatalf("request was not saved: %+v, %v", created, err)
			}
			p := created.Snapshot.Plans[0]
			if p.Title != tt.title || p.NextAt.Before(before.Add(30*time.Second)) || p.NextAt.After(time.Now().Add(30*time.Second)) {
				t.Fatalf("incorrect saved reminder: %+v", p)
			}
			s = reopenReminderStore(t, s)
			lease, err := s.Acquire(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Release(1, lease)
			if err := s.TickReminders(t.Context(), p.NextAt.Add(-time.Second), "template"); err != nil {
				t.Fatal(err)
			}
			if delivery, _, err := s.ClaimReminderNotification(t.Context(), p.NextAt.Add(-time.Second), "template"); err != nil || delivery != nil {
				t.Fatalf("reminded early: %+v, %v", delivery, err)
			}
			if err := s.TickReminders(t.Context(), p.NextAt, "template"); err != nil {
				t.Fatal(err)
			}
			delivery, _, err := s.ClaimReminderNotification(t.Context(), p.NextAt, "template")
			if err != nil || delivery == nil || delivery.Title != tt.title || !delivery.DueAt.Equal(p.NextAt) {
				t.Fatalf("chat activity blocked the reminder: %+v, %v", delivery, err)
			}
		})
	}
}
