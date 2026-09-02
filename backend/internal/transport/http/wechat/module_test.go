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
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

type fakeRepository struct{}

func testRuntime(token string) *config.Runtime {
	return config.NewRuntime(config.Config{WeChatCallbackToken: token})
}

func (fakeRepository) IssueRegistrationCode(_ context.Context, _ string, _ string) (string, error) {
	return "ABCD-EFGH-IJKL-MNPQ", nil
}

var _ repository.WeChatRegistrationRepository = fakeRepository{}

type fakeNotifier struct {
	messageCalls int
	replyCalls   int
	followCalls  int
	messageType  string
	content      string
}

func (f *fakeNotifier) NotifyWeChatMessage(_ string, messageType string, content string) {
	f.messageCalls++
	f.messageType = messageType
	f.content = content
}

func (f *fakeNotifier) NotifyWeChatReply(_ string, content string) {
	f.replyCalls++
	f.content = content
}

func (f *fakeNotifier) NotifyWeChatFollow(_ string) {
	f.followCalls++
}

func TestVerifySignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runtime := config.NewRuntime(config.Config{WeChatCallbackToken: "token"})
	router := gin.New()
	router.GET("/wechat/callback", NewHandler(appwechat.NewService(fakeRepository{}), runtime, nil).Verify)
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

func TestVerifySignatureUsesUpdatedRuntimeToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	runtime := config.NewRuntime(config.Config{WeChatCallbackToken: "old-token"})
	router := gin.New()
	router.GET("/wechat/callback", NewHandler(appwechat.NewService(fakeRepository{}), runtime, nil).Verify)

	runtime.Store(config.Config{WeChatCallbackToken: "new-token"})
	oldRecorder := httptest.NewRecorder()
	router.ServeHTTP(oldRecorder, signedRequestWithToken(http.MethodGet, "/wechat/callback", "", "old-token"))
	if oldRecorder.Code != http.StatusForbidden {
		t.Fatalf("old token status = %d", oldRecorder.Code)
	}
	newRecorder := httptest.NewRecorder()
	router.ServeHTTP(newRecorder, signedRequestWithToken(http.MethodGet, "/wechat/callback", "", "new-token"))
	if newRecorder.Code != http.StatusOK {
		t.Fatalf("new token status = %d body=%q", newRecorder.Code, newRecorder.Body.String())
	}
}

func TestModuleRegistersPublicCallbackPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	NewModule(NewHandler(appwechat.NewService(fakeRepository{}), testRuntime("token"), nil)).RegisterPublicRoutes(api)

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

func TestReceiveKeepsRegistrationTextKeyword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/wechat/callback", NewHandler(appwechat.NewService(fakeRepository{}), testRuntime("token"), nil).Receive)

	body := `<xml><ToUserName>official-account</ToUserName><FromUserName>openid-text</FromUserName><CreateTime>1</CreateTime><MsgType>text</MsgType><Content>13004</Content></xml>`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedRequest(http.MethodPost, "/wechat/callback", body))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "ABCD-EFGH-IJKL-MNPQ") {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestReceiveNotifiesWechatMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	notifier := &fakeNotifier{}
	router := gin.New()
	router.POST("/wechat/callback", NewHandler(appwechat.NewService(fakeRepository{}), testRuntime("token"), notifier).Receive)

	body := `<xml><ToUserName>official-account</ToUserName><FromUserName>openid-text</FromUserName><CreateTime>1</CreateTime><MsgType>text</MsgType><Content>hello</Content></xml>`
	recorder := httptest.NewRecorder()
	request := signedRequest(http.MethodPost, "/wechat/callback", body)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if notifier.messageCalls != 1 || notifier.messageType != "text" || notifier.content != "hello" {
		t.Fatalf("expected inbound message notification, got %#v", notifier)
	}
}

func TestReceiveNotifiesSubscribeEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	notifier := &fakeNotifier{}
	router := gin.New()
	router.POST("/wechat/callback", NewHandler(appwechat.NewService(fakeRepository{}), testRuntime("token"), notifier).Receive)

	body := `<xml><ToUserName>official-account</ToUserName><FromUserName>openid-follow</FromUserName><CreateTime>1</CreateTime><MsgType>event</MsgType><Event>subscribe</Event></xml>`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedRequest(http.MethodPost, "/wechat/callback", body))

	if recorder.Code != http.StatusOK || notifier.followCalls != 1 || notifier.messageCalls != 0 {
		t.Fatalf("expected one follow notification, status=%d notifier=%#v", recorder.Code, notifier)
	}
}

func TestReceiveSkipsOtherWechatEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	notifier := &fakeNotifier{}
	router := gin.New()
	router.POST("/wechat/callback", NewHandler(appwechat.NewService(fakeRepository{}), testRuntime("token"), notifier).Receive)

	body := `<xml><ToUserName>official-account</ToUserName><FromUserName>openid-unfollow</FromUserName><CreateTime>1</CreateTime><MsgType>event</MsgType><Event>unsubscribe</Event></xml>`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedRequest(http.MethodPost, "/wechat/callback", body))

	if recorder.Code != http.StatusOK || notifier.followCalls != 0 || notifier.messageCalls != 0 {
		t.Fatalf("expected event notification to be skipped, status=%d notifier=%#v", recorder.Code, notifier)
	}
}

func signedRequest(method, path, body string) *http.Request {
	return signedRequestWithToken(method, path, body, "token")
}

func signedRequestWithToken(method, path, body, token string) *http.Request {
	values := url.Values{"timestamp": {"1"}, "nonce": {"2"}}
	parts := []string{token, values.Get("timestamp"), values.Get("nonce")}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	values.Set("signature", hex.EncodeToString(digest[:]))
	return httptest.NewRequest(method, path+"?"+values.Encode(), strings.NewReader(body))
}
