package registrationcode

import "github.com/gin-gonic/gin"

func (m *Module) RegisterAdminRoutes(adminGroup *gin.RouterGroup) {
	adminGroup.GET("/registration-codes", m.Handler.List)
	adminGroup.POST("/registration-codes", m.Handler.Create)
	adminGroup.DELETE("/registration-codes/:id", m.Handler.Delete)
}
