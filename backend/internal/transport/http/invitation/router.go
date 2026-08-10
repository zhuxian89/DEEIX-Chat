package invitation

import (
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册用户侧邀请路由（需登录）。
func (m *Module) RegisterRoutes(group *gin.RouterGroup) {
	group.GET("/me/invitation", m.Handler.GetPanel)
	group.GET("/me/invitations", m.Handler.ListInvitedUsers)
}

// RegisterAdminRoutes 注册管理端邀请路由（需管理员）。
func (m *Module) RegisterAdminRoutes(group *gin.RouterGroup) {
	group.GET("/invitations", m.Handler.ListRelationships)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}
