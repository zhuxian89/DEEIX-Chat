package companion

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	chat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	domainchat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/google/uuid"
)

type reminderModelStub struct {
	output   string
	err      error
	calls    int
	input    chat.TemporaryChatInput
	messages []domainchat.Message
	during   func()
}

func (m *reminderModelStub) StreamTemporaryChat(ctx context.Context, input chat.TemporaryChatInput, _ func(string) error) (*chat.SendMessageResult, error) {
	m.calls++
	m.input = input
	if m.during != nil {
		m.during()
	}
	return &chat.SendMessageResult{AssistantMessage: domainchat.Message{Content: m.output}}, m.err
}

func (m *reminderModelStub) CompanionMessages(context.Context, uint, uint, int) ([]domainchat.Message, error) {
	return m.messages, nil
}

func reminderLanguageFixture(t *testing.T) (*Service, *testRepository, *reminderModelStub) {
	t.Helper()
	store := testStore(t)
	if _, err := store.Ensure(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Exec("UPDATE companion_profiles SET model=?,conversation_id=1,conversation_public_id=? WHERE user_id=1", "configured-model", "owned").Error; err != nil {
		t.Fatal(err)
	}
	model := &reminderModelStub{}
	return &Service{Store: store, ReminderModel: model}, store, model
}

func modelReminderJSON(action, evidence, title, id string, schedule *reminderLanguageSchedule) string {
	c := reminderLanguageCommand{Action: action, Evidence: evidence, Title: title, ID: id, Schedule: schedule}
	if id != "" {
		c.Title, c.TargetTitle = "", title
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// These are integration contract tests with a model stub, not an evaluation of
// language comprehension. They prove that the actual command path uses model
// decisions, including inputs with no legacy parser keywords.
func TestReminderLanguageUsesModelForEveryTextAndSavesRealPlans(t *testing.T) {
	for _, test := range []struct{ input, title string }{{"30秒之后提醒我 测试", "测试"}, {"30秒后通知我写作业", "写作业"}, {"过半分钟喊我拿快递", "拿快递"}, {"待会儿喊我拿快递，半分钟就行", "拿快递"}} {
		input := test.input
		t.Run(input, func(t *testing.T) {
			service, store, model := reminderLanguageFixture(t)
			model.output = modelReminderJSON("create", input, test.title, "", &reminderLanguageSchedule{Kind: "once", DelaySeconds: 30})
			before := time.Now()
			request := domain.ReminderCommand{RequestID: uuid.NewString(), Input: input}
			result, err := service.ReminderCommand(t.Context(), 1, request)
			if err != nil || !result.Handled || len(result.Snapshot.Plans) != 1 || model.calls != 1 {
				t.Fatalf("model command was not saved: %+v, %v, calls=%d", result, err, model.calls)
			}
			plan := result.Snapshot.Plans[0]
			if plan.Title != test.title || plan.NextAt.Before(before.Add(30*time.Second)) || plan.NextAt.After(time.Now().Add(30*time.Second)) || !strings.Contains(result.Turn.Reply, "已设置") {
				t.Fatalf("saved result differs from interpreted action: %+v", result)
			}
			if model.input.Model != "configured-model" || model.input.UserID != 1 || len(model.input.SelectedToolIDs) != 0 {
				t.Fatal("wrong model routing or unexpected external tools")
			}
			model.err = errors.New("unavailable")
			replayed, err := service.ReminderCommand(t.Context(), 1, request)
			if err != nil || replayed.Turn.ID != result.Turn.ID || len(replayed.Snapshot.Plans) != 1 || model.calls != 1 {
				t.Fatal("retry inferred again or duplicated the reminder", err)
			}
			request.Input = "a different request"
			if _, err := service.ReminderCommand(t.Context(), 1, request); !errors.Is(err, ErrInvalid) {
				t.Fatal("same request ID accepted with different input", err)
			}
			state, _ := store.ReminderSnapshot(t.Context(), 1, "owned", time.Now())
			if len(state.Plans) != 1 {
				t.Fatal("retry changed persistent state")
			}
		})
	}
}

func TestReminderLanguageDoesNotOverrideOrdinaryChatOrModelFailure(t *testing.T) {
	service, store, model := reminderLanguageFixture(t)
	for _, input := range []string{"今天心情不错", "如果我说30秒后提醒我写作业呢", "别创建提醒", "你支持闹钟吗"} {
		model.output = `{"action":"none"}`
		result, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: input})
		if err != nil || result.Handled {
			t.Fatal("model ordinary-chat decision was overridden by keywords", input, result, err)
		}
	}
	for _, output := range []string{"not json", "null", `{"action":"create","evidence":"30秒后提醒我喝水","title":"喝水","schedule":{"kind":"once","delay_seconds":30},"user_id":2}`} {
		model.output = output
		if _, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: "30秒后提醒我喝水"}); err == nil {
			t.Fatal("invalid model result was accepted")
		}
	}
	model.output = modelReminderJSON("create", "30秒后提醒我喝水", "喝水", "", &reminderLanguageSchedule{Kind: "once", DelaySeconds: 30})
	model.err = context.DeadlineExceeded
	if _, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: "30秒后提醒我喝水"}); err == nil {
		t.Fatal("timed-out inference fell back to keywords")
	}
	state, _ := store.ReminderSnapshot(t.Context(), 1, "owned", time.Now())
	if len(state.Plans) != 0 || len(state.Turns) != 0 || model.calls != 8 {
		t.Fatal("ordinary chat or failed understanding changed reminders", state, model.calls)
	}
}

