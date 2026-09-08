package companion

import (
	"encoding/json"
	"net/http"
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type Initiative struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Text           string    `json:"text"`
	AfterMessageID uint      `json:"afterMessageID"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
	AcceptedAt     time.Time `json:"acceptedAt"`
	Topic          *Topic    `json:"topic,omitempty"`
}

type InitiativeRequest struct {
	Kind           string `json:"kind" binding:"required,oneof=home idle"`
	VisitID        string `json:"visitID" binding:"required,min=8,max=64"`
	AfterMessageID uint   `json:"afterMessageID"`
}

type AcceptInitiativeRequest struct {
	VisitID string `json:"visitID" binding:"required,min=8,max=64"`
}

type InitiativeResponse struct {
	ErrorMsg string           `json:"errorMsg"`
	Data     InitiativeResult `json:"data"`
}

type InitiativeResult struct {
	Message *Initiative `json:"message" extensions:"x-nullable,!x-omitempty"`
}

func initiativeDTO(item *app.Initiative) *Initiative {
	if item == nil {
		return nil
	}
	result := &Initiative{ID: item.ID, Kind: item.Kind, Text: item.Text, AfterMessageID: item.AfterMessageID,
		CreatedAt: item.CreatedAt, ExpiresAt: item.ExpiresAt, AcceptedAt: item.AcceptedAt}
	if item.TopicJSON != "" {
		var topic Topic
		if json.Unmarshal([]byte(item.TopicJSON), &topic) == nil {
			result.Topic = &topic
		}
	}
	return result
}

// PrepareInitiative godoc
// @Summary 为前台空闲准备一条候选主动消息，不立即展示、不扣用户余额
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body InitiativeRequest true "触发场景与页面停留标识"
// @Success 200 {object} InitiativeResponse
// @Router /companion/initiatives [post]
func (h *Handler) PrepareInitiative(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req InitiativeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	select {
	case h.initiativeSlots <- struct{}{}:
		defer func() { <-h.initiativeSlots }()
	default:
		response.Success(c, InitiativeResult{})
		return
	}
	item, err := h.service.PrepareInitiative(c.Request.Context(), middleware.MustUserID(c), req.Kind, req.VisitID, req.AfterMessageID)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, InitiativeResult{Message: initiativeDTO(item)})
}

// AcceptInitiative godoc
// @Summary 前台仍空闲时确认展示候选，拒绝过期或会话已变化的消息
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "候选消息 ID"
// @Param body body AcceptInitiativeRequest true "原页面停留标识"
// @Success 200 {object} InitiativeResponse
// @Router /companion/initiatives/{id}/accept [post]
func (h *Handler) AcceptInitiative(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req AcceptInitiativeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.AcceptInitiative(c.Request.Context(), middleware.MustUserID(c), c.Param("id"), req.VisitID)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, InitiativeResult{Message: initiativeDTO(item)})
}
