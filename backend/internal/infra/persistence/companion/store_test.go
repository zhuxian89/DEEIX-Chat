package companion

import (
	"path/filepath"
	"testing"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCompanionStoreReusesExistingRowsAndUniqueMemoryKeys(t *testing.T) {
	db := testDB(t)
	// Existing rows use the deployed table and column names, independently of
	// the new private Go record names. AutoMigrate may add companion-owned fields.
	for _, statement := range []string{
		`CREATE TABLE companion_profiles (user_id integer PRIMARY KEY, conversation_id integer, conversation_public_id text, model text, quiet numeric, summary text, through_id integer, revision integer)`,
		`INSERT INTO companion_profiles VALUES (7, 17, 'existing-chat', 'existing-model', 1, 'existing-summary', 23, 4)`,
		`CREATE TABLE companion_memories (id text PRIMARY KEY, user_id integer, key text, value text, evidence text, source_message_id integer, expires_at datetime, updated_at datetime)`,
		`CREATE UNIQUE INDEX companion_memory_owner_key ON companion_memories(user_id, key)`,
		`INSERT INTO companion_memories VALUES ('existing-memory', 7, 'music', 'jazz', 'I like jazz', 21, '2099-01-01 00:00:00', '2026-09-08 00:00:00')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		store, err := NewStore(db)
		if err != nil {
			t.Fatal(err)
		}
		p, err := store.Profile(t.Context(), 7)
		if err != nil || p.ConversationID != 17 || p.ConversationPublicID != "existing-chat" || p.Model != "existing-model" || !p.Quiet || p.Summary != "existing-summary" || p.ThroughID != 23 || p.Revision != 4 {
			t.Fatalf("profile changed during migration: %+v %v", p, err)
		}
		memories, err := store.Memories(t.Context(), 7)
		if err != nil || len(memories) != 1 || memories[0].ID != "existing-memory" || memories[0].SourceMessageID != 21 || memories[0].Value != "jazz" {
			t.Fatalf("memory changed during migration: %+v %v", memories, err)
		}
	}
	duplicate := memoryRecord{ID: "duplicate", UserID: 7, Key: "music", Value: "duplicate"}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("existing unique owner/key constraint was lost")
	}
	var tables []string
	if err := db.Raw("SELECT name FROM sqlite_master WHERE type = 'table' AND name IN ('profiles', 'memories', 'profile_records', 'memory_records')").Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	if len(tables) != 0 {
		t.Fatalf("new Go type names created shadow tables: %v", tables)
	}
}

func TestCompanionExtractionRollsBackSummaryWhenMemoryWriteFails(t *testing.T) {
	db := testDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Ensure(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	claimed, err := store.ClaimRefresh(t.Context(), 7, 0, "refresh", now)
	if err != nil || !claimed {
		t.Fatalf("refresh claim failed: %v", err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_memory BEFORE INSERT ON companion_memories BEGIN SELECT RAISE(ABORT, 'simulated memory write failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	err = store.SaveExtraction(t.Context(), domain.Extraction{UserID: 7, Token: "refresh", ThroughID: 20, Summary: "new summary", At: now,
		Memories: []domain.Memory{{ID: "memory", UserID: 7, Key: "music", Value: "jazz", ExpiresAt: now.Add(time.Hour)}}})
	if err == nil {
		t.Fatal("expected memory write failure")
	}
	p, err := store.Profile(t.Context(), 7)
	if err != nil || p.Summary != "" || p.ThroughID != 0 || p.RefreshToken != "refresh" {
		t.Fatalf("summary advanced without its memories: %+v %v", p, err)
	}
}

func testDB(t *testing.T) *gorm.DB {
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
	return db
}