func TestReminderLanguageKeepsClarificationContextAndLifecycle(t *testing.T) {
	service, store, model := reminderLanguageFixture(t)
	command := func(input string) domain.ReminderResult {
		t.Helper()
		result, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: input})
		if err != nil || !result.Handled {
			t.Fatal("command failed", input, err)
		}
		return result
	}
	model.output = `{"action":"clarify","evidence":"记得喊我拿快递","question":"什么时候喊你拿快递？"}`
	result := command("记得喊我拿快递")
	if len(result.Snapshot.Plans) != 0 {
		t.Fatal("clarification created a task")
	}
	model.output = modelReminderJSON("create", "就三十秒吧", "拿快递", "", &reminderLanguageSchedule{Kind: "once", DelaySeconds: 30})
	result = command("就三十秒吧")
	if !strings.Contains(model.input.Messages[0].Content, "什么时候喊你拿快递") || !strings.Contains(model.input.Messages[0].Content, "记得喊我拿快递") {
		t.Fatal("clarification history was not supplied to the model")
	}
	plan := result.Snapshot.Plans[0]
	oldDue := plan.NextAt
	model.output = modelReminderJSON("update", "换成一分钟吧", plan.Title, plan.ID, &reminderLanguageSchedule{Kind: "once", DelaySeconds: 60})
	result = command("换成一分钟吧")
	if len(result.Snapshot.Plans) != 1 || !result.Snapshot.Plans[0].NextAt.After(oldDue) || result.Snapshot.Plans[0].ID != plan.ID {
		t.Fatal("contextual update duplicated or failed to change the task")
	}
	for _, action := range []string{"pause", "resume", "delete"} {
		input := map[string]string{"pause": "先停一下这个", "resume": "接着叫我吧", "delete": "算了不用了"}[action]
		model.output = modelReminderJSON(action, input, plan.Title, plan.ID, nil)
		result = command(input)
		if action == "delete" && len(result.Snapshot.Plans) != 0 {
			t.Fatal("deleted plan remained active")
		}
	}
	state, _ := store.ReminderSnapshot(t.Context(), 1, "owned", time.Now())
	if len(state.Turns) != 6 || len(state.Plans) != 0 {
		t.Fatal("conversation lifecycle was not persisted", state)
	}
}

func TestReminderLanguageRejectsForeignTargetsAndConcurrentEdits(t *testing.T) {
	service, store, model := reminderLanguageFixture(t)
	created, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Action: "create", Title: "吃药", Schedule: domain.ReminderSchedule{Kind: "daily", StartAt: time.Now(), Times: []int{480}}})
	if err != nil || model.calls != 0 {
		t.Fatal("card action depended on model", err)
	}
	plan := created.Snapshot.Plans[0]
	model.output = modelReminderJSON("delete", "取消这个", plan.Title, "another-users-id", nil)
	result, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: "取消这个"})
	if err != nil || !result.Handled || len(result.Snapshot.Plans) != 1 || len(result.Turn.ReminderIDs) != 0 {
		t.Fatal("foreign or fabricated target was acted upon", err)
	}
	model.output = modelReminderJSON("delete", "取消这个", plan.Title, plan.ID, nil)
	model.during = func() {
		_, err := store.ExecuteReminder(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Action: "pause", ID: plan.ID, Revision: plan.Revision}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: "取消这个"}); !errors.Is(err, ErrBusy) {
		t.Fatal("model overwrote an edit made during inference", err)
	}
	state, _ := store.ReminderSnapshot(t.Context(), 1, "owned", time.Now())
	if len(state.Plans) != 1 || state.Plans[0].Status != "paused" {
		t.Fatal("concurrent update was lost")
	}
}

