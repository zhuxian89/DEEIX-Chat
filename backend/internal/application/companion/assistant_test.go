package companion

import (
	"context"
	"errors"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"testing"
	"time"
)

type followupSender struct {
	count int
	err   error
}

func (s *followupSender) Send(_ context.Context, _ domain.NotificationDelivery) error {
	s.count++
	return s.err
}

func TestAssistantWorkerRunsWithoutForegroundAndDoesNotRetryAmbiguousSend(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "sent", true: "uncertain"}[failure], func(t *testing.T) {
			store := testStore(t)
			_, _ = store.Ensure(t.Context(), 1)
			if err := store.db.Exec("UPDATE companion_profiles SET conversation_id=1 WHERE user_id=1").Error; err != nil {
				t.Fatal(err)
			}
			if err := store.db.Exec("CREATE TABLE wechat_miniapp_bindings(id integer PRIMARY KEY,user_id integer,app_id text,open_id text,revoked_at datetime,created_at datetime,updated_at datetime,deleted_at datetime)").Error; err != nil {
				t.Fatal(err)
			}
			if err := store.db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','owner')").Error; err != nil {
				t.Fatal(err)
			}
			store.SetAppIDProvider(func() string { return "app" })
			if err := store.SetNotificationSettings(t.Context(), 1, domain.NotificationSettings{Enabled: true, TemplateID: "template"}, true); err != nil {
				t.Fatal(err)
			}
			_, err := store.SaveFollowup(t.Context(), domain.Followup{UserID: 1, Title: "到期事项", DueAt: time.Now().Add(-time.Minute), Status: "pending", SourceKind: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			sender := &followupSender{}
			if failure {
				sender.err = errors.New("connection lost")
			}
			service := &Service{Store: store, Notifier: sender, NotificationTemplateID: "template"}
			for i := 0; i < 2; i++ {
				if _, err := service.TickAssistant(t.Context(), 0); err != nil {
					t.Fatal(err)
				}
			}
			state, err := service.State(t.Context(), 1)
			if err != nil || len(state.Initiatives) != 1 || sender.count != 1 {
				t.Fatal("worker did not persist exactly one message", err, sender.count)
			}
			items, _ := store.Followups(t.Context(), 1)
			if items[0].NotificationStatus != map[bool]string{false: "sent", true: "uncertain"}[failure] {
				t.Fatal("wrong notification outcome", items)
			}
		})
	}
}

func TestFollowupExtractionRequiresLiveEvidenceAndPreservesUserEdits(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	source := domain.ContextSource{ID: "message:21", Kind: "conversation", ConversationID: "owned", MessageID: 21, Text: "我明天上午九点参加面试", At: now}
	v := followupSuggestion{Title: "参加面试", DueAt: now.Add(24 * time.Hour).Format(time.RFC3339), Status: "pending", SourceID: source.ID, Evidence: "明天上午九点参加面试"}
	items := validatedFollowups(1, []followupSuggestion{v}, nil, []domain.ContextSource{source}, now)
	if len(items) != 1 || items[0].UserID != 1 || items[0].SourceConversationID != "owned" {
		t.Fatalf("valid plan lost: %+v", items)
	}
	for _, mode := range []string{"foreign source", "missing evidence", "past", "too distant", "user managed", "older correction", "foreign event"} {
		t.Run(mode, func(t *testing.T) {
			candidate := v
			existing := []domain.Followup{}
			switch mode {
			case "foreign source":
				candidate.SourceID = "message:foreign"
			case "missing evidence":
				candidate.Evidence = "从来没有说过的计划"
			case "past":
				candidate.DueAt = now.Add(-48 * time.Hour).Format(time.RFC3339)
			case "too distant":
				candidate.DueAt = now.Add(100 * 24 * time.Hour).Format(time.RFC3339)
			default:
				old := items[0]
				old.ID = "existing"
				candidate.ID = old.ID
				if mode == "user managed" {
					old.UserManaged = true
				}
				if mode == "older correction" {
					old.SourceAt = now.Add(time.Second)
				}
				if mode == "foreign event" {
					old.UserID = 2
				}
				existing = append(existing, old)
			}
			if got := validatedFollowups(1, []followupSuggestion{candidate}, existing, []domain.ContextSource{source}, now); len(got) != 0 {
				t.Fatalf("invalid plan accepted: %+v", got)
			}
		})
	}
	old := items[0]
	old.ID = "same-event"
	old.Revision = 3
	correction := source
	correction.ID = "message:22"
	correction.MessageID = 22
	correction.At = now.Add(time.Minute)
	correction.Text = "面试改到后天下午三点"
	v.ID, v.SourceID, v.Evidence, v.DueAt = old.ID, correction.ID, correction.Text, now.Add(48*time.Hour).Format(time.RFC3339)
	changed := validatedFollowups(1, []followupSuggestion{v}, []domain.Followup{old}, []domain.ContextSource{correction}, now)
	if len(changed) != 1 || changed[0].ID != old.ID || changed[0].Revision != 3 || changed[0].SourceMessageID != 22 {
		t.Fatal("reschedule created a new event or lost revision")
	}
	v.Status = "canceled"
	canceled := validatedFollowups(1, []followupSuggestion{v}, []domain.Followup{old}, []domain.ContextSource{correction}, now)
	if len(canceled) != 1 || canceled[0].Status != "canceled" {
		t.Fatal("cancellation lost")
	}
}

func TestFollowupExtractionDeduplicatesRepeatedPlan(t *testing.T) {
	now := time.Now()
	source := domain.ContextSource{ID: "message:1", Kind: "conversation", Text: "明天上午需要参加面试", At: now}
	v := followupSuggestion{Title: "参加面试", Status: "pending", DueAt: now.Add(time.Hour).Format(time.RFC3339), SourceID: source.ID, Evidence: source.Text}
	f := validatedFollowups(1, []followupSuggestion{v, v}, nil, []domain.ContextSource{source}, now)
	if len(f) != 1 {
		t.Fatal("duplicate model items were accepted")
	}
	source.ID = "message:2"
	v.SourceID = source.ID
	if len(validatedFollowups(1, []followupSuggestion{v}, f, []domain.ContextSource{source}, now)) != 0 {
		t.Fatal("same plan repeated in another message duplicated")
	}
}
