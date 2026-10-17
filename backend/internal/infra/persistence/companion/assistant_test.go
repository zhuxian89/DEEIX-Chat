package companion

import (
	"encoding/json"
	"errors"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	todo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	"gorm.io/gorm"
	"sync"
	"testing"
	"time"
)

func assistantFixture(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	db := testDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE chat_conversations (id integer PRIMARY KEY,user_id integer,public_id text,title text,created_at datetime,updated_at datetime,deleted_at datetime)`,
		`CREATE TABLE chat_messages (id integer PRIMARY KEY,user_id integer,conversation_id integer,parent_message_id integer,role text,status text,content text,created_at datetime,deleted_at datetime)`,
		`CREATE TABLE wechat_miniapp_bindings (id integer PRIMARY KEY,user_id integer,app_id text,open_id text,revoked_at datetime,created_at datetime,updated_at datetime,deleted_at datetime)`,
		`CREATE TABLE miniapp_todo_tasks (app_id text,owner_open_id text,id text,data text,updated_at datetime,deleted_at datetime,PRIMARY KEY(app_id,owner_open_id,id))`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []uint{1, 2} {
		if _, err := store.Ensure(t.Context(), user); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&profileRecord{}).Where("user_id = 1").Updates(map[string]interface{}{"conversation_id": 1, "conversation_public_id": "companion"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Hour)
	for _, c := range []struct {
		id, user      int
		public, title string
	}{{1, 1, "companion", "小禾"}, {2, 1, "travel", "旅行计划"}, {3, 2, "foreign", "隐私"}} {
		if err := db.Exec("INSERT INTO chat_conversations(id,user_id,public_id,title,created_at,updated_at) VALUES(?,?,?,?,?,?)", c.id, c.user, c.public, c.title, now, now).Error; err != nil {
			t.Fatal(err)
		}
	}
	store.SetAppIDProvider(func() string { return "app" })
	return store, db
}
func sourceMessage(t *testing.T, db *gorm.DB, id, user, conv int, parent interface{}, text string) {
	t.Helper()
	if err := db.Exec("INSERT INTO chat_messages(id,user_id,conversation_id,parent_message_id,role,status,content,created_at) VALUES(?,?,?,?,?,?,?,?)", id, user, conv, parent, "user", "success", text, time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
}

func TestGlobalSourcesRespectOwnerBranchExclusionsAndForget(t *testing.T) {
	store, db := assistantFixture(t)
	sourceMessage(t, db, 10, 1, 1, nil, "我喜欢茶")
	sourceMessage(t, db, 20, 1, 2, nil, "旧版本的旅行")
	sourceMessage(t, db, 21, 1, 2, nil, "明天去北京旅行")
	sourceMessage(t, db, 30, 2, 3, nil, "别人的秘密")
	sources, err := store.ContextSources(t.Context(), 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("wrong sources: %+v", sources)
	}
	for _, s := range sources {
		if s.MessageID == 20 || s.MessageID == 30 {
			t.Fatal("old branch/foreign source leaked")
		}
	}
	found, err := store.ContextSources(t.Context(), 1, "北京")
	if err != nil || len(found) != 1 || found[0].MessageID != 21 {
		t.Fatalf("keyword lookup: %+v %v", found, err)
	}
	if err := store.SetContextPreferences(t.Context(), 1, domain.ContextPreferences{Enabled: true, Excluded: []string{"foreign"}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("foreign exclusion accepted")
	}
	if err := store.SetContextPreferences(t.Context(), 1, domain.ContextPreferences{Enabled: true, Excluded: []string{"travel"}}); err != nil {
		t.Fatal(err)
	}
	sources, err = store.ContextSources(t.Context(), 1, "")
	if err != nil || len(sources) != 1 || sources[0].MessageID != 10 {
		t.Fatal("exclusion not respected", err, sources)
	}
	if err := store.SetContextPreferences(t.Context(), 1, domain.ContextPreferences{Enabled: false, Excluded: []string{}}); err != nil {
		t.Fatal(err)
	}
	sources, err = store.ContextSources(t.Context(), 1, "")
	if err != nil || len(sources) != 1 {
		t.Fatal("disabled global context still read others", err, sources)
	}
	if err := store.Forget(t.Context(), 1, 10, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.SetContextPreferences(t.Context(), 1, domain.ContextPreferences{Enabled: true, Excluded: []string{}}); err != nil {
		t.Fatal(err)
	}
	sources, err = store.ContextSources(t.Context(), 1, "")
	if err != nil || len(sources) != 0 {
		t.Fatal("forgotten context reentered", err, sources)
	}
}

func TestFollowupDeliveryDeduplicatesAndRearmsOnlyForReschedule(t *testing.T) {
	store, _ := assistantFixture(t)
	now := time.Now()
	f, err := store.SaveFollowup(t.Context(), domain.Followup{UserID: 1, Title: "出发", DueAt: now.Add(-time.Minute), Status: "pending", SourceKind: "manual", UserManaged: true})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 6; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := store.PublishDueFollowup(t.Context(), 1, now, "")
			if err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	items, _ := store.Initiatives(t.Context(), 1, 1)
	if len(items) != 1 || items[0].Kind != "followup" {
		t.Fatalf("delivery duplicated or lost: %+v", items)
	}
	all, _ := store.Followups(t.Context(), 1)
	f = all[0]
	if f.DeliveredAt.IsZero() || f.NotificationStatus != "not_configured" {
		t.Fatal("in-app delivery depends on template")
	}
	stale := f
	f.Title = "去机场"
	f, err = store.SaveFollowup(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveFollowup(t.Context(), stale); !errors.Is(err, ErrBusy) {
		t.Fatal("stale revision overwrote latest")
	}
	if _, err := store.PublishDueFollowup(t.Context(), 1, now, ""); err != nil {
		t.Fatal(err)
	}
	items, _ = store.Initiatives(t.Context(), 1, 1)
	if len(items) != 1 {
		t.Fatal("rename sent twice")
	}
	f.DueAt = now.Add(time.Hour)
	f, err = store.SaveFollowup(t.Context(), f)
	if err != nil || !f.DeliveredAt.IsZero() {
		t.Fatal("reschedule did not rearm", err)
	}
	if _, err := store.PublishDueFollowup(t.Context(), 1, now.Add(2*time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	items, _ = store.Initiatives(t.Context(), 1, 1)
	if len(items) != 2 {
		t.Fatal("rescheduled reminder missing")
	}
}

func TestConversationFollowupWaitsForLatestCorrectionsAndRejectsDeletedBranches(t *testing.T) {
	store, db := assistantFixture(t)
	now := time.Now()
	sourceMessage(t, db, 20, 1, 2, nil, "今天需要参加面试")
	sources, _ := store.ContextSources(t.Context(), 1, "")
	src := sources[0]
	f, err := store.SaveFollowup(t.Context(), domain.Followup{UserID: 1, Title: "面试", DueAt: now.Add(-time.Minute), Status: "pending", SourceKind: "conversation", SourceID: src.ID, SourceConversationID: src.ConversationID, SourceMessageID: src.MessageID, SourceAt: src.At})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishDueFollowup(t.Context(), 1, now, ""); err != nil {
		t.Fatal(err)
	}
	items, _ := store.Initiatives(t.Context(), 1, 1)
	if len(items) != 0 {
		t.Fatal("unprocessed correction window was ignored")
	}
	if err := store.CompleteFollowupRefresh(t.Context(), 1, domain.ContextDigest(sources)); err != nil {
		t.Fatal(err)
	}
	sourceMessage(t, db, 21, 1, 2, nil, "面试已取消") // An edited branch replaces message 20.
	contextItems, err := store.ContextFollowups(t.Context(), 1)
	if err != nil || len(contextItems) != 0 {
		t.Fatal("abandoned source reentered prompt through a reminder", err)
	}
	sources, _ = store.ContextSources(t.Context(), 1, "")
	_ = store.CompleteFollowupRefresh(t.Context(), 1, domain.ContextDigest(sources))
	if _, err := store.PublishDueFollowup(t.Context(), 1, now, ""); err != nil {
		t.Fatal(err)
	}
	items, _ = store.Initiatives(t.Context(), 1, 1)
	if len(items) != 0 {
		t.Fatal("abandoned branch reminded")
	}
	all, _ := store.Followups(t.Context(), 1)
	if all[0].ID != f.ID || all[0].Status != "canceled" {
		t.Fatal("stale source not canceled")
	}
}

func TestDeletedSourcesAndInactiveAccountsCannotProduceReminders(t *testing.T) {
	store, db := assistantFixture(t)
	now := time.Now()
	sourceMessage(t, db, 20, 1, 2, nil, "明天需要去北京")
	sources, _ := store.ContextSources(t.Context(), 1, "")
	src := sources[0]
	f, err := store.SaveFollowup(t.Context(), domain.Followup{UserID: 1, Title: "去北京", DueAt: now, Status: "pending", SourceKind: "conversation", SourceID: src.ID, SourceConversationID: src.ConversationID, SourceMessageID: src.MessageID, SourceAt: src.At})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_conversations SET deleted_at=? WHERE id=2", now).Error; err != nil {
		t.Fatal(err)
	}
	contexts, err := store.ContextFollowups(t.Context(), 1)
	if err != nil || len(contexts) != 0 {
		t.Fatal("deleted source leaked into context")
	}
	sources, _ = store.ContextSources(t.Context(), 1, "")
	_ = store.CompleteFollowupRefresh(t.Context(), 1, domain.ContextDigest(sources))
	if _, err := store.PublishDueFollowup(t.Context(), 1, now, ""); err != nil {
		t.Fatal(err)
	}
	items, _ := store.Followups(t.Context(), 1)
	if items[0].ID != f.ID || items[0].Status != "canceled" {
		t.Fatal("deleted source still scheduled")
	}
	if err := db.Exec("CREATE TABLE identity_users(id integer PRIMARY KEY,status text,deleted_at datetime)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO identity_users(id,status) VALUES(1,'deactivated'),(2,'active')").Error; err != nil {
		t.Fatal(err)
	}
	users, err := store.AssistantUsers(t.Context(), 0, 10)
	if err != nil || len(users) != 0 {
		t.Fatal("inactive account selected", err, users)
	}
	_, err = store.SaveFollowup(t.Context(), domain.Followup{UserID: 1, Title: "manual", DueAt: now, Status: "pending", SourceKind: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishDueFollowup(t.Context(), 1, now, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal("inactive account received a reminder", err)
	}
	if err := store.PurgeDeletedUsers(t.Context()); err != nil {
		t.Fatal(err)
	}
	items, _ = store.Followups(t.Context(), 1)
	if len(items) != 0 {
		t.Fatal("orphan followups not removed")
	}
}

func TestTodoSyncHandlesDatesCompletionDeletionAndAccountBinding(t *testing.T) {
	store, db := assistantFixture(t)
	now := time.Now()
	if err := db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','owner'),(2,'app','other')").Error; err != nil {
		t.Fatal(err)
	}
	task := todo.Task{ID: "task", Title: "提交材料", DueDate: now.Add(time.Hour).In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02"), DueTime: now.Add(time.Hour).In(time.FixedZone("UTC+8", 8*3600)).Format("15:04"), Timezone: "Asia/Shanghai", UpdatedAt: now, Version: 1}
	save := func(owner string) {
		t.Helper()
		data, _ := json.Marshal(task)
		if err := db.Exec("INSERT OR REPLACE INTO miniapp_todo_tasks(app_id,owner_open_id,id,data,updated_at) VALUES('app',?,?,?,?)", owner, task.ID, string(data), now).Error; err != nil {
			t.Fatal(err)
		}
	}
	save("owner")
	task.Title = "他人的秘密"
	save("other")
	if err := store.SyncTodoFollowups(t.Context(), 1, now); err != nil {
		t.Fatal(err)
	}
	items, _ := store.Followups(t.Context(), 1)
	if len(items) != 1 || items[0].Title != "提交材料" {
		t.Fatal("TODO ownership failed", items)
	}
	original := items[0].ID
	task.Title = "提交材料"
	task.DueDate = now.Add(24 * time.Hour).In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
	task.Version++
	save("owner")
	if err := store.SyncTodoFollowups(t.Context(), 1, now); err != nil {
		t.Fatal(err)
	}
	items, _ = store.Followups(t.Context(), 1)
	if len(items) != 1 || items[0].ID != original || items[0].Revision != 2 {
		t.Fatal("TODO change duplicated event")
	}
	task.CompletedAt = &now
	save("owner")
	if err := store.SyncTodoFollowups(t.Context(), 1, now); err != nil {
		t.Fatal(err)
	}
	items, _ = store.Followups(t.Context(), 1)
	if items[0].Status != "completed" {
		t.Fatal("completion not propagated")
	}
	task.CompletedAt = nil
	save("owner")
	_ = store.SyncTodoFollowups(t.Context(), 1, now)
	if err := db.Exec("DELETE FROM miniapp_todo_tasks WHERE owner_open_id='owner'").Error; err != nil {
		t.Fatal(err)
	}
	_ = store.SyncTodoFollowups(t.Context(), 1, now)
	items, _ = store.Followups(t.Context(), 1)
	if items[0].Status != "canceled" {
		t.Fatal("deleted TODO still active")
	}
}

func TestNotificationConsentIsConsumedAtomicallyAndQuietPausesDelivery(t *testing.T) {
	store, db := assistantFixture(t)
	now := time.Now()
	if err := db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','owner')").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.SetNotificationSettings(t.Context(), 1, domain.NotificationSettings{Enabled: true, TemplateID: "template", AppID: "previous-app"}, true); err == nil {
		t.Fatal("consent from an outdated runtime was accepted for another app")
	}
	for i := 0; i < 3; i++ {
		if err := store.SetNotificationSettings(t.Context(), 1, domain.NotificationSettings{Enabled: true, TemplateID: "template"}, true); err != nil {
			t.Fatal(err)
		}
	}
	n, _ := store.NotificationSettings(t.Context(), 1)
	if n.Credits != 1 {
		t.Fatal("unverified grants accumulated")
	}
	_, err := store.SaveFollowup(t.Context(), domain.Followup{UserID: 1, Title: "提交", DueAt: now, Status: "pending", SourceKind: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetProactivity(t.Context(), 1, "off"); err != nil {
		t.Fatal(err)
	}
	delivery, err := store.PublishDueFollowup(t.Context(), 1, now, "template")
	if err != nil || delivery != nil {
		t.Fatal("quiet ignored", err)
	}
	_ = store.SetProactivity(t.Context(), 1, "normal")
	delivery, err = store.PublishDueFollowup(t.Context(), 1, now, "template")
	if err != nil || delivery == nil || delivery.OpenID != "owner" {
		t.Fatal("notification missing", err)
	}
	again, err := store.PublishDueFollowup(t.Context(), 1, now, "template")
	if err != nil || again != nil {
		t.Fatal("duplicate notification")
	}
	n, _ = store.NotificationSettings(t.Context(), 1)
	if n.Credits != 0 {
		t.Fatal("credit not consumed")
	}
	if err := store.FinishNotification(t.Context(), delivery.ID, "uncertain"); err != nil {
		t.Fatal(err)
	}
	again, _ = store.PublishDueFollowup(t.Context(), 1, now.Add(time.Minute), "template")
	if again != nil {
		t.Fatal("ambiguous send was retried")
	}
	items, _ := store.Followups(t.Context(), 2)
	if len(items) != 0 {
		t.Fatal("cross-account event leaked")
	}
}

func TestReminderWriteFailureRollsBackMessageAndNotificationCredit(t *testing.T) {
	store, db := assistantFixture(t)
	now := time.Now()
	if err := db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','owner')").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.SetNotificationSettings(t.Context(), 1, domain.NotificationSettings{Enabled: true, TemplateID: "template"}, true); err != nil {
		t.Fatal(err)
	}
	_, err := store.SaveFollowup(t.Context(), domain.Followup{UserID: 1, Title: "提交", DueAt: now, Status: "pending", SourceKind: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_delivery BEFORE UPDATE OF delivered_at ON companion_followups BEGIN SELECT RAISE(ABORT,'simulated delivery failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	delivery, err := store.PublishDueFollowup(t.Context(), 1, now, "template")
	if err == nil || delivery != nil {
		t.Fatal("failed transaction exposed a notification")
	}
	n, _ := store.NotificationSettings(t.Context(), 1)
	items, _ := store.Initiatives(t.Context(), 1, 1)
	if n.Credits != 1 || len(items) != 0 {
		t.Fatal("partial message/credit committed")
	}
	if err := db.Exec("DROP TRIGGER reject_delivery").Error; err != nil {
		t.Fatal(err)
	}
	delivery, err = store.PublishDueFollowup(t.Context(), 1, now, "template")
	if err != nil || delivery == nil {
		t.Fatal("delivery did not recover", err)
	}
}

func TestUnavailableSourceDoesNotCancelFollowup(t *testing.T) {
	store, db := assistantFixture(t)
	now := time.Now()
	sourceMessage(t, db, 20, 1, 2, nil, "需要按时提交材料")
	sources, _ := store.ContextSources(t.Context(), 1, "")
	src := sources[0]
	_, err := store.SaveFollowup(t.Context(), domain.Followup{UserID: 1, Title: "材料", DueAt: now, Status: "pending", SourceKind: "conversation", SourceID: src.ID, SourceConversationID: src.ConversationID, SourceMessageID: src.MessageID, SourceAt: src.At})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP TABLE chat_messages").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteFollowupRefresh(t.Context(), 1, domain.ContextDigest(nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishDueFollowup(t.Context(), 1, now, ""); err == nil {
		t.Fatal("source failure silently consumed event")
	}
	items, _ := store.Followups(t.Context(), 1)
	if items[0].Status != "pending" || !items[0].DeliveredAt.IsZero() {
		t.Fatal("temporary source outage canceled reminder")
	}
}

func TestGlobalKeywordLookupCanRecallOlderLiveMessages(t *testing.T) {
	store, db := assistantFixture(t)
	sourceMessage(t, db, 10, 1, 2, nil, "我要参加杭州演讲")
	for i := 11; i < 45; i++ {
		sourceMessage(t, db, i, 1, 2, i-1, "最近在看电影")
	}
	sources, err := store.ContextSources(t.Context(), 1, "杭州演讲")
	if err != nil || len(sources) != 1 || sources[0].MessageID != 10 {
		t.Fatal("older matching source was lost behind recent window", err, sources)
	}
}

func TestRebindingWechatDoesNotTransferNotificationConsentOrOldTodo(t *testing.T) {
	store, db := assistantFixture(t)
	now := time.Now()
	if err := db.Exec("INSERT INTO wechat_miniapp_bindings(user_id,app_id,open_id) VALUES(1,'app','first-owner')").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.SetNotificationSettings(t.Context(), 1, domain.NotificationSettings{Enabled: true, TemplateID: "template"}, true); err != nil {
		t.Fatal(err)
	}
	task := todo.Task{ID: "same-task-id", Title: "原身份的事项", DueDate: now.Add(time.Hour).In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02"), DueTime: now.Add(time.Hour).In(time.FixedZone("UTC+8", 8*3600)).Format("15:04"), Timezone: "Asia/Shanghai", UpdatedAt: now}
	for _, owner := range []string{"first-owner", "second-owner"} {
		data, _ := json.Marshal(task)
		if err := db.Exec("INSERT INTO miniapp_todo_tasks(app_id,owner_open_id,id,data,updated_at) VALUES('app',?,?,?,?)", owner, task.ID, string(data), now).Error; err != nil {
			t.Fatal(err)
		}
		task.Title = "新身份的事项"
	}
	if err := store.SyncTodoFollowups(t.Context(), 1, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE wechat_miniapp_bindings SET open_id='second-owner' WHERE user_id=1").Error; err != nil {
		t.Fatal(err)
	}
	n, err := store.NotificationSettings(t.Context(), 1)
	if err != nil || n.Enabled || n.Credits != 0 {
		t.Fatal("old consent transferred to new binding", err)
	}
	if err := store.SyncTodoFollowups(t.Context(), 1, now); err != nil {
		t.Fatal(err)
	}
	items, _ := store.Followups(t.Context(), 1)
	pending := 0
	for _, item := range items {
		if item.Status == "pending" {
			pending++
			if item.Title != "新身份的事项" {
				t.Fatal("old identity TODO stayed active")
			}
		}
	}
	if len(items) != 2 || pending != 1 {
		t.Fatal("same task ID across bindings was confused", items)
	}
}
