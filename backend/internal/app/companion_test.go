package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/lifecycle"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCompanionRegistrationIsAdditiveAndAuthenticated(t *testing.T) {
	t.Setenv("DEEIX_COMPANION_ENABLED", "true")
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "registration.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	shutdown := lifecycle.NewShutdown()
	defer shutdown.BeginDrain()
	engine := gin.New()
	if err := registerCompanion(engine, db, config.NewRuntime(config.Config{JWTSecret: "test-companion-secret"}), nil, nil, nil, shutdown, nil); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("companion_profiles") || !db.Migrator().HasTable("companion_memories") || !db.Migrator().HasTable("companion_topic_caches") {
		t.Fatal("missing companion tables")
	}
	if db.Migrator().HasTable("identity_users") || db.Migrator().HasTable("chat_messages") {
		t.Fatal("companion migrated upstream tables")
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/companion", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unprotected companion endpoint: %d", response.Code)
	}
}

func TestCompanionCanBeDisabledWhileTodoStaysAvailable(t *testing.T) {
	t.Setenv("DEEIX_COMPANION_ENABLED", "false")
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "todo-registration.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	engine := gin.New()
	if err := registerCompanion(engine, db, config.NewRuntime(config.Config{JWTSecret: "test"}), nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/companion", nil))
	if response.Code != http.StatusNotFound {
		t.Fatal("disabled companion registered routes")
	}
	if db.Migrator().HasTable("companion_profiles") || db.Migrator().HasTable("identity_users") {
		t.Fatal("disabled companion or upstream tables migrated")
	}
	if !db.Migrator().HasTable("miniapp_entry_unlocks") || !db.Migrator().HasTable("miniapp_todo_tasks") {
		t.Fatal("TODO must migrate independently")
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/miniapp-entry/status", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("TODO route must remain authenticated: %d", response.Code)
	}
}
