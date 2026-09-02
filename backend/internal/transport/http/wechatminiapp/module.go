package wechatminiapp

import "github.com/gin-gonic/gin"

type Module struct {
	Handler *Handler
}

func NewModule(handler *Handler) *Module { return &Module{Handler: handler} }

func (m *Module) RegisterPublicRoutes(api *gin.RouterGroup) {
	api.POST("/auth/wechat-miniapp/login", m.Handler.Login)
}
