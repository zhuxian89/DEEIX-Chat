package companion

import (
	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
)

type CompanionContextPreferences struct {
	Enabled  bool     `json:"enabled"`
	Excluded []string `json:"excluded" binding:"max=500,dive,max=64"`
}
type CompanionContextSource struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	Text           string    `json:"text"`
	ConversationID string    `json:"conversationID"`
	At             time.Time `json:"at"`
}
type CompanionContextConversation struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Excluded bool   `json:"excluded"`
}
type CompanionFollowup struct {
	ID                   string    `json:"id"`
	Title                string    `json:"title"`
	DueAt                time.Time `json:"dueAt"`
	Status               string    `json:"status"`
	SourceKind           string    `json:"sourceKind"`
	SourceID             string    `json:"sourceID"`
	SourceConversationID string    `json:"sourceConversationID"`
	Evidence             string    `json:"evidence"`
	Revision             uint      `json:"revision"`
	UserManaged          bool      `json:"userManaged"`
	DeliveredAt          time.Time `json:"deliveredAt"`
	NotificationStatus   string    `json:"notificationStatus"`
}
type CompanionDecision struct {
	Kind   string    `json:"kind"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}
type CompanionNotifications struct {
	Ready      bool   `json:"ready"`
	Enabled    bool   `json:"enabled"`
	Credits    int    `json:"credits"`
	TemplateID string `json:"templateID"`
}
type CompanionAssistantState struct {
	Preferences   CompanionContextPreferences `json:"preferences"`
	Sources       []CompanionContextSource    `json:"sources"`
	Followups     []CompanionFollowup         `json:"followups"`
	Decisions     []CompanionDecision         `json:"decisions"`
	Notifications CompanionNotifications      `json:"notifications"`
}
type CompanionAssistantResponse struct {
	ErrorMsg string                   `json:"errorMsg"`
	Data     *CompanionAssistantState `json:"data"`
}
type CompanionConversationsResponse struct {
	ErrorMsg string                         `json:"errorMsg"`
	Data     []CompanionContextConversation `json:"data"`
}
type CompanionFollowupResponse struct {
	ErrorMsg string            `json:"errorMsg"`
	Data     CompanionFollowup `json:"data"`
}
type CompanionFollowupRequest struct {
	ID       string    `json:"id" binding:"max=64"`
	Title    string    `json:"title" binding:"required,max=120"`
	DueAt    time.Time `json:"dueAt" binding:"required"`
	Status   string    `json:"status" binding:"required,oneof=pending completed canceled"`
	Revision uint      `json:"revision"`
}
type CompanionNotificationRequest struct {
	Enabled    bool   `json:"enabled"`
	Accepted   bool   `json:"accepted"`
	TemplateID string `json:"templateID" binding:"max=128"`
	RequestID  string `json:"requestID,omitempty" binding:"max=100"`
	Automatic  bool   `json:"automatic,omitempty"`
}

func followupDTO(f domain.Followup) CompanionFollowup {
	return CompanionFollowup{ID: f.ID, Title: f.Title, DueAt: f.DueAt, Status: f.Status, SourceKind: f.SourceKind, SourceID: f.SourceID, SourceConversationID: f.SourceConversationID, Evidence: f.Evidence, Revision: f.Revision, UserManaged: f.UserManaged, DeliveredAt: f.DeliveredAt, NotificationStatus: f.NotificationStatus}
}
func assistantDTO(s *app.AssistantState) *CompanionAssistantState {
	result := &CompanionAssistantState{Preferences: CompanionContextPreferences{Enabled: s.Preferences.Enabled, Excluded: s.Preferences.Excluded}, Sources: []CompanionContextSource{}, Followups: []CompanionFollowup{}, Decisions: []CompanionDecision{}, Notifications: CompanionNotifications{Ready: s.NotificationReady, Enabled: s.Notifications.Enabled, Credits: s.Notifications.Credits, TemplateID: s.Notifications.TemplateID}}
	for _, v := range s.Sources {
		result.Sources = append(result.Sources, CompanionContextSource{ID: v.ID, Kind: v.Kind, Title: v.Title, Text: v.Text, ConversationID: v.ConversationID, At: v.At})
	}
	for _, v := range s.Followups {
		result.Followups = append(result.Followups, followupDTO(v))
	}
	for _, v := range s.Decisions {
		result.Decisions = append(result.Decisions, CompanionDecision{Kind: v.Kind, Reason: v.Reason, At: v.At})
	}
	return result
}

// Assistant godoc
// @Summary 查看助手全局来源、跟进和通知状态
// @Tags companion
// @Produce json
// @Security BearerAuth
// @Success 200 {object} CompanionAssistantResponse
// @Router /companion/assistant [get]
func (h *Handler) Assistant(c *gin.Context) {
	s, err := h.service.AssistantState(c.Request.Context(), middleware.MustUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, assistantDTO(s))
}

// ContextPreferences godoc
// @Summary 设置同账号会话和待办的读取范围
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CompanionContextPreferences true "全局读取与排除会话"
// @Success 200 {object} response.SuccessDoc
// @Router /companion/assistant/context [patch]
func (h *Handler) ContextPreferences(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req CompanionContextPreferences
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if err := h.service.SetContextPreferences(c.Request.Context(), middleware.MustUserID(c), req.Enabled, req.Excluded); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}

// ContextConversations godoc
// @Summary 分页选择允许助手读取的会话
// @Tags companion
// @Produce json
// @Security BearerAuth
// @Param q query string false "标题关键词"
// @Param page query int false "页码，从1开始，每页50条"
// @Success 200 {object} CompanionConversationsResponse
// @Router /companion/assistant/conversations [get]
func (h *Handler) ContextConversations(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		fail(c, app.ErrInvalid)
		return
	}
	items, err := h.service.ContextConversations(c.Request.Context(), middleware.MustUserID(c), c.Query("q"), page)
	if err != nil {
		fail(c, err)
		return
	}
	result := make([]CompanionContextConversation, 0, len(items))
	for _, v := range items {
		result = append(result, CompanionContextConversation{ID: v.ID, Title: v.Title, Excluded: v.Excluded})
	}
	response.Success(c, result)
}

// SaveFollowup godoc
// @Summary 新增、改期、完成或取消跟进事项
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CompanionFollowupRequest true "新增时ID为空；更新需要当前revision"
// @Success 200 {object} CompanionFollowupResponse
// @Router /companion/assistant/followups [post]
func (h *Handler) SaveFollowup(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	var req CompanionFollowupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.SaveFollowup(c.Request.Context(), middleware.MustUserID(c), req.ID, req.Title, req.Status, req.DueAt, req.Revision)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, followupDTO(item))
}

// Notifications godoc
// @Summary 记录微信订阅选择或关闭离线通知
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CompanionNotificationRequest true "用户在微信弹窗中的真实选择"
// @Success 200 {object} response.SuccessDoc
// @Router /companion/assistant/notifications [post]
func (h *Handler) Notifications(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<10)
	var req CompanionNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if err := h.service.SetNotifications(c.Request.Context(), middleware.MustUserID(c), req.Enabled, req.Accepted, req.TemplateID, domain.NotificationConsent{RequestID: req.RequestID, Automatic: req.Automatic}); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}
