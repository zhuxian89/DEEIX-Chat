package conversation

import (
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// GetConversationDefaultImageModel godoc
// @Summary 获取生图入口默认模型
// @Description 返回后台配置的候选模型；客户端仍须与当前用户可用的生图模型目录交叉校验。空值表示使用第一个可用生图模型。
// @Tags conversations
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ConversationDefaultModelCandidateResponseDoc
// @Failure 401 {object} response.Envelope
// @Router /conversations/default-image-model-candidate [get]
func (h *Handler) GetConversationDefaultImageModel(c *gin.Context) {
	result := ConversationDefaultModelCandidateResponse{
		PlatformModelName: h.service.GetConversationDefaultImageModel(),
	}
	if result.PlatformModelName != "" {
		result.Source = "system_default"
	}
	response.Success(c, result)
}