func TestReminderLanguageCalendarValidationAndRequestTime(t *testing.T) {
	now := time.Date(2026, 10, 17, 23, 59, 45, 0, domain.ReminderZone)
	for _, test := range []struct {
		schedule reminderLanguageSchedule
		kind     string
		due      time.Time
	}{
		{reminderLanguageSchedule{Kind: "once", DelaySeconds: 30}, "once", now.Add(30 * time.Second)},
		{reminderLanguageSchedule{Kind: "once", DelaySeconds: 1}, "once", now.Add(6 * time.Second)},
		{reminderLanguageSchedule{Kind: "daily", Times: []string{"20:00"}}, "daily", time.Date(2026, 10, 18, 20, 0, 0, 0, domain.ReminderZone)},
		{reminderLanguageSchedule{Kind: "weekly", Times: []string{"08:00"}, Weekdays: []int{1, 2, 3, 4, 5}}, "weekly", time.Date(2026, 10, 19, 8, 0, 0, 0, domain.ReminderZone)},
	} {
		s, err := test.schedule.schedule(now, now.Add(5*time.Second))
		if err != nil || s.Kind != test.kind || !s.Next(now.Add(5*time.Second), 0).Equal(test.due) {
			t.Fatalf("wrong schedule: %+v, %v", s, err)
		}
	}
	for _, schedule := range []reminderLanguageSchedule{
		{Kind: "once"}, {Kind: "once", DelaySeconds: -30}, {Kind: "once", DelaySeconds: 367 * 86400},
		{Kind: "once", StartAt: "2026-10-17T08:00:00"}, {Kind: "once", StartAt: "2026-10-17T08:00:00+08:00"},
		{Kind: "once", DelaySeconds: 30, StartAt: "2026-10-18T08:00:00+08:00"},
		{Kind: "daily", Times: []string{"25:00"}}, {Kind: "daily", Times: []string{"08:00", "08:00"}},
		{Kind: "weekly", Times: []string{"08:00"}, Weekdays: []int{8}}, {Kind: "interval", IntervalMinutes: 0},
	} {
		if _, err := schedule.schedule(now, now); err == nil {
			t.Fatal("invalid model schedule was accepted", schedule)
		}
	}
}

func TestReminderLanguageHistoryRespectsForgettingAndOrdering(t *testing.T) {
	now := time.Now()
	profile := &Profile{UserID: 1, ForgetThroughID: 10}
	messages := []domainchat.Message{
		{ID: 11, UserID: 1, Role: "user", Content: "current context", Status: "success", CreatedAt: now.Add(-time.Minute)},
		{ID: 10, UserID: 1, Role: "user", Content: "forgotten secret", Status: "success"},
		{ID: 12, UserID: 2, Role: "user", Content: "foreign secret", Status: "success"},
		{ID: 13, UserID: 1, Role: "user", Content: "blocked secret", Status: "blocked"},
	}
	state := domain.ReminderSnapshot{Turns: []domain.ReminderTurn{{Input: "recent reminder", Reply: "when?", CreatedAt: now.Add(-time.Second)}, {Input: "stale clarification", CreatedAt: now.Add(-time.Hour)}}}
	prompt := reminderLanguagePrompt(profile, "current request", state, messages, now)
	for _, secret := range []string{"forgotten secret", "foreign secret", "blocked secret", "stale clarification"} {
		if strings.Contains(prompt, secret) {
			t.Fatal("ineligible history reached model", secret)
		}
	}
	if strings.Index(prompt, "current context") > strings.Index(prompt, "recent reminder") || !strings.Contains(prompt, "current request") {
		t.Fatal("context ordering or current request lost")
	}
}

func TestReminderLanguageSnoozeSecondsAndCompleteKeepDailyRecurrence(t *testing.T) {
	service, _, model := reminderLanguageFixture(t)
	result, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Action: "create", Title: "陪孩子写作业", Schedule: domain.ReminderSchedule{Kind: "daily", StartAt: time.Now(), Times: []int{1200}}})
	if err != nil {
		t.Fatal(err)
	}
	plan := result.Snapshot.Plans[0]
	input := "这一回晚半分钟叫我"
	raw, _ := json.Marshal(reminderLanguageCommand{Action: "snooze", Evidence: input, ID: plan.ID, TargetTitle: plan.Title, DelaySeconds: 30})
	model.output = string(raw)
	result, err = service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: input})
	if err != nil || len(result.Snapshot.Occurrences) != 1 || !result.Snapshot.Occurrences[0].DueAt.Equal(plan.NextAt.Add(30*time.Second)) || result.Snapshot.Plans[0].Schedule.Kind != "daily" {
		t.Fatal("seconds snooze changed the repeating schedule", result, err)
	}
	model.output = modelReminderJSON("complete", "这次做完了", plan.Title, plan.ID, nil)
	result, err = service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: "这次做完了"})
	if err != nil || len(result.Snapshot.Plans) != 1 || result.Snapshot.Plans[0].Status != "active" || !result.Snapshot.Plans[0].NextAt.After(plan.NextAt) || result.Snapshot.Occurrences[0].Status != "completed" {
		t.Fatal("completing this occurrence removed future reminders", result, err)
	}
}

