package wechat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	appwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/wechat"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

type fakeRepository struct{}

func (fakeRepository) IssueRegistrationCode(_ context.Context, _ string, _ string) (string, error) {
	return "ABCD-EFGH-IJKL-MNPQ", nil
}

var _ repository.WeChatRegistrationRepository = fakeRepository{}

func TestVerifySignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/wechat/callback", NewHandler(appwechat.NewService(fakeRepository{}), "token").Verify)
	values := url.Values{"timestamp": {"1"}, "nonce": {"2"}, "echostr": {"ok"}}
	parts := []string{"token", "1", "2"}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	values.Set("signature", hex.EncodeToString(digest[:]))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/wechat/callback?"+values.Encode(), nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestModuleRegistersPublicCallbackPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	NewModule(NewHandler(appwechat.NewService(fakeRepository{}), "token")).RegisterPublicRoutes(api)

	values := url.Values{"timestamp": {"1"}, "nonce": {"2"}, "echostr": {"ok"}}
	parts := []string{"token", "1", "2"}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	values.Set("signature", hex.EncodeToString(digest[:]))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/wechat/callback?"+values.Encode(), nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}
