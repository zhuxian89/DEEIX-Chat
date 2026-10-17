package companion

import (
	"errors"
	"fmt"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"sync"
	"testing"
	"time"
)

func reminderCommand(t *testing.T, s *Store, c domain.ReminderCommand, now time.Time) domain.ReminderResult {
	t.Helper()
	if c.RequestID == "" {
		c.RequestID = fmt.Sprintf("request-%d", time.Now().UnixNano())
	}
	c.ConversationID = "companion"
	result, err := s.ExecuteReminder(t.Context(), 1, c, now)
	if err != nil {
		t.Fatal(c.Action, err)
	}
	return result
}
func TestReminderLifecycleIndependentOfChat(t *testing.T) {
	s, _ := assistantFixture(t)
	now := time.Date(2026, 10, 16, 7, 0, 0, 0, domain.ReminderZone)
	c := domain.ReminderCommand{RequestID: "create-medicine", Action: "create", Title: "吃药", Schedule: domain.ReminderSchedule{Kind: "daily", StartAt: now, Times: []int{480}}}
	created := reminderCommand(t, s, c, now)
	p := created.Snapshot.Plans[0]
	again := reminderCommand(t, s, c, now.Add(time.Minute))
	if again.Turn.ID != created.Turn.ID || len(again.Snapshot.Plans) != 1 {
		t.Fatal("retry duplicated")
	}
	c.Title = "different"
	c.ConversationID = "companion"
	if _, err := s.ExecuteReminder(t.Context(), 1, c, now); !errors.Is(err, ErrInvalid) {
		t.Fatal("request id reused with different input", err)
	}
	token, err := s.Acquire(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Release(1, token)
	// Hold the chat lease and quiet profile while the reminder becomes due.
	if err := s.db.Model(&profileRecord{}).Where("user_id = 1").Update("quiet", true).Error; err != nil {
		t.Fatal(err)
	}
	due := now.Add(time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.TickReminders(t.Context(), due, ""); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	state, err := s.ReminderSnapshot(t.Context(), 1, "companion", due)
	if err != nil || len(state.Occurrences) != 1 || state.Occurrences[0].Status != "due" {
		t.Fatalf("due event %+v %v", state, err)
	}
	done := reminderCommand(t, s, domain.ReminderCommand{Action: "complete", ID: p.ID, Revision: p.Revision}, due)
	p = done.Snapshot.Plans[0]
	if p.Status != "active" || domain.ReminderTime(p.NextAt) != "2026-10-17 08:00" {
		t.Fatal("completion lost next day", p)
	}
	snoozed := reminderCommand(t, s, domain.ReminderCommand{Action: "snooze", ID: p.ID, Revision: p.Revision, DelayMinutes: 10}, due)
	p = snoozed.Snapshot.Plans[0]
	if !p.NextAt.Equal(due.Add(24*time.Hour + 10*time.Minute)) {
		t.Fatal("snooze failed", p)
	}
	if got := domain.ReminderTime(p.Cursor); got != "2026-10-18 08:00" {
		t.Fatal("snooze shifted repeating clock", got)
	}
	paused := reminderCommand(t, s, domain.ReminderCommand{Action: "pause", ID: p.ID, Revision: p.Revision}, due)
	p = paused.Snapshot.Plans[0]
	if err := s.TickReminders(t.Context(), due.Add(10*time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	resumed := reminderCommand(t, s, domain.ReminderCommand{Action: "resume", ID: p.ID, Revision: p.Revision}, due.Add(20*time.Minute))
	p = resumed.Snapshot.Plans[0]
	if domain.ReminderTime(p.NextAt) != "2026-10-17 08:10" {
		t.Fatal(p)
	}
	deleted := reminderCommand(t, s, domain.ReminderCommand{Action: "delete", ID: p.ID, Revision: p.Revision}, due)
	if len(deleted.Snapshot.Plans) != 0 {
		t.Fatal("deleted plan visible")
	}
	if err := s.TickReminders(t.Context(), due.Add(24*time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	state, _ = s.ReminderSnapshot(t.Context(), 1, "companion", due)
	for _, e := range state.Occurrences {
		if e.Status == "pending" || e.Status == "due" {
			t.Fatal("deleted reminder survived", e)
		}
	}
}

func TestReminderCompletionMatchesNextTimeBeforeLaterSnoozedOccurrence(t *testing.T) {
	s, _ := assistantFixture(t)
	now := time.Date(2026, 10, 17, 7, 0, 0, 0, domain.ReminderZone)
	created := reminderCommand(t, s, domain.ReminderCommand{Action: "create", Title: "medicine", Schedule: domain.ReminderSchedule{Kind: "daily", StartAt: now, Times: []int{480, 540}}}, now)
	p := created.Snapshot.Plans[0]
	snoozed := reminderCommand(t, s, domain.ReminderCommand{Action: "snooze", ID: p.ID, Revision: p.Revision, DelayMinutes: 120}, now)
	p = snoozed.Snapshot.Plans[0]
	if !p.NextAt.Equal(now.Add(2 * time.Hour)) {
		t.Fatal("next calendar time was hidden by later snooze", p.NextAt)
	}
	snoozedID := snoozed.Snapshot.Occurrences[0].ID
	done := reminderCommand(t, s, domain.ReminderCommand{Action: "complete", ID: p.ID, Revision: p.Revision}, now)
	if len(done.Snapshot.Occurrences) != 2 || !done.Snapshot.Plans[0].NextAt.Equal(now.Add(3*time.Hour)) {
		t.Fatal("completion did not advance to the snoozed occurrence", done.Snapshot)
	}
	for _, e := range done.Snapshot.Occurrences {
		if e.ID == snoozedID {
			if e.Status != "pending" {
				t.Fatal("completed the later snooze instead of the displayed next time", e)
			}
		} else if e.Status != "completed" || !e.ScheduledAt.Equal(now.Add(2*time.Hour)) {
			t.Fatal("wrong calendar occurrence completed", e)
		}
	}
}

func TestReminderOwnershipRevisionsAndRestart(t *testing.T) {
	s, db := assistantFixture(t)
	now := time.Now().UTC()
	result := reminderCommand(t, s, domain.ReminderCommand{Action: "create", Title: "喝水", Schedule: domain.ReminderSchedule{Kind: "once", StartAt: now.Add(time.Minute)}}, now)
	p := result.Snapshot.Plans[0]
	c := domain.ReminderCommand{RequestID: "foreign-delete", Action: "delete", ID: p.ID, Revision: p.Revision}
	if _, err := s.ExecuteReminder(t.Context(), 2, c, now); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign mutation", err)
	}
	c.RequestID = "stale-mutation"
	c.Revision = 99
	if _, err := s.ExecuteReminder(t.Context(), 1, c, now); !errors.Is(err, ErrBusy) {
		t.Fatal("stale overwrite", err)
	}
	restarted, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.TickReminders(t.Context(), now.Add(30*time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	state, _ := restarted.ReminderSnapshot(t.Context(), 1, "companion", now)
	if len(state.Occurrences) != 1 || state.Occurrences[0].Status != "missed" || state.Plans[0].Status != "ended" {
		t.Fatal("missed restart", state)
	}
}

func TestReminderOutboxDeleteAndCrashRecovery(t *testing.T) {
	s, db := assistantFixture(t)
	now := time.Now().UTC()
	if err := db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','owner')").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotificationSettings(t.Context(), 1, domain.NotificationSettings{Enabled: true, TemplateID: "template"}, true); err != nil {
		t.Fatal(err)
	}
	result := reminderCommand(t, s, domain.ReminderCommand{Action: "create", Title: "跳绳", Schedule: domain.ReminderSchedule{Kind: "once", StartAt: now.Add(time.Minute)}}, now)
	p := result.Snapshot.Plans[0]
	due := now.Add(time.Minute)
	if err := s.TickReminders(t.Context(), due, "template"); err != nil {
		t.Fatal(err)
	}
	reminderCommand(t, s, domain.ReminderCommand{Action: "delete", ID: p.ID, Revision: p.Revision}, due)
	d, _, err := s.ClaimReminderNotification(t.Context(), due, "template")
	if err != nil || d != nil {
		t.Fatal("deleted queued send", err, d)
	}
	result = reminderCommand(t, s, domain.ReminderCommand{Action: "create", Title: "喝水", Schedule: domain.ReminderSchedule{Kind: "once", StartAt: due.Add(time.Minute)}}, due)
	due = due.Add(time.Minute)
	if err := s.TickReminders(t.Context(), due, "template"); err != nil {
		t.Fatal(err)
	}
	d, _, err = s.ClaimReminderNotification(t.Context(), due, "template")
	if err != nil || d == nil {
		t.Fatal("missing notification", err)
	}
	again, _, err := s.ClaimReminderNotification(t.Context(), due, "template")
	if err != nil || again != nil {
		t.Fatal("duplicate dispatch", err)
	}
	if _, _, err := s.ClaimReminderNotification(t.Context(), due.Add(2*time.Minute), "template"); err != nil {
		t.Fatal(err)
	}
	state, _ := s.ReminderSnapshot(t.Context(), 1, "companion", due)
	found := false
	for _, e := range state.Occurrences {
		if e.ReminderID == result.Snapshot.Plans[0].ID {
			found = true
			if e.NotificationStatus != "uncertain" {
				t.Fatal("crash state not recovered", e)
			}
		}
	}
	if !found {
		t.Fatal("lost event")
	}
}

func TestReminderNotificationContinuesAfterExpiredCandidateBatch(t *testing.T) {
	s, db := assistantFixture(t)
	now := time.Now().UTC()
	if err := db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','owner')").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.SetNotificationSettings(t.Context(), 1, domain.NotificationSettings{Enabled: true, TemplateID: "template"}, true); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		reminderCommand(t, s, domain.ReminderCommand{Action: "create", Title: "earlier", Schedule: domain.ReminderSchedule{Kind: "once", StartAt: now.Add(time.Minute)}}, now)
	}
	if err := s.TickReminders(t.Context(), now.Add(time.Minute), "template"); err != nil {
		t.Fatal(err)
	}
	due := now.Add(20 * time.Minute)
	reminderCommand(t, s, domain.ReminderCommand{Action: "create", Title: "current", Schedule: domain.ReminderSchedule{Kind: "once", StartAt: due}}, now)
	if err := s.TickReminders(t.Context(), due, "template"); err != nil {
		t.Fatal(err)
	}
	d, more, err := s.ClaimReminderNotification(t.Context(), due, "template")
	if err != nil || d != nil || !more {
		t.Fatalf("expired batch hid later work: delivery=%v more=%v err=%v", d, more, err)
	}
	d, _, err = s.ClaimReminderNotification(t.Context(), due, "template")
	if err != nil || d == nil || d.Title != "current" {
		t.Fatalf("later authorized reminder was not claimed: delivery=%v err=%v", d, err)
	}
	d, more, err = s.ClaimReminderNotification(t.Context(), due, "template")
	if err != nil || d != nil || more {
		t.Fatalf("empty queue reported pending work: delivery=%v more=%v err=%v", d, more, err)
	}
}

func TestReminderCapacity500Users5000Plans(t *testing.T) {
	s, db := assistantFixture(t)
	now := time.Now().UTC()
	due := now.Add(time.Minute)
	profiles := []profileRecord{}
	owners := []reminderOwner{}
	plans := []reminderRecord{}
	for user := uint(1); user <= 500; user++ {
		if user > 2 {
			profiles = append(profiles, profileRecord{UserID: user})
		}
		owners = append(owners, reminderOwner{UserID: user})
		for n := 0; n < 10; n++ {
			at := due.Add(time.Duration(n) * time.Hour)
			p := domain.Reminder{ID: fmt.Sprintf("capacity-%d-%d", user, n), UserID: user, Title: "按时休息", Status: "active", Schedule: domain.ReminderSchedule{Kind: "once", StartAt: at}, Cursor: at, Revision: 1, CreatedAt: now}
			plans = append(plans, reminderRecord{ID: p.ID, UserID: user, Status: p.Status, Cursor: p.Cursor, Data: reminderJSON(p)})
		}
	}
	if err := db.CreateInBatches(profiles, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CreateInBatches(owners, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.CreateInBatches(plans, 100).Error; err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.TickReminders(t.Context(), due, ""); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var events []reminderEvent
	if err := db.Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 500 {
		t.Fatalf("expected 500 simultaneous events, got %d", len(events))
	}
	seen := map[uint]bool{}
	for _, e := range events {
		if seen[e.UserID] || e.Status != "due" {
			t.Fatal("duplicate or missed", e)
		}
		seen[e.UserID] = true
	}
	elapsed := time.Since(start)
	t.Logf("500 users / 5000 plans / 500 simultaneous due / 2 schedulers: %s", elapsed)
	state, err := s.ReminderSnapshot(t.Context(), 23, "", due)
	if err != nil || len(state.Plans) != 10 || len(state.Occurrences) != 1 {
		t.Fatal("owner isolation", state, err)
	}
}
