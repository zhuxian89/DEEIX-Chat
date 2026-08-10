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

type Module struct {
	Handler      *Handler
	AdminHandler *AdminHandler
}

func NewModule(handler *Handler, adminHandlers ...*AdminHandler) *Module {
	module := &Module{Handler: handler}
	if len(adminHandlers) > 0 {
		module.AdminHandler = adminHandlers[0]
	}
	return module
}

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

func (m *Module) RegisterAdminRoutes(adminGroup *gin.RouterGroup) {
	if m.AdminHandler == nil {
		return
	}
	adminGroup.GET("/wechat/actions", m.AdminHandler.Actions)
	adminGroup.GET("/wechat/summary", m.AdminHandler.Summary)
	adminGroup.GET("/wechat/rules", m.AdminHandler.ListRules)
	adminGroup.POST("/wechat/rules", m.AdminHandler.CreateRule)
	adminGroup.PATCH("/wechat/rules/:id", m.AdminHandler.UpdateRule)
	adminGroup.PATCH("/wechat/rules/:id/enabled", m.AdminHandler.SetRuleEnabled)
	adminGroup.GET("/wechat/templates", m.AdminHandler.ListTemplates)
	adminGroup.POST("/wechat/templates", m.AdminHandler.CreateTemplate)
	adminGroup.PATCH("/wechat/templates/:id", m.AdminHandler.UpdateTemplate)
	adminGroup.PATCH("/wechat/templates/:id/enabled", m.AdminHandler.SetTemplateEnabled)
	adminGroup.GET("/wechat/issuances", m.AdminHandler.ListIssuances)
	adminGroup.GET("/wechat/logs", m.AdminHandler.ListLogs)
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
	if message.MsgType != "text" {
		c.String(http.StatusOK, "success")
		return
	}
	result, err := h.service.HandleTextMessage(c.Request.Context(), message.FromUserName, message.Content)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "handle wechat keyword failed")
		return
	}
	if !result.Matched {
		c.String(http.StatusOK, "success")
		return
	}
	reply, err := xml.Marshal(textReply{
		ToUserName: message.FromUserName, FromUserName: message.ToUserName,
		CreateTime: time.Now().Unix(), MsgType: "text",
		Content: result.Content,
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
