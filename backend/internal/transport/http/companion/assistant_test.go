package companion

import (
	"encoding/json"
	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	storepkg "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssistantHTTPScopesEditsAndNeverEnablesUnconfiguredNotifications(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "assistant-http.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	store, err := storepkg.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.Ensure(t.Context(), 1)
	_, _ = store.Ensure(t.Context(), 2)
	foreign, err := store.SaveFollowup(t.Context(), domain.Followup{UserID: 2, Title: "private-title", DueAt: time.Now().Add(time.Hour), Status: "pending", SourceKind: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	own, err := store.SaveFollowup(t.Context(), domain.Followup{UserID: 1, Title: "自己的事项", DueAt: time.Now().Add(time.Hour), Status: "pending", SourceKind: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(&app.Service{Store: store}, config.NewRuntime(config.Config{}), nil, nil)
	engine := gin.New()
	h.Register(engine.Group("/api/v1", func(c *gin.Context) { c.Set(middleware.ContextKeyUserID, uint(1)) }))
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, r)
		return w
	}
	response := request("GET", "/api/v1/companion/assistant", "")
	if response.Code != 200 || strings.Contains(response.Body.String(), "private-title") {
		t.Fatal("assistant source isolation failed", response.Body.String())
	}
	var state CompanionAssistantResponse
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil || state.Data.Notifications.Ready {
		t.Fatal("unconfigured notification shown as ready")
	}
	response = request("POST", "/api/v1/companion/assistant/notifications", `{"enabled":true,"accepted":true,"templateID":"invented"}`)
	if response.Code != 400 {
		t.Fatal("unconfigured consent accepted")
	}
	for _, tc := range []struct {
		item   domain.Followup
		rev    uint
		status int
	}{{foreign, foreign.Revision, 404}, {own, 0, 409}, {own, own.Revision, 200}} {
		raw, _ := json.Marshal(CompanionFollowupRequest{ID: tc.item.ID, Title: tc.item.Title, DueAt: tc.item.DueAt, Status: "canceled", Revision: tc.rev})
		response = request("POST", "/api/v1/companion/assistant/followups", string(raw))
		if response.Code != tc.status {
			t.Fatalf("edit status %d wanted %d: %s", response.Code, tc.status, response.Body.String())
		}
	}
	response = request("POST", "/api/v1/companion/assistant/followups", strings.Repeat("x", 9000))
	if response.Code != 400 {
		t.Fatal("unbounded body accepted")
	}
}
