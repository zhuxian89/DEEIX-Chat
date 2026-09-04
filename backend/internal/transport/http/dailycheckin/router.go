package dailycheckin

import "github.com/gin-gonic/gin"

func (m *Module) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/daily-checkin/status", m.Handler.GetStatus)
	group.POST("/daily-checkin/claim", m.Handler.Claim)
}
