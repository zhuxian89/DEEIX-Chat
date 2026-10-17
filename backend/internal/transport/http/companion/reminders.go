package companion

import (
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

type ReminderSchedule struct {
	Kind            string    `json:"kind"`
	StartAt         time.Time `json:"startAt"`
	Until           time.Time `json:"until"`
	Times           []int     `json:"times"`
	Weekdays        []int     `json:"weekdays"`
	IntervalMinutes int       `json:"intervalMinutes"`
	Day             int       `json:"day"`
	Month           int       `json:"month"`
	Limit           int       `json:"limit"`
}
type ReminderPlan struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Status      string           `json:"status"`
	Schedule    ReminderSchedule `json:"schedule"`
	Description string           `json:"description"`
	NextAt      time.Time        `json:"nextAt"`
	Revision    uint             `json:"revision"`
}
type ReminderEvent struct {
	ID                 string    `json:"id"`
	ReminderID         string    `json:"reminderID"`
	Title              string    `json:"title"`
	Status             string    `json:"status"`
	DueAt              time.Time `json:"dueAt"`
	NotifiedAt         time.Time `json:"notifiedAt"`
	NotificationStatus string    `json:"notificationStatus"`
	Revision           uint      `json:"revision"`
}
type ReminderDialogue struct {
	ID          string    `json:"id"`
	Input       string    `json:"input"`
	Reply       string    `json:"reply"`
	ReminderIDs []string  `json:"reminderIDs"`
	CreatedAt   time.Time `json:"createdAt"`
}
type ReminderState struct {
	Plans       []ReminderPlan     `json:"plans"`
	Occurrences []ReminderEvent    `json:"occurrences"`
	Turns       []ReminderDialogue `json:"turns"`
	ServerTime  time.Time          `json:"serverTime"`
}
type ReminderCommandRequest struct {
	RequestID      string           `json:"requestID" binding:"required,min=8,max=100"`
	ConversationID string           `json:"conversationID" binding:"max=64"`
	Input          string           `json:"input" binding:"max=8000"`
	Action         string           `json:"action" binding:"omitempty,oneof=create list update pause resume delete complete skip snooze"`
	ID             string           `json:"id" binding:"max=64"`
	OccurrenceID   string           `json:"occurrenceID" binding:"max=64"`
	Title          string           `json:"title" binding:"max=120"`
	Revision       uint             `json:"revision"`
	DelayMinutes   int              `json:"delayMinutes"`
	Schedule       ReminderSchedule `json:"schedule"`
}
type ReminderCommandResult struct {
	Handled  bool             `json:"handled"`
	Turn     ReminderDialogue `json:"turn"`
	Snapshot ReminderState    `json:"snapshot"`
}
type ReminderStateResponse struct {
	ErrorMsg string        `json:"errorMsg"`
	Data     ReminderState `json:"data"`
}
type ReminderCommandResponse struct {
	ErrorMsg string                `json:"errorMsg"`
	Data     ReminderCommandResult `json:"data"`
}

func reminderDialogueDTO(t domain.ReminderTurn) ReminderDialogue {
	return ReminderDialogue{ID: t.ID, Input: t.Input, Reply: t.Reply, ReminderIDs: t.ReminderIDs, CreatedAt: t.CreatedAt}
}
func reminderStateDTO(s domain.ReminderSnapshot) ReminderState {
	r := ReminderState{Plans: []ReminderPlan{}, Occurrences: []ReminderEvent{}, Turns: []ReminderDialogue{}, ServerTime: s.ServerTime}
	for _, p := range s.Plans {
		v := p.Schedule
		r.Plans = append(r.Plans, ReminderPlan{ID: p.ID, Title: p.Title, Status: p.Status, Schedule: ReminderSchedule{Kind: v.Kind, StartAt: v.StartAt, Until: v.Until, Times: v.Times, Weekdays: v.Weekdays, IntervalMinutes: v.IntervalMinutes, Day: v.Day, Month: v.Month, Limit: v.Limit}, Description: v.Description(), NextAt: p.NextAt, Revision: p.Revision})
	}
	for _, e := range s.Occurrences {
		r.Occurrences = append(r.Occurrences, ReminderEvent{ID: e.ID, ReminderID: e.ReminderID, Title: e.Title, Status: e.Status, DueAt: e.DueAt, NotifiedAt: e.NotifiedAt, NotificationStatus: e.NotificationStatus, Revision: e.Revision})
	}
	for _, t := range s.Turns {
		r.Turns = append(r.Turns, reminderDialogueDTO(t))
	}
	return r
}

// Reminders godoc
// @Summary 查询提醒、到期记录及提醒对话
// @Tags companion
// @Produce json
// @Security BearerAuth
// @Param conversationID query string false "当前小禾会话"
// @Success 200 {object} ReminderStateResponse
// @Router /companion/reminders [get]
func (h *Handler) Reminders(c *gin.Context) {
	s, err := h.service.Reminders(c.Request.Context(), middleware.MustUserID(c), c.Query("conversationID"))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, reminderStateDTO(s))
}

// ReminderCommand godoc
// @Summary 通过对话或卡片创建、查询和管理提醒
// @Tags companion
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ReminderCommandRequest true "requestID保证重试幂等；卡片操作须携带revision"
// @Success 200 {object} ReminderCommandResponse
// @Router /companion/reminders/command [post]
func (h *Handler) ReminderCommand(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 40<<10)
	var req ReminderCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	// Text commands and structured actions are separate contracts.
	if req.Action != "" && req.Input != "" {
		fail(c, domain.ErrInvalid)
		return
	}
	v := req.Schedule
	r, err := h.service.ReminderCommand(c.Request.Context(), middleware.MustUserID(c), domain.ReminderCommand{RequestID: req.RequestID, ConversationID: req.ConversationID, Input: req.Input, Action: req.Action, ID: req.ID, OccurrenceID: req.OccurrenceID, Title: req.Title, Revision: req.Revision, DelayMinutes: req.DelayMinutes, Schedule: domain.ReminderSchedule{Kind: v.Kind, StartAt: v.StartAt, Until: v.Until, Times: v.Times, Weekdays: v.Weekdays, IntervalMinutes: v.IntervalMinutes, Day: v.Day, Month: v.Month, Limit: v.Limit}})
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, ReminderCommandResult{Handled: r.Handled, Turn: reminderDialogueDTO(r.Turn), Snapshot: reminderStateDTO(r.Snapshot)})
}
