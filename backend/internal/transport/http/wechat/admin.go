package wechat

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	appwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/wechat"
	domainwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechat"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	service *appwechat.AdminService
}

func NewAdminHandler(service *appwechat.AdminService) *AdminHandler {
	return &AdminHandler{service: service}
}

type keywordRuleRequest struct {
	Keyword    string `json:"keyword" binding:"required,max=128"`
	Action     string `json:"action" binding:"required,max=64"`
	TemplateID uint   `json:"templateId" binding:"required"`
	Enabled    *bool  `json:"enabled"`
}

type replyTemplateRequest struct {
	Name         string `json:"name" binding:"required,max=128"`
	ResponseType string `json:"responseType" binding:"required,max=32"`
	Content      string `json:"content" binding:"required"`
	Enabled      *bool  `json:"enabled"`
}

type actionResponse struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type summaryResponse struct {
	IssuanceCount int64 `json:"issuanceCount"`
	SuccessCount  int64 `json:"successCount"`
	FailureCount  int64 `json:"failureCount"`
}

type ruleResponse struct {
	ID              uint   `json:"id"`
	Keyword         string `json:"keyword"`
	Action          string `json:"action"`
	TemplateID      uint   `json:"templateId"`
	TemplateName    string `json:"templateName"`
	TemplateType    string `json:"templateType"`
	TemplateContent string `json:"templateContent"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

type templateResponse struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	ResponseType string `json:"responseType"`
	Content      string `json:"content"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type issuanceResponse struct {
	ID                 uint    `json:"id"`
	OpenID             string  `json:"openID"`
	RegistrationCodeID uint    `json:"registrationCodeId"`
	Code               string  `json:"code"`
	Status             string  `json:"status"`
	UsedByUserID       uint    `json:"usedByUserId"`
	UsedAt             *string `json:"usedAt"`
	CreatedAt          string  `json:"createdAt"`
}

type logResponse struct {
	ID                 uint   `json:"id"`
	OpenID             string `json:"openID"`
	Keyword            string `json:"keyword"`
	Action             string `json:"action"`
	TemplateID         uint   `json:"templateId"`
	RegistrationCodeID uint   `json:"registrationCodeId"`
	Result             string `json:"result"`
	ErrorCode          string `json:"errorCode"`
	ErrorMessage       string `json:"errorMessage"`
	CreatedAt          string `json:"createdAt"`
}

func (h *AdminHandler) Actions(c *gin.Context) {
	items := h.service.Actions()
	result := make([]actionResponse, 0, len(items))
	for _, item := range items {
		result = append(result, actionResponse{Key: item.Key, Label: item.Label})
	}
	response.Success(c, result)
}

func (h *AdminHandler) Summary(c *gin.Context) {
	stats, err := h.service.Stats(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "load WeChat summary failed")
		return
	}
	response.Success(c, summaryResponse{IssuanceCount: stats.IssuanceCount, SuccessCount: stats.SuccessCount, FailureCount: stats.FailureCount})
}

func (h *AdminHandler) ListRules(c *gin.Context) {
	page, pageSize := adminPageParams(c)
	items, total, err := h.service.ListRules(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list WeChat keyword rules failed")
		return
	}
	result := make([]ruleResponse, 0, len(items))
	for _, item := range items {
		result = append(result, toRuleResponse(item))
	}
	response.SuccessPage(c, total, result)
}

func (h *AdminHandler) CreateRule(c *gin.Context) {
	var req keywordRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item := domainwechat.KeywordRule{Keyword: req.Keyword, Action: req.Action, TemplateID: req.TemplateID, Enabled: enabled}
	if err := h.service.SaveRule(c.Request.Context(), item); err != nil {
		writeAdminError(c, err, "create WeChat keyword rule failed")
		return
	}
	response.Success(c, gin.H{"saved": true})
}

func (h *AdminHandler) UpdateRule(c *gin.Context) {
	id, err := parseAdminID(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid WeChat keyword rule id")
		return
	}
	var req keywordRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if err := h.service.SaveRule(c.Request.Context(), domainwechat.KeywordRule{ID: id, Keyword: req.Keyword, Action: req.Action, TemplateID: req.TemplateID}); err != nil {
		writeAdminError(c, err, "update WeChat keyword rule failed")
		return
	}
	if req.Enabled != nil {
		if err := h.service.SetRuleEnabled(c.Request.Context(), id, *req.Enabled); err != nil {
			writeAdminError(c, err, "update WeChat keyword rule status failed")
			return
		}
	}
	response.Success(c, gin.H{"saved": true})
}

func (h *AdminHandler) SetRuleEnabled(c *gin.Context) {
	id, err := parseAdminID(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid WeChat keyword rule id")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if err := h.service.SetRuleEnabled(c.Request.Context(), id, req.Enabled); err != nil {
		writeAdminError(c, err, "update WeChat keyword rule status failed")
		return
	}
	response.Success(c, gin.H{"saved": true})
}

func (h *AdminHandler) ListTemplates(c *gin.Context) {
	page, pageSize := adminPageParams(c)
	items, total, err := h.service.ListTemplates(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list WeChat reply templates failed")
		return
	}
	result := make([]templateResponse, 0, len(items))
	for _, item := range items {
		result = append(result, toTemplateResponse(item))
	}
	response.SuccessPage(c, total, result)
}