func TestReminderLanguageRejectsUntrustedOutputAndChangedProfile(t *testing.T) {
	now := time.Now()
	state := domain.ReminderSnapshot{Plans: []domain.Reminder{{ID: "owned", Title: "宝宝的药", Revision: 3, Status: "active"}}}
	for _, raw := range []string{
		`{"action":"delete","evidence":"other input","id":"owned","target_title":"宝宝的药"}`,
		`{"action":"delete","evidence":"取消这个","id":"owned","target_title":"宝宝的药"} {"action":"none"}`,
		`{"action":"delete","evidence":"取消这个","id":"owned","target_title":"宝宝的药","revision":99}`,
		`{"action":"destroy","evidence":"取消这个","id":"owned"}`,
	} {
		if _, err := decodeReminderLanguage(raw, "取消这个", state, now, now); err == nil {
			t.Fatal("untrusted model output was accepted", raw)
		}
	}
	wrongTitle := modelReminderJSON("delete", "取消这个", "宝宝药", "owned", nil)
	if c, err := decodeReminderLanguage(wrongTitle, "取消这个", state, now, now); err != nil || c.Action != "clarify" {
		t.Fatal("mismatched literal title selected a different reminder", c, err)
	}
	for _, raw := range []string{
		`{"action":"list","evidence":"还有多久","id":"owned"}`,
		`{"action":"list","evidence":"还有多久","ids":["owned"],"id":"owned"}`,
	} {
		c, err := decodeReminderLanguage(raw, "还有多久", state, now, now)
		if err != nil || c.Action != "list" || len(c.IDs) != 1 || c.IDs[0] != "owned" {
			t.Fatal("single-target countdown became a query for all reminders", c, err)
		}
	}
	if c, err := decodeReminderLanguage(`{"action":"list","evidence":"还有多久","target_title":"不存在的提醒"}`, "还有多久", state, now, now); err != nil || c.Action != "clarify" {
		t.Fatal("unresolved target became a query for all reminders", c, err)
	}
	service, store, model := reminderLanguageFixture(t)
	model.output = modelReminderJSON("create", "半分钟后叫我拿快递", "拿快递", "", &reminderLanguageSchedule{Kind: "once", DelaySeconds: 30})
	model.during = func() {
		if err := store.db.Exec("UPDATE companion_profiles SET revision=revision+1 WHERE user_id=1").Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: "半分钟后叫我拿快递"}); !errors.Is(err, ErrBusy) {
		t.Fatal("changed profile was not rechecked after inference", err)
	}
	stored, _ := store.ReminderSnapshot(t.Context(), 1, "owned", now)
	if len(stored.Plans) != 0 {
		t.Fatal("stale inference wrote a reminder")
	}
}

func TestReminderLanguageRenameKeepsDueOccurrenceAndProgress(t *testing.T) {
	service, store, model := reminderLanguageFixture(t)
	// SQLite compares stored timestamps as text; keep the fixture clock in the
	// same UTC representation that ReminderSchedule.Next persists.
	now := time.Now().UTC()
	created, err := store.ExecuteReminder(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Action: "create", Title: "药", Schedule: domain.ReminderSchedule{Kind: "daily", StartAt: now.Add(-time.Hour), Times: []int{now.In(domain.ReminderZone).Hour()*60 + now.In(domain.ReminderZone).Minute()}, Limit: 3}}, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	plan := created.Snapshot.Plans[0]
	if err := store.TickReminders(t.Context(), now, ""); err != nil {
		t.Fatal(err)
	}
	before, _ := store.ReminderSnapshot(t.Context(), 1, "owned", now)
	if len(before.Occurrences) != 1 {
		t.Fatal("expected a due occurrence", before)
	}
	input := "把名字改成吃药"
	raw, _ := json.Marshal(reminderLanguageCommand{Action: "update", Evidence: input, ID: plan.ID, TargetTitle: plan.Title, Title: "吃药"})
	model.output = string(raw)
	result, err := service.ReminderCommand(t.Context(), 1, domain.ReminderCommand{RequestID: uuid.NewString(), Input: input})
	if err != nil || len(result.Snapshot.Occurrences) != 1 || len(result.Snapshot.Plans) != 1 {
		t.Fatal("rename failed", result, err)
	}
	got, event := result.Snapshot.Plans[0], result.Snapshot.Occurrences[0]
	if got.Title != "吃药" || got.Generated != before.Plans[0].Generated || got.ScheduleVersion != before.Plans[0].ScheduleVersion || !got.Cursor.Equal(before.Plans[0].Cursor) || event.ID != before.Occurrences[0].ID || event.Status != "due" || event.Title != "吃药" || !event.DueAt.Equal(before.Occurrences[0].DueAt) {
		t.Fatal("rename reset progress or canceled an existing occurrence", result)
	}
}
