package invitation

import (
	"net/http"
	"strconv"

	appinvitation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/invitation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service *appinvitation.Service }

func NewHandler(service *appinvitation.Service) *Handler { return &Handler{service: service} }

// GetPanel godoc
// @Summary 获取我的邀请面板
// @Tags invitation
// @Produce json
// @Security BearerAuth
// @Success 200 {object} InvitationPanelResponse
// @Router /me/invitation [get]
func (h *Handler) GetPanel(c *gin.Context) {
	userID := middleware.MustUserID(c)
	panel, err := h.service.GetPanel(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "load invitation panel failed")
		return
	}
	response.Success(c, InvitationPanelResponse{
		InvitationCode: panel.InvitationCode,
		InviteLink:     panel.InviteLink,
		InviteCount:    panel.InviteCount,
	})
}

// ListInvitedUsers godoc
// @Summary 获取我邀请的用户列表
// @Tags invitation
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} InvitedUserListDoc
// @Router /me/invitations [get]
func (h *Handler) ListInvitedUsers(c *gin.Context) {
	userID := middleware.MustUserID(c)
	page, pageSize := parsePaging(c)
	items, total, err := h.service.ListInvitedUsers(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list invited users failed")
		return
	}
	views := make([]InvitedUserResponse, 0, len(items))
	for _, item := range items {
		views = append(views, InvitedUserResponse{
			RelationshipID:       item.RelationshipID,
			InvitedUserID:        item.InvitedUserID,
			InvitedDisplayName:   item.InvitedDisplayName,
			InvitedUsername:      item.InvitedUsername,
			InvitedAt:            formatTime(item.InvitedAt),
			InviterRewardNanousd: item.InviterRewardNanousd,
		})
	}
	response.SuccessPage(c, total, views)
}

// ListRelationships godoc
// @Summary 管理员查询邀请关系
// @Tags admin-invitation
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Param inviter_user_id query int false "邀请人ID"
// @Param invited_user_id query int false "被邀请人ID"
// @Success 200 {object} InvitationRelationshipListDoc
// @Router /admin/invitations [get]
func (h *Handler) ListRelationships(c *gin.Context) {
	page, pageSize := parsePaging(c)
	inviterUserID, _ := strconv.ParseUint(c.Query("inviter_user_id"), 10, 32)
	invitedUserID, _ := strconv.ParseUint(c.Query("invited_user_id"), 10, 32)
	items, total, err := h.service.ListRelationships(c.Request.Context(), page, pageSize, uint(inviterUserID), uint(invitedUserID))
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list invitation relationships failed")
		return
	}
	views := make([]InvitationRelationshipResponse, 0, len(items))
	for _, item := range items {
		views = append(views, InvitationRelationshipResponse{
			ID:                   item.ID,
			InviterUserID:        item.InviterUserID,
			InvitedUserID:        item.InvitedUserID,
			InvitationCode:       item.InvitationCode,
			InviteeRewardNanousd: item.InviteeRewardNanousd,
			InviterRewardNanousd: item.InviterRewardNanousd,
			InviteeRewardedAt:    formatTimePtr(item.InviteeRewardedAt),
			InviterRewardedAt:    formatTimePtr(item.InviterRewardedAt),
			CreatedAt:            formatTime(item.CreatedAt),
		})
	}
	response.SuccessPage(c, total, views)
}

func parsePaging(c *gin.Context) (int, int) {
	page, pageSize := 1, 20
	if value, err := strconv.Atoi(c.Query("page")); err == nil && value > 0 {
		page = value
	}
	if value, err := strconv.Atoi(c.Query("page_size")); err == nil && value > 0 {
		pageSize = value
	}
	return page, pageSize
}
