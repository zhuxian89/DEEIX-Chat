package companion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/lifecycle"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	service         *app.Service
	cfg             *config.Runtime
	shutdown        *lifecycle.Shutdown
	logger          *zap.Logger
	refreshSlots    chan struct{}
	topicSlots      chan struct{}
	initiativeSlots chan struct{}
}

func NewHandler(service *app.Service, cfg *config.Runtime, shutdown *lifecycle.Shutdown, logger *zap.Logger) *Handler {
	return &Handler{service: service, cfg: cfg, shutdown: shutdown, logger: logger, refreshSlots: make(chan struct{}, 2), topicSlots: make(chan struct{}, 1), initiativeSlots: make(chan struct{}, 2)}
}

func (h *Handler) Register(group *gin.RouterGroup) {
	g := group.Group("/companion")
	g.GET("", h.State)
	g.POST("/open", h.Open)
	g.POST("/read", h.Read)
	g.PATCH("/preferences", h.Preferences)
	g.DELETE("/memories", h.Forget)
	g.DELETE("/memories/:id", h.Forget)
	g.PATCH("/memories/:id", h.EditMemory)
	g.POST("/topics/feedback", h.TopicFeedback)
	g.POST("/initiatives", h.PrepareInitiative)
	g.POST("/initiatives/:id/accept", h.AcceptInitiative)
	g.POST("/conversations/:id/messages/stream", h.Stream)
}

type StateResponse struct {
	ErrorMsg string `json:"errorMsg"`
	Data     *State `json:"data"`
}
type OpenRequest struct {
	AllowGreeting bool `json:"allowGreeting"`
}
type PreferencesRequest struct {
	Quiet       *bool   `json:"quiet"`
	Proactivity *string `json:"proactivity" binding:"omitempty,oneof=normal less off"`
}

// CompanionChatRequest preserves the upstream wire contract under a unique
// Swagger name; cross-package references otherwise collide in swag's registry.
type CompanionChatRequest conversation.SendMessageRequest

type EditMemoryRequest struct {
	Value string `json:"value" binding:"required,max=120"`
}
type ReadRequest struct {
	MessageID uint `json:"messageID" binding:"required"`
}

type TopicFeedbackRequest struct {
	TopicURL   string `json:"topicURL" binding:"required,max=1500"`
	Preference string `json:"preference" binding:"required,oneof=like avoid"`
}

// TopicFeedback godoc
// @Summary 调整主动话题偏好，保存为可删除的助手记忆
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body TopicFeedbackRequest true "已展示话题的反馈"
// @Success 200 {object} response.SuccessDoc
// @Router /companion/topics/feedback [post]
func (h *Handler) TopicFeedback(c *gin.Context) {
	var req TopicFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if err := h.service.TopicFeedback(c.Request.Context(), middleware.MustUserID(c), req.TopicURL, req.Preference); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}

// Read godoc
// @Summary 记录前台已阅读的助手消息，抑制未读时的主动开场
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ReadRequest true "已读消息"
// @Success 200 {object} response.SuccessDoc
// @Router /companion/read [post]
func (h *Handler) Read(c *gin.Context) {
	var req ReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if err := h.service.MarkRead(c.Request.Context(), middleware.MustUserID(c), req.MessageID); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}

func fail(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, app.ErrNotFound) {
		status = http.StatusNotFound
	}
	if errors.Is(err, app.ErrBusy) {
		status = http.StatusConflict
	}
	if errors.Is(err, app.ErrInvalid) {
		status = http.StatusBadRequest
	}
	response.Error(c, status, err.Error())
}

// State godoc
// @Summary 获取固定助手及专属记忆
// @Tags companion
// @Produce json
// @Security BearerAuth
// @Success 200 {object} StateResponse
// @Failure 404 {object} response.SuccessDoc
// @Router /companion [get]
func (h *Handler) State(c *gin.Context) {
	state, err := h.service.State(c.Request.Context(), middleware.MustUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, stateDTO(state))
}

// Open godoc
// @Summary 打开助手，仅在前台空闲时允许一次适度开场
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body OpenRequest true "前台状态"
// @Success 200 {object} StateResponse
// @Router /companion/open [post]
func (h *Handler) Open(c *gin.Context) {
	var req OpenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	model := strings.TrimSpace(h.cfg.Snapshot().WeChatMiniAppDefaultChatModel)
	if model == "" {
		model = h.service.Chat.GetConversationSystemDefaultModel()
	}
	state, err := h.service.Open(c.Request.Context(), middleware.MustUserID(c), model, req.AllowGreeting)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, stateDTO(state))
	h.refresh(middleware.MustUserID(c))
	h.refreshTopics(middleware.MustUserID(c))
}

