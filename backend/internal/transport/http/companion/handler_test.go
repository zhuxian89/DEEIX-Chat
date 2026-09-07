package companion

import (
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
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
	store, err := app.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Ensure(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Ensure(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
	db.Create(&app.Memory{ID: "private", UserID: 2, Key: "name", Value: "another user secret", ExpiresAt: time.Now().Add(time.Hour)})
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
