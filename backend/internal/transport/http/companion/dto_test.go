package companion

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
	"github.com/gin-gonic/gin"
)

func TestCompanionStateWireContractPreservesFieldsAndHidesInternalData(t *testing.T) {
	at := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	state := &app.State{Name: "小伴", Model: "configured-model", ConversationPublicID: "conversation",
		Greeting: "你好", GreetingID: "greeting", GreetingAt: at, GreetingOffered: true,
		Memories: []app.Memory{{ID: "memory", UserID: 42, Key: "music", Value: "喜欢爵士", Evidence: "我喜欢爵士",
			SourceMessageID: 123, SourceAt: at, ExpiresAt: at, UpdatedAt: at}},
		Topic: &app.Topic{Category: "music", Title: "报道", URL: "https://www.bbc.com/news/music",
			Opener: "聊聊音乐", PublishedAt: at, FetchedAt: at}}
	encoded, err := json.Marshal(stateDTO(state))
	if err != nil {
		t.Fatal(err)
	}
	var actual, expected map[string]interface{}
	if err := json.Unmarshal(encoded, &actual); err != nil {
		t.Fatal(err)
	}
	// This fixture describes the existing public API, including nested field names.
	if err := json.Unmarshal([]byte(`{"name":"小伴","model":"configured-model","conversationPublicID":"conversation","quiet":false,"greeting":"你好","greetingID":"greeting","greetingAt":"2026-09-08T00:00:00Z","greetingOffered":true,"memories":[{"id":"memory","key":"music","value":"喜欢爵士","evidence":"我喜欢爵士","expiresAt":"2026-09-08T00:00:00Z","updatedAt":"2026-09-08T00:00:00Z"}],"topic":{"category":"music","title":"报道","url":"https://www.bbc.com/news/music","opener":"聊聊音乐","publishedAt":"2026-09-08T00:00:00Z","fetchedAt":"2026-09-08T00:00:00Z"}}`), &expected); err != nil {
		t.Fatal(err)
	}
	expected["proactivity"], expected["initiativeVersion"], expected["initiatives"] = "", float64(0), []interface{}{}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("public state contract changed or exposed internal fields: %s", encoded)
	}
	empty, err := json.Marshal(stateDTO(&app.State{}))
	if err != nil {
		t.Fatal(err)
	}
	var emptyState map[string]json.RawMessage
	if err := json.Unmarshal(empty, &emptyState); err != nil {
		t.Fatal(err)
	}
	if string(emptyState["memories"]) != "[]" || emptyState["topic"] != nil {
		t.Fatalf("empty state no longer matches the client contract: %s", empty)
	}
}

func TestCompanionErrorsKeepStatusAndEnglishFallbacks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		err     error
		status  int
		message string
	}{
		{app.ErrBusy, http.StatusConflict, "resource conflict"},
		{app.ErrNotFound, http.StatusNotFound, "resource not found"},
		{app.ErrInvalid, http.StatusBadRequest, "invalid request"},
		{errors.New("private database detail"), http.StatusInternalServerError, "internal server error"},
	} {
		t.Run(tc.message, func(t *testing.T) {
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			fail(ctx, tc.err)
			var envelope struct {
				ErrorMsg string `json:"errorMsg"`
			}
			if err := json.Unmarshal(writer.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if writer.Code != tc.status || envelope.ErrorMsg != tc.message {
				t.Fatalf("unexpected error envelope: %d %s", writer.Code, writer.Body.String())
			}
		})
	}
}
