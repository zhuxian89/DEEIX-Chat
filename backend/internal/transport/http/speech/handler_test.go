package speech

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/speech"
	domainspeech "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/speech"
	speechport "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/speech"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type settingsStub struct{}

func (settingsStub) RuntimeValuesByNamespace(context.Context, string) (map[string]string, error) {
	return map[string]string{"enabled": "true", "tencent_secret_id": "private-id", "tencent_secret_key": "private-key"}, nil
}

type recognizerStub struct{ calls int }

func (r *recognizerStub) Recognize(context.Context, domainspeech.Config, []byte) (speechport.Transcript, error) {
	r.calls++
	return speechport.Transcript{Text: "recognized draft", DurationMS: 1000}, nil
}

func TestSpeechHTTPRejectsInvalidBodiesAndReturnsOnlyText(t *testing.T) {
	recognizer := &recognizerStub{}
	engine := gin.New()
	group := engine.Group("/api/v1", func(c *gin.Context) { c.Set(middleware.ContextKeyUserID, uint(1)) })
	NewHandler(app.NewService(settingsStub{}, recognizer, nil)).Register(group)
	for _, body := range []string{
		`{"audio":"@@@"}`, `{"audio":"bXAz","url":"http://private.local"}`, `{"audio":"bXAz"} {}`,
		`{"audio":"` + strings.Repeat("A", 800*1024) + `"}`,
		`{"audio":"` + base64.StdEncoding.EncodeToString(make([]byte, speechport.MaxAudioBytes+1)) + `"}`,
	} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/speech/transcriptions", strings.NewReader(body)))
		if recorder.Code != 400 && recorder.Code != 413 {
			t.Fatal("invalid body accepted", recorder.Code)
		}
	}
	if recognizer.calls != 0 {
		t.Fatal("invalid input reached provider")
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/speech/transcriptions", strings.NewReader(`{"audio":"bXAz"}`)))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "recognized draft") || recognizer.calls != 1 {
		t.Fatal("recognition failed")
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/speech/capabilities", nil))
	if response.Code != 200 || strings.Contains(response.Body.String(), "private") {
		t.Fatal("capability response exposed configuration")
	}
	if !strings.Contains(response.Body.String(), fmt.Sprintf(`"maxDurationMs":%d`, 60000)) {
		t.Fatal("missing recording limit")
	}
}
