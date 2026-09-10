package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSpeechRemainsAuthenticatedWhenCompanionIsDisabled(t *testing.T) {
	t.Setenv("DEEIX_COMPANION_ENABLED", "false")
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "speech.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	engine := gin.New()
	if err := registerCompanion(engine, db, config.NewRuntime(config.Config{JWTSecret: "test"}), nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path string }{{"GET", "/api/v1/speech/capabilities"}, {"POST", "/api/v1/speech/transcriptions"}} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatal("unprotected speech endpoint", response.Code)
		}
	}
	tables, err := db.Migrator().GetTables()
	if err != nil || len(tables) != 5 {
		t.Fatal("only the five independent TODO tables should be registered", tables, err)
	}
	for _, table := range tables {
		if !strings.HasPrefix(table, "miniapp_") {
			t.Fatalf("speech must not create or alter tables: %s", table)
		}
	}
}
