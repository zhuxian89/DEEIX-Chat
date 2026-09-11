package miniapptodo

import (
	"errors"
	"net/http"
	"strconv"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/miniapptodo"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service *app.Service }

func NewHandler(service *app.Service) *Handler { return &Handler{service: service} }
func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/miniapp-entry/status", h.Status)
	group.GET("/miniapp-todo/snapshot", h.Snapshot)
	group.POST("/miniapp-todo/sync", h.Sync)
	group.GET("/miniapp-todo/tasks", h.Tasks)
	group.GET("/miniapp-todo/export", h.Export)
	group.POST("/miniapp-todo/feedback", h.Feedback)
}
func fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrIdentity):
		response.ErrorWithCode(c, http.StatusForbidden, "miniapp_todo.identity_required", "")
	case errors.Is(err, domain.ErrInvalidCode):
		response.ErrorWithCode(c, http.StatusBadRequest, "miniapp_todo.invalid_code", "")
	case errors.Is(err, domain.ErrExportLimit):
		response.ErrorWithCode(c, http.StatusBadRequest, "miniapp_todo.export_limit_exceeded", "")
	case errors.Is(err, domain.ErrInvalid):
		response.ErrorWithCode(c, http.StatusBadRequest, "miniapp_todo.invalid_request", "")
	case errors.Is(err, domain.ErrConflict):
		response.ErrorWithCode(c, http.StatusConflict, "miniapp_todo.version_conflict", "")
	case errors.Is(err, domain.ErrSnapshotLimit):
		response.ErrorWithCode(c, http.StatusConflict, "miniapp_todo.snapshot_limit_exceeded", "")
	default:
		response.Error(c, http.StatusInternalServerError, "internal server error")
	}
}
func bind(c *gin.Context, value any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024*1024)
	if err := c.ShouldBindJSON(value); err != nil {
		response.InvalidRequestBody(c, err)
		return false
	}
	return true
}

// Status godoc
// @Summary 查询当前微信身份的默认入口
// @Tags miniapp-todo
// @Produce json
// @Security BearerAuth
// @Success 200 {object} EntryStatusResponse
// @Router /miniapp-entry/status [get]
func (h *Handler) Status(c *gin.Context) {
	value, err := h.service.Status(c.Request.Context(), middleware.MustUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, EntryStatusResponse(value))
}

// Snapshot godoc
// @Summary 获取当前用户的待办与近期记录
// @Tags miniapp-todo
// @Produce json
// @Security BearerAuth
// @Success 200 {object} SnapshotResponse
// @Router /miniapp-todo/snapshot [get]
func (h *Handler) Snapshot(c *gin.Context) {
	value, err := h.service.Snapshot(c.Request.Context(), middleware.MustUserID(c))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, snapshotResponse(value))
}

// Sync godoc
// @Summary 按操作标识和版本同步待办
// @Tags miniapp-todo
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body SyncRequest true "待同步操作"
// @Success 200 {object} SyncResponse
// @Router /miniapp-todo/sync [post]
func (h *Handler) Sync(c *gin.Context) {
	var req SyncRequest
	if !bind(c, &req) {
		return
	}
	ops := make([]domain.Operation, 0, len(req.Operations))
	for _, op := range req.Operations {
		ops = append(ops, op.domain())
	}
	value, err := h.service.Sync(c.Request.Context(), middleware.MustUserID(c), ops)
	if err != nil {
		fail(c, err)
		return
	}
	result := SyncResponse{Snapshot: snapshotResponse(value.Snapshot), Results: []OperationResultResponse{}}
	for _, receipt := range value.Results {
		result.Results = append(result.Results, OperationResultResponse(receipt))
	}
	response.Success(c, result)
}
func query(c *gin.Context) (domain.Query, error) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		return domain.Query{}, domain.ErrInvalid
	}
	size, err := strconv.Atoi(c.DefaultQuery("pageSize", "50"))
	if err != nil {
		return domain.Query{}, domain.ErrInvalid
	}
	return domain.Query{Q: c.Query("q"), Completed: c.Query("completed"), ListID: c.Query("listID"), Page: page, PageSize: size}, nil
}

// Tasks godoc
// @Summary 查询与分页读取待办历史
// @Tags miniapp-todo
// @Produce json
// @Security BearerAuth
// @Param q query string false "标题或备注"
// @Param completed query string false "完成状态 true 或 false"
// @Param listID query string false "清单 ID"
// @Param page query int false "页码"
// @Param pageSize query int false "每页数量，最多 100"
// @Success 200 {object} TaskPageResponse
// @Router /miniapp-todo/tasks [get]
func (h *Handler) Tasks(c *gin.Context) {
	q, err := query(c)
	if err != nil {
		fail(c, err)
		return
	}
	value, err := h.service.Query(c.Request.Context(), middleware.MustUserID(c), q)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, TaskPageResponse{Results: tasksResponse(value.Results), Total: value.Total})
}

// Export godoc
// @Summary 导出当前用户的待办
// @Tags miniapp-todo
// @Produce json
// @Security BearerAuth
// @Param format query string true "text 或 csv"
// @Param listID query string false "清单 ID"
// @Param completed query string false "完成状态 true 或 false"
// @Success 200 {object} ExportResponse
// @Router /miniapp-todo/export [get]
func (h *Handler) Export(c *gin.Context) {
	q, err := query(c)
	if err != nil {
		fail(c, err)
		return
	}
	value, err := h.service.Export(c.Request.Context(), middleware.MustUserID(c), q, c.Query("format"))
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, ExportResponse(value))
}

// Feedback godoc
// @Summary 提交待办反馈并返回当前入口状态
// @Tags miniapp-todo
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body FeedbackRequest true "普通反馈正文"
// @Success 200 {object} EntryStatusResponse
// @Router /miniapp-todo/feedback [post]
func (h *Handler) Feedback(c *gin.Context) {
	var req FeedbackRequest
	if !bind(c, &req) {
		return
	}
	value, err := h.service.Feedback(c.Request.Context(), middleware.MustUserID(c), req.Content)
	if err != nil {
		fail(c, err)
		return
	}
	response.Success(c, EntryStatusResponse(value))
}