// Preferences godoc
// @Summary 调整助手主动程度，兼容旧版开场开关
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body PreferencesRequest true "开场偏好"
// @Success 200 {object} response.SuccessDoc
// @Router /companion/preferences [patch]
func (h *Handler) Preferences(c *gin.Context) {
	var req PreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if (req.Quiet == nil) == (req.Proactivity == nil) {
		fail(c, app.ErrInvalid)
		return
	}
	var err error
	if req.Proactivity != nil {
		err = h.service.SetProactivity(c.Request.Context(), middleware.MustUserID(c), *req.Proactivity)
	} else {
		err = h.service.SetQuiet(c.Request.Context(), middleware.MustUserID(c), *req.Quiet)
	}
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}

// Forget godoc
// @Summary 删除助手记忆，并阻止旧上下文再次提取
// @Tags companion
// @Produce json
// @Security BearerAuth
// @Param id path string false "记忆 ID；集合接口删除全部"
// @Success 200 {object} response.SuccessDoc
// @Router /companion/memories [delete]
// @Router /companion/memories/{id} [delete]
func (h *Handler) Forget(c *gin.Context) {
	if err := h.service.Forget(c.Request.Context(), middleware.MustUserID(c), c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}

// EditMemory godoc
// @Summary 纠正助手的一条记忆
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "记忆 ID"
// @Param body body EditMemoryRequest true "纠正后的事实"
// @Success 200 {object} response.SuccessDoc
// @Router /companion/memories/{id} [patch]
func (h *Handler) EditMemory(c *gin.Context) {
	var req EditMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if err := h.service.EditMemory(c.Request.Context(), middleware.MustUserID(c), c.Param("id"), req.Value); err != nil {
		fail(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}

// Stream godoc
// @Summary 与固定助手聊天，沿用聊天事件、任务恢复和计费契约
// @Tags companion
// @Accept json
// @Produce application/x-ndjson
// @Security BearerAuth
// @Param id path string true "助手会话 public_id"
// @Param body body CompanionChatRequest true "标准聊天请求"
// @Success 200 {string} string "NDJSON stream"
// @Router /companion/conversations/{id}/messages/stream [post]
func (h *Handler) Stream(c *gin.Context) {
	userID := middleware.MustUserID(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		fail(c, app.ErrInvalid)
		return
	}
	var req conversation.SendMessageRequest
	if json.Unmarshal(body, &req) != nil || len([]rune(req.Content)) > 10_000 || len(req.FileIDs) > 4 {
		fail(c, app.ErrInvalid)
		return
	}
	token, err := h.service.Store.Acquire(c.Request.Context(), userID)
	if err != nil {
		fail(c, err)
		return
	}
	defer h.service.Store.Release(userID, token)
	service, profile, err := h.service.Conversation(c.Request.Context(), userID, c.Param("id"), req.Content)
	if err != nil {
		fail(c, err)
		return
	}
	// The server chooses Exa tools; caller-selected tools/skills remain unavailable.
	req.Model = profile.Model
	req.SelectedToolIDs, req.SkillIDs, req.KnowledgeBaseIDs = nil, nil, nil
	if toolIDs, toolErr := service.CompanionWebToolIDs(c.Request.Context()); toolErr == nil {
		req.SelectedToolIDs = toolIDs
	}
	req.BranchReason, req.ParentMessagePublicID, req.SourceMessagePublicID = "default", "", ""
	req.HTMLVisualPromptEnabled = false
	body, err = json.Marshal(req)
	if err != nil {
		fail(c, err)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	leaseCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				return
			case <-ticker.C:
				ctx, stop := context.WithTimeout(leaseCtx, 5*time.Second)
				err := h.service.Store.Renew(ctx, userID, token)
				stop()
				if err != nil {
					if req.ClientRunID != "" {
						service.CancelMessageGeneration(context.Background(), userID, req.ClientRunID)
					}
					return
				}
			}
		}
	}()
	conversation.NewHandler(service, h.cfg, h.shutdown).StreamMessage(c)
	h.refresh(userID)
	h.refreshTopics(userID)
}

func (h *Handler) refreshTopics(userID uint) {
	if h.service.TopicProvider == nil {
		return
	}
	select {
	case h.topicSlots <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-h.topicSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), 22*time.Second)
		defer cancel()
		if err := h.service.RefreshTopics(ctx, userID); err != nil && h.logger != nil {
			h.logger.Debug("companion_topics_unavailable")
		}
	}()
}

func (h *Handler) refresh(userID uint) {
	select {
	case h.refreshSlots <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-h.refreshSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if err := h.service.Refresh(ctx, userID); err != nil && h.logger != nil {
			h.logger.Warn("companion_memory_refresh_failed", zap.Uint("user_id", userID), zap.Error(err))
		}
	}()
}
