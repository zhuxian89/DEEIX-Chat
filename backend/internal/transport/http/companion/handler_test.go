package companion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	companionstore "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCompanionRoutesIsolateStateAndBoundRequestBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "http.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	store, err := companionstore.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Ensure(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Ensure(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	db.Table("companion_memories").Create(&app.Memory{ID: "private", UserID: 2, Key: "name", Value: "another user secret", ExpiresAt: time.Now().Add(time.Hour)})
	h := NewHandler(&app.Service{Store: store}, config.NewRuntime(config.Config{}), nil, nil)
	engine := gin.New()
	group := engine.Group("/api/v1", func(c *gin.Context) { c.Set(middleware.ContextKeyUserID, uint(1)) })
	h.Register(group)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/v1/companion", "", 200},
		{"PATCH", "/api/v1/companion/preferences", `{"quiet":true}`, 200},
		{"PATCH", "/api/v1/companion/preferences", `{}`, 400},
		{"PATCH", "/api/v1/companion/preferences", `{"proactivity":"invalid"}`, 400},
		{"PATCH", "/api/v1/companion/preferences", `{"quiet":true,"proactivity":"normal"}`, 400},
		{"PATCH", "/api/v1/companion/preferences", `{"proactivity":"less"}`, 200},
		{"PATCH", "/api/v1/companion/preferences", `{"proactivity":"off"}`, 200},
		{"POST", "/api/v1/companion/initiatives", `{"kind":"other","visitID":"visit-1234"}`, 400},
		{"POST", "/api/v1/companion/initiatives", strings.Repeat("x", 2049), 400},
		{"POST", "/api/v1/companion/initiatives/foreign/accept", `{"visitID":"visit-1234"}`, 404},
		{"POST", "/api/v1/companion/topics/feedback", `{"topicURL":"https://www.bbc.com/news/foreign","preference":"like"}`, 404},
		{"POST", "/api/v1/companion/topics/feedback", `{"topicURL":"https://www.bbc.com/news/foreign","preference":"invalid"}`, 400},
		{"POST", "/api/v1/companion/conversations/foreign/messages/stream", `{"content":"hello"}`, 404},
		{"POST", "/api/v1/companion/conversations/foreign/messages/stream", strings.Repeat("x", 65537), 400},
	} {
		t.Run(tc.method+tc.path+strconv.Itoa(tc.status), func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			writer := httptest.NewRecorder()
			engine.ServeHTTP(writer, request)
			if writer.Code != tc.status {
				t.Fatalf("got %d: %s", writer.Code, writer.Body.String())
			}
			if strings.Contains(writer.Body.String(), "another user secret") {
				t.Fatal("cross-user memory exposed")
			}
			if writer.Header().Get("Content-Type") == "application/x-ndjson" {
				t.Fatal("invalid request entered generation")
			}
		})
	}
	p, _ := store.Profile(t.Context(), 1)
	if !p.Quiet || p.LeaseUntil != 0 {
		t.Fatalf("preferences/lease not saved correctly: %+v", p)
	}
}

func TestCompanionStreamUsesCurrentMessageFileLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "file-limit.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	store, err := companionstore.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Ensure(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	runtimeCfg := config.NewRuntime(config.Config{MaxMessageFiles: 9})
	h := NewHandler(&app.Service{Store: store}, runtimeCfg, nil, nil)
	engine := gin.New()
	group := engine.Group("/api/v1", func(c *gin.Context) { c.Set(middleware.ContextKeyUserID, uint(1)) })
	h.Register(group)
	for _, tc := range []struct {
		name       string
		configured int
		files      int
		status     int
	}{
		{"nine photos accepted", 9, 9, http.StatusNotFound},
		{"configured boundary enforced", 9, 10, http.StatusBadRequest},
		{"runtime decrease applied", 2, 3, http.StatusBadRequest},
		{"runtime increase applied", 10, 9, http.StatusNotFound},
		{"zero uses default", 0, 10, http.StatusNotFound},
		{"zero default boundary enforced", 0, 11, http.StatusBadRequest},
		{"negative uses default", -1, 10, http.StatusNotFound},
		{"negative default boundary enforced", -1, 11, http.StatusBadRequest},
		{"http boundary accepted", 20, 20, http.StatusNotFound},
		{"http boundary enforced", 100, 21, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtimeCfg.Store(config.Config{MaxMessageFiles: tc.configured})
			fileIDs := make([]string, tc.files)
			for i := range fileIDs {
				fileIDs[i] = "photo-" + strconv.Itoa(i)
			}
			body, err := json.Marshal(map[string]any{
				"contentType": "mixed",
				"content":     "Describe these photos together.",
				"fileIDs":     fileIDs,
			})
			if err != nil {
				t.Fatal(err)
			}
			// A permitted count reaches the conversation ownership check and must
			// still reject this foreign conversation before generation begins.
			request := httptest.NewRequest(http.MethodPost, "/api/v1/companion/conversations/foreign/messages/stream", strings.NewReader(string(body)))
			request.Header.Set("Content-Type", "application/json")
			writer := httptest.NewRecorder()
			engine.ServeHTTP(writer, request)
			if writer.Code != tc.status {
				t.Fatalf("got %d, want %d: %s", writer.Code, tc.status, writer.Body.String())
			}
			profile, err := store.Profile(t.Context(), 1)
			if err != nil || profile.LeaseUntil != 0 {
				t.Fatalf("request left a conversation lease: %+v, %v", profile, err)
			}
		})
	}
}
