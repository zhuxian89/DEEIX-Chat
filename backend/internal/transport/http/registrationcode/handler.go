package registrationcode

import (
	"net/http"
	"strconv"

	appregistrationcode "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/registrationcode"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service *appregistrationcode.Service }

func NewHandler(service *appregistrationcode.Service) *Handler { return &Handler{service: service} }

// List godoc
// @Summary 管理员查询注册码
// @Tags admin-registration-codes
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Param status query string false "状态：active/used"
// @Success 200 {object} ListResponseDoc
// @Failure 500 {object} ErrorDoc
// @Router /admin/registration-codes [get]
func (h *Handler) List(c *gin.Context) {
	page, pageSize := 1, 20
	if value, err := strconv.Atoi(c.Query("page")); err == nil && value > 0 {
		page = value
	}
	if value, err := strconv.Atoi(c.Query("page_size")); err == nil && value > 0 {
		pageSize = value
	}
	items, total, err := h.service.List(c.Request.Context(), page, pageSize, c.Query("status"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list registration codes failed")
		return
	}
	views := make([]CodeResponse, 0, len(items))
	for _, item := range items {
		views = append(views, toResponse(item))
	}
	response.SuccessPage(c, total, views)
}

// Create godoc
// @Summary 管理员生成注册码
// @Tags admin-registration-codes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CreateRequest true "生成数量"
// @Success 200 {object} CreateResponseDoc
// @Failure 400 {object} ErrorDoc
// @Router /admin/registration-codes [post]
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	items, err := h.service.Create(c.Request.Context(), middleware.MustUserID(c), req.Quantity)
	if err != nil {
		response.ErrorFrom(c, http.StatusBadRequest, err)
		return
	}
	views := make([]CodeResponse, 0, len(items))
	for _, item := range items {
		views = append(views, toResponse(item))
	}
	response.Success(c, gin.H{"results": views})
}

// Delete godoc
// @Summary 删除未使用注册码
// @Tags admin-registration-codes
// @Produce json
// @Security BearerAuth
// @Param id path int true "注册码 ID"
// @Success 200 {object} DeleteResponseDoc
// @Failure 409 {object} ErrorDoc
// @Router /admin/registration-codes/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid registration code id")
		return
	}
	if err = h.service.Delete(c.Request.Context(), id); err != nil {
		response.Error(c, http.StatusConflict, "delete registration code failed")
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func parseID(value string) (uint, error) {
	parsed, err := strconv.ParseUint(value, 10, 32)
	return uint(parsed), err
}
