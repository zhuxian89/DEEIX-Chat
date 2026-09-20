package wechat

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/wechat"
	domainwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechat"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

type imageCallbackRepo struct {
	repository.WeChatAdminRepository
	lookups int
	logs    int
}

func (r *imageCallbackRepo) GetKeywordRule(_ context.Context, keyword string) (*domainwechat.KeywordRule, error) {
	r.lookups++
	if keyword != "小程序" {
		return nil, repository.ErrNotFound
	}
	return &domainwechat.KeywordRule{Keyword: "小程序", Action: domainwechat.ActionReplyImage, TemplateID: 3,
		Enabled: true, TemplateEnabled: true, TemplateType: "image", TemplateName: "小程序码", TemplateContent: "permanent-image_123"}, nil
}

func (r *imageCallbackRepo) CreateInvocationLog(_ context.Context, _ *domainwechat.InvocationLog) error {
	r.logs++
	return nil
}

func TestReceiveRepliesWithImageXML(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &imageCallbackRepo{}
	notifier := &fakeNotifier{}
	router := gin.New()
	router.POST("/wechat/callback", NewHandler(appwechat.NewService(repo), testRuntime("token"), notifier).Receive)
	body := `<xml><ToUserName>official-account</ToUserName><FromUserName>openid</FromUserName><MsgType>text</MsgType><Content>小程序</Content></xml>`
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, signedRequest(http.MethodPost, "/wechat/callback", body))
	var reply struct {
		XMLName      xml.Name
		ToUserName   string
		FromUserName string
		CreateTime   int64
		MsgType      string
		Image        struct {
			MediaID string `xml:"MediaId"`
		}
	}
	if recorder.Code != http.StatusOK || xml.Unmarshal(recorder.Body.Bytes(), &reply) != nil {
		t.Fatalf("invalid response: %d %s", recorder.Code, recorder.Body.String())
	}
	if reply.XMLName.Local != "xml" || reply.ToUserName != "openid" || reply.FromUserName != "official-account" || reply.MsgType != "image" || reply.CreateTime == 0 || reply.Image.MediaID != "permanent-image_123" {
		t.Fatalf("unexpected image XML: %+v", reply)
	}
	if !strings.HasPrefix(recorder.Header().Get("Content-Type"), "application/xml") || strings.Contains(recorder.Body.String(), "<Content>") {
		t.Fatalf("image was returned as text: %s", recorder.Body.String())
	}
	if repo.logs != 1 || notifier.replyCalls != 1 || notifier.content != "[图片] 小程序码" {
		t.Fatalf("image reply audit missing: repo=%+v notifier=%+v", repo, notifier)
	}
	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, signedRequestWithToken(http.MethodPost, "/wechat/callback", body, "wrong-token"))
	if invalid.Code != http.StatusForbidden || repo.lookups != 1 {
		t.Fatal("invalid signature reached keyword service")
	}
}
