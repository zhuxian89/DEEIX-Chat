package wechat

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	appwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/wechat"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/gin-gonic/gin"
)

type Module struct{ Handler *Handler }

func NewModule(handler *Handler) *Module { return &Module{Handler: handler} }

type Handler struct {
	service *appwechat.Service
	token   string
}

func NewHandler(service *appwechat.Service, token string) *Handler {
	return &Handler{service: service, token: strings.TrimSpace(token)}
}

func (m *Module) RegisterPublicRoutes(api *gin.RouterGroup) {
	api.GET("/wechat/callback", m.Handler.Verify)
	api.POST("/wechat/callback", m.Handler.Receive)
}

type inboundMessage struct {
	ToUserName   string `xml:"ToUserName"`
	FromUserName string `xml:"FromUserName"`
	CreateTime   int64  `xml:"CreateTime"`
	MsgType      string `xml:"MsgType"`
	Content      string `xml:"Content"`
	MsgID        int64  `xml:"MsgId"`
}

type textReply struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
}

func (h *Handler) Verify(c *gin.Context) {
	if !h.validSignature(c) {
		c.String(http.StatusForbidden, "invalid signature")
		return
	}
	c.String(http.StatusOK, c.Query("echostr"))
}

func (h *Handler) Receive(c *gin.Context) {
	if !h.validSignature(c) {
		c.String(http.StatusForbidden, "invalid signature")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid wechat message")
		return
	}
	var message inboundMessage
	if err := xml.Unmarshal(body, &message); err != nil || strings.TrimSpace(message.FromUserName) == "" {
		response.Error(c, http.StatusBadRequest, "invalid wechat message")
		return
	}
	if message.MsgType != "text" || !appwechat.IsRegistrationKeyword(message.Content) {
		c.String(http.StatusOK, "success")
		return
	}
	code, err := h.service.IssueRegistrationCode(c.Request.Context(), message.FromUserName)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "issue registration code failed")
		return
	}
	reply, err := xml.Marshal(textReply{
		ToUserName: message.FromUserName, FromUserName: message.ToUserName,
		CreateTime: time.Now().Unix(), MsgType: "text",
		Content: "您的专属注册码：" + code,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "build wechat reply failed")
		return
	}
	c.Data(http.StatusOK, "application/xml; charset=utf-8", reply)
}

func (h *Handler) validSignature(c *gin.Context) bool {
	if h.token == "" {
		return false
	}
	parts := []string{h.token, c.Query("timestamp"), c.Query("nonce")}
	if parts[1] == "" || parts[2] == "" || c.Query("signature") == "" {
		return false
	}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	expected := []byte(hex.EncodeToString(digest[:]))
	provided := []byte(strings.ToLower(strings.TrimSpace(c.Query("signature"))))
	return len(expected) == len(provided) && subtle.ConstantTimeCompare(expected, provided) == 1
}
