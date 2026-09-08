package companion

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	companionstore "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/companion"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testRepository struct {
	*companionstore.Store
	db *gorm.DB
}

var _ Store = (*companionstore.Store)(nil)

func testStore(t *testing.T) *testRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "companion.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	store, err := companionstore.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return &testRepository{Store: store, db: db}
}

func TestCompanionRenameReadsLegacyGreetingWithoutRewritingStoredContext(t *testing.T) {
	store := testStore(t)
	ctx := t.Context()
	if _, err := store.Ensure(ctx, 7); err != nil {
		t.Fatal(err)
	}
	const oldGreeting = "早呀。我是小伴，一个 AI 聊天伙伴。不用想好问题，随口说点什么也可以。"
	const summary = "用户说自己的宠物叫小伴。"
	now := time.Now()
	if err := store.db.Table("companion_profiles").Where("user_id = ?", 7).Updates(map[string]interface{}{
		"last_greeting": oldGreeting, "summary": summary, "summary_at": now,
		"model": "configured-model", "conversation_public_id": "existing-conversation",
	}).Error; err != nil {
		t.Fatal(err)
	}
	state, err := (&Service{Store: store}).State(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if state.Name != Name || state.Model != "configured-model" || state.ConversationPublicID != "existing-conversation" || state.Greeting != strings.Replace(oldGreeting, "我是小伴", "我是"+Name, 1) {
		t.Fatalf("legacy state lost identity or conversation continuity: %+v", state)
	}
	profile, err := store.Profile(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	if profile.LastGreeting != oldGreeting || profile.Summary != summary {
		t.Fatal("reading legacy state rewrote stored context")
	}
	prompt := BuildPrompt(profile, nil, now)
	if !strings.Contains(prompt, state.Greeting) || !strings.Contains(prompt, summary) {
		t.Fatal("prompt did not preserve user context alongside the current greeting")
	}
}

func TestCompanionLeaseSerializesConcurrentDevices(t *testing.T) {
	s := testStore(t)
	if _, err := s.Ensure(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ensure(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var token string
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := s.Acquire(context.Background(), 7)
			if err == nil {
				wins.Add(1)
				token = value
			} else if !errors.Is(err, ErrBusy) {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if wins.Load() != 1 {
		t.Fatalf("got %d lease owners", wins.Load())
	}
	s.Release(7, "old-token")
	if _, err := s.Acquire(t.Context(), 7); !errors.Is(err, ErrBusy) {
		t.Fatalf("wrong token released lease: %v", err)
	}
	if err := s.Renew(t.Context(), 7, token); err != nil {
		t.Fatal(err)
	}
	s.Release(7, token)
	if _, err := s.Acquire(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	var count int64
	s.db.Table("companion_profiles").Count(&count)
	if count != 1 {
		t.Fatalf("created duplicate profiles: %d", count)
	}
}

func TestCompanionForgetCannotBeUndoneByStaleExtraction(t *testing.T) {
	s := testStore(t)
	p, _ := s.Ensure(t.Context(), 1)
	_, _ = s.Ensure(t.Context(), 2)
	now := time.Now()
	for _, m := range []Memory{
		{ID: "mine", UserID: 1, Key: "interest", Value: "旧事实", ExpiresAt: now.Add(time.Hour)},
		{ID: "other", UserID: 2, Key: "interest", Value: "另一位用户", ExpiresAt: now.Add(time.Hour)},
	} {
		if err := s.db.Table("companion_memories").Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	s.db.Table("companion_profiles").Where("user_id = ?", 1).Updates(map[string]interface{}{"summary": "旧摘要", "refresh_token": "old"})
	if err := s.Forget(t.Context(), 1, 100, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user forget accepted: %v", err)
	}
	if err := s.Forget(t.Context(), 1, 100, "mine"); err != nil {
		t.Fatal(err)
	}
	service := &Service{Store: s}
	err := service.saveExtraction(t.Context(), p, "old", 99, extractedMemory{Summary: "旧摘要回来", Memories: []extractedFact{
		{Key: "interest", Value: "旧事实", Evidence: "我喜欢旧事实", SourceMessageID: 90, Days: 30},
	}}, map[uint]extractionSource{90: {Text: "我喜欢旧事实"}})
	if err != nil {
		t.Fatal(err)
	}
	profile, _ := s.Profile(t.Context(), 1)
	if profile.Summary != "" || profile.ForgetThroughID != 100 || profile.ThroughID != 100 || profile.Revision != 1 {
		t.Fatalf("forget boundary lost: %+v", profile)
	}
	mine, _ := s.Memories(t.Context(), 1)
	other, _ := s.Memories(t.Context(), 2)
	if len(mine) != 0 || len(other) != 1 {
		t.Fatalf("memory isolation lost: mine=%v other=%v", mine, other)
	}
}

func TestCompanionExtractionRequiresUserEvidenceAndBoundsMemory(t *testing.T) {
	s := testStore(t)
	p, _ := s.Ensure(t.Context(), 1)
	now := time.Now()
	for i := 0; i < 60; i++ {
		m := Memory{ID: fmt.Sprint(i), UserID: 1, Key: fmt.Sprint(i), Value: "fact", ExpiresAt: now.Add(time.Hour), UpdatedAt: now.Add(-time.Hour)}
		if err := s.db.Table("companion_memories").Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	facts := []extractedFact{
		{Key: "invalid", Value: "幻觉", Evidence: "不存在的原话", SourceMessageID: 99},
		{Key: "assistant", Value: "助手的猜测", Evidence: "助手的猜测", SourceMessageID: 100},
		{Key: "music", Value: "喜欢爵士音乐", Evidence: "我喜欢爵士音乐", SourceMessageID: 10, Days: 90},
		{Key: "music", Value: "重复", Evidence: "我喜欢爵士音乐", SourceMessageID: 10},
	}
	s.db.Table("companion_profiles").Where("user_id = 1").Update("refresh_token", "new")
	if err := (&Service{Store: s}).saveExtraction(t.Context(), p, "new", 11, extractedMemory{Summary: strings.Repeat("长", 3000), Memories: facts}, map[uint]extractionSource{10: {Text: "最近发现我喜欢爵士音乐"}}); err != nil {
		t.Fatal(err)
	}
	memories, _ := s.Memories(t.Context(), 1)
	if len(memories) != 60 || memories[0].Key != "music" {
		t.Fatalf("bad memory cap or extraction: %v", memories)
	}
	profile, _ := s.Profile(t.Context(), 1)
	if len([]rune(profile.Summary)) != 1800 {
		t.Fatal("summary is not bounded")
	}
}

func TestCompanionCorrectionClearsOldContextAndPreservesMemoryID(t *testing.T) {
	s := testStore(t)
	_, _ = s.Ensure(t.Context(), 1)
	s.db.Table("companion_memories").Create(&Memory{ID: "mine", UserID: 1, Key: "name", Value: "旧名字", ExpiresAt: time.Now().Add(time.Hour)})
	if err := s.Edit(t.Context(), 2, 100, "mine", "别人的修改"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := s.Edit(t.Context(), 1, 100, "mine", "正确名字"); err != nil {
		t.Fatal(err)
	}
	items, _ := s.Memories(t.Context(), 1)
	p, _ := s.Profile(t.Context(), 1)
	if len(items) != 1 || items[0].ID != "mine" || items[0].Value != "正确名字" || items[0].Evidence != "由你修改" || p.ForgetThroughID != 100 {
		t.Fatalf("correction failed: %+v %+v", items, p)
	}
}

func TestCompanionGreetingDoesNotNag(t *testing.T) {
	now := time.Now()
	p := &Profile{}
	if !CanGreet(p, 0, now) {
		t.Fatal("missing first welcome")
	}
	p.GreetingAt, p.GreetingAfterUserID = now.Add(-48*time.Hour), 10
	if CanGreet(p, 10, now) {
		t.Fatal("unanswered greeting repeated")
	}
	if !CanGreet(p, 11, now) {
		t.Fatal("engaged user cannot get a later greeting")
	}
	p.GreetingAt = now.Add(-time.Minute)
	if CanGreet(p, 12, now) {
		t.Fatal("quick reentry greeted again")
	}
	p.GreetingAt = now.Add(-48 * time.Hour)
	p.Quiet = true
	if CanGreet(p, 12, now) {
		t.Fatal("quiet user greeted")
	}
	p.Quiet = false
	p.LeaseUntil = now.Add(time.Hour).Unix()
	if CanGreet(p, 12, now) {
		t.Fatal("active reply interrupted")
	}
}

func TestCompanionPromptExpiresAndRecallsRelevantFacts(t *testing.T) {
	now := time.Now()
	p := &Profile{Summary: "已过期的旧摘要", SummaryAt: now.Add(-31 * 24 * time.Hour)}
	items := []Memory{{ID: "expired", Key: "expired", Value: "过期事实不能注入", ExpiresAt: now.Add(-time.Second)}}
	for i := 0; i < 60; i++ {
		items = append(items, Memory{ID: fmt.Sprint(i), Key: fmt.Sprint(i), Value: strings.Repeat("日常", 100), UpdatedAt: now.Add(time.Duration(i) * time.Minute), ExpiresAt: now.Add(time.Hour)})
	}
	items = append(items, Memory{ID: "old-interest", Key: "音乐", Value: "喜欢爵士音乐", UpdatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)})
	prompt := BuildPrompt(p, items, now, "你有什么爵士音乐推荐")
	if strings.Contains(prompt, "已过期的旧摘要") || strings.Contains(prompt, "过期事实不能注入") {
		t.Fatal("expired context was injected")
	}
	if !strings.Contains(prompt, "喜欢爵士音乐") || len([]rune(prompt)) > 7000 {
		t.Fatal("bounded relevant recall failed")
	}
}

func TestCompanionDeletedAccountCleanupOnlyTouchesOwnTables(t *testing.T) {
	s := testStore(t)
	if err := s.db.Exec("CREATE TABLE identity_users (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	s.db.Exec("INSERT INTO identity_users (id) VALUES (2)")
	_, _ = s.Ensure(t.Context(), 1)
	_, _ = s.Ensure(t.Context(), 2)
	s.db.Table("companion_memories").Create(&Memory{ID: "orphan", UserID: 1, Key: "old", Value: "removed", ExpiresAt: time.Now().Add(time.Hour)})
	if err := s.PurgeDeletedUsers(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Profile(t.Context(), 1); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted account data survived")
	}
	if _, err := s.Profile(t.Context(), 2); err != nil {
		t.Fatal("live account removed")
	}
	var users, memories int64
	s.db.Table("identity_users").Count(&users)
	s.db.Table("companion_memories").Count(&memories)
	if users != 1 || memories != 0 {
		t.Fatalf("cleanup affected wrong data: users=%d memories=%d", users, memories)
	}
}
