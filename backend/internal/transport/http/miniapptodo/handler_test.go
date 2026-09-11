package miniapptodo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/miniapptodo"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type handlerStore struct {
	domain.Repository
	grant    *time.Time
	last     domain.Operation
	feedback string
}

func (s *handlerStore) ResolveOwner(_ context.Context, user uint, appID string) (domain.Owner, error) {
	if user != 7 {
		return domain.Owner{}, domain.ErrIdentity
	}
	return domain.Owner{AppID: appID, OpenID: "server-openid"}, nil
}
func (s *handlerStore) UnlockedAt(context.Context, domain.Owner) (*time.Time, error) {
	return s.grant, nil
}
func (s *handlerStore) Unlock(_ context.Context, _ domain.Owner, at time.Time) (time.Time, error) {
	s.grant = &at
	return at, nil
}
func (s *handlerStore) Snapshot(context.Context, domain.Owner) (domain.Snapshot, error) {
	return domain.Snapshot{Lists: []domain.List{{ID: "inbox", Name: "收件箱", Version: 1}}, Tasks: []domain.Task{}}, nil
}
func (s *handlerStore) Apply(_ context.Context, _ domain.Owner, op domain.Operation, _ time.Time) (domain.OperationResult, error) {
	s.last = op
	return domain.OperationResult{ID: op.ID, Status: "applied"}, nil
}
func (s *handlerStore) Feedback(_ context.Context, _ domain.Owner, content string, _ time.Time) error {
	s.feedback = content
	return nil
}
func TestHTTPEntrySyncAndFeedbackContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &handlerStore{}
	engine := gin.New()
	group := engine.Group("/api/v1", func(c *gin.Context) { c.Set(middleware.ContextKeyUserID, uint(7)) })
	NewHandler(app.NewService(store, app.Config{AppID: "app"})).RegisterRoutes(group)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		return recorder
	}
	bad := call(http.MethodPost, "/miniapp-entry/unlock", `{"code":"wrong","openid":"attacker"}`)
	if bad.Code != 400 || store.grant != nil {
		t.Fatal(bad.Code, bad.Body.String())
	}
	if !strings.Contains(bad.Body.String(), `"errorCode":"miniapp_todo.invalid_code"`) {
		t.Fatal("missing stable code for invalid experience code:", bad.Body.String())
	}
	good := call(http.MethodPost, "/miniapp-entry/unlock", `{"code":"666"}`)
	if good.Code != 200 || strings.Contains(good.Body.String(), "server-openid") {
		t.Fatal(good.Code, good.Body.String())
	}
	var envelope struct{ Data EntryStatusResponse }
	if err := json.Unmarshal(good.Body.Bytes(), &envelope); err != nil || !envelope.Data.Unlocked || envelope.Data.OwnerKey == "" {
		t.Fatal(envelope, err)
	}
	result := call(http.MethodPost, "/miniapp-todo/sync", `{"operations":[{"id":"a","entityID":"b","kind":"task.save","baseVersion":3,"task":{"id":"b","listID":"inbox","title":"标题","repeatKind":"none"}}]}`)
	if result.Code != 200 || store.last.BaseVersion != 3 || store.last.Task.Title != "标题" {
		t.Fatal(result.Code, result.Body.String(), store.last)
	}
	if !strings.Contains(result.Body.String(), `"tasks":[]`) {
		t.Fatal(result.Body.String())
	}
	feedback := call(http.MethodPost, "/miniapp-todo/feedback", `{"content":"建议"}`)
	if feedback.Code != 200 || store.feedback != "建议" {
		t.Fatal(feedback.Body.String())
	}
	malformed := call(http.MethodPost, "/miniapp-todo/sync", `{"operations":[]}`)
	if malformed.Code != 400 {
		t.Fatal(malformed.Code)
	}
}

func TestHTTPFailuresUseTODOErrorCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		err     error
		status  int
		code    string
		message string
	}{
		{domain.ErrIdentity, http.StatusForbidden, "miniapp_todo.identity_required", "verified miniapp identity required"},
		{domain.ErrInvalid, http.StatusBadRequest, "miniapp_todo.invalid_request", "invalid todo request"},
		{domain.ErrInvalidCode, http.StatusBadRequest, "miniapp_todo.invalid_code", "invalid todo experience code"},
		{domain.ErrExportLimit, http.StatusBadRequest, "miniapp_todo.export_limit_exceeded", "TODO export limit exceeded; narrow the selected range"},
		{domain.ErrConflict, http.StatusConflict, "miniapp_todo.version_conflict", "TODO version conflict"},
		{domain.ErrSnapshotLimit, http.StatusConflict, "miniapp_todo.snapshot_limit_exceeded", "TODO task capacity exceeded; complete or delete tasks and retry"},
		{errors.New("private database connection detail"), http.StatusInternalServerError, "internal.error", "internal server error"},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			fail(ctx, tc.err)
			var envelope response.Envelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != tc.status || envelope.ErrorCode != tc.code || envelope.ErrorMsg != tc.message {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