func (h *AdminHandler) CreateTemplate(c *gin.Context) {
	var req replyTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item := domainwechat.ReplyTemplate{Name: req.Name, ResponseType: req.ResponseType, Content: req.Content, Enabled: enabled}
	if err := h.service.SaveTemplate(c.Request.Context(), item); err != nil {
		writeAdminError(c, err, "create WeChat reply template failed")
		return
	}
	response.Success(c, gin.H{"saved": true})
}

func (h *AdminHandler) UpdateTemplate(c *gin.Context) {
	id, err := parseAdminID(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid WeChat reply template id")
		return
	}
	var req replyTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if err := h.service.SaveTemplate(c.Request.Context(), domainwechat.ReplyTemplate{ID: id, Name: req.Name, ResponseType: req.ResponseType, Content: req.Content}); err != nil {
		writeAdminError(c, err, "update WeChat reply template failed")
		return
	}
	if req.Enabled != nil {
		if err := h.service.SetTemplateEnabled(c.Request.Context(), id, *req.Enabled); err != nil {
			writeAdminError(c, err, "update WeChat reply template status failed")
			return
		}
	}
	response.Success(c, gin.H{"saved": true})
}

func (h *AdminHandler) SetTemplateEnabled(c *gin.Context) {
	id, err := parseAdminID(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid WeChat reply template id")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if err := h.service.SetTemplateEnabled(c.Request.Context(), id, req.Enabled); err != nil {
		writeAdminError(c, err, "update WeChat reply template status failed")
		return
	}
	response.Success(c, gin.H{"saved": true})
}

func (h *AdminHandler) ListIssuances(c *gin.Context) {
	page, pageSize := adminPageParams(c)
	items, total, err := h.service.ListIssuances(c.Request.Context(), page, pageSize, c.Query("q"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list WeChat issuance records failed")
		return
	}
	result := make([]issuanceResponse, 0, len(items))
	for _, item := range items {
		result = append(result, toIssuanceResponse(item))
	}
	response.SuccessPage(c, total, result)
}

func (h *AdminHandler) ListLogs(c *gin.Context) {
	page, pageSize := adminPageParams(c)
	items, total, err := h.service.ListLogs(c.Request.Context(), page, pageSize, c.Query("result"), c.Query("action"), c.Query("q"))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list WeChat invocation logs failed")
		return
	}
	result := make([]logResponse, 0, len(items))
	for _, item := range items {
		result = append(result, toLogResponse(item))
	}
	response.SuccessPage(c, total, result)
}

func adminPageParams(c *gin.Context) (int, int) {
	page, pageSize := 1, 20
	if value, err := strconv.Atoi(c.Query("page")); err == nil && value > 0 {
		page = value
	}
	if value, err := strconv.Atoi(c.Query("page_size")); err == nil && value > 0 {
		pageSize = value
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func parseAdminID(value string) (uint, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
	if err != nil || parsed == 0 {
		return 0, errors.New("invalid id")
	}
	return uint(parsed), nil
}

func writeAdminError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, appwechat.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid WeChat management input")
	case errors.Is(err, appwechat.ErrDuplicate):
		response.Error(c, http.StatusConflict, "WeChat management record already exists")
	case errors.Is(err, appwechat.ErrNotFound):
		response.Error(c, http.StatusNotFound, "WeChat management record not found")
	default:
		response.Error(c, http.StatusInternalServerError, fallback)
	}
}

func toRuleResponse(item domainwechat.KeywordRule) ruleResponse {
	return ruleResponse{ID: item.ID, Keyword: item.Keyword, Action: item.Action, TemplateID: item.TemplateID, TemplateName: item.TemplateName, TemplateType: item.TemplateType, TemplateContent: item.TemplateContent, Enabled: item.Enabled, CreatedAt: item.CreatedAt.Format("2006-01-02T15:04:05.999Z07:00"), UpdatedAt: item.UpdatedAt.Format("2006-01-02T15:04:05.999Z07:00")}
}

func toTemplateResponse(item domainwechat.ReplyTemplate) templateResponse {
	return templateResponse{ID: item.ID, Name: item.Name, ResponseType: item.ResponseType, Content: item.Content, Enabled: item.Enabled, CreatedAt: item.CreatedAt.Format("2006-01-02T15:04:05.999Z07:00"), UpdatedAt: item.UpdatedAt.Format("2006-01-02T15:04:05.999Z07:00")}
}

func toIssuanceResponse(item domainwechat.IssuanceRecord) issuanceResponse {
	var usedAt *string
	if item.UsedAt != nil {
		value := item.UsedAt.Format("2006-01-02T15:04:05.999Z07:00")
		usedAt = &value
	}
	return issuanceResponse{ID: item.ID, OpenID: item.OpenID, RegistrationCodeID: item.RegistrationCodeID, Code: item.Code, Status: item.Status, UsedByUserID: item.UsedByUserID, UsedAt: usedAt, CreatedAt: item.CreatedAt.Format("2006-01-02T15:04:05.999Z07:00")}
}

func toLogResponse(item domainwechat.InvocationLog) logResponse {
	return logResponse{ID: item.ID, OpenID: item.OpenID, Keyword: item.Keyword, Action: item.Action, TemplateID: item.TemplateID, RegistrationCodeID: item.RegistrationCodeID, Result: item.Result, ErrorCode: item.ErrorCode, ErrorMessage: item.ErrorMessage, CreatedAt: item.CreatedAt.Format("2006-01-02T15:04:05.999Z07:00")}
}
