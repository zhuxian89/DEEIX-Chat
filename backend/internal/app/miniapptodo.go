package app

import (
	todoapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/miniapptodo"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	todostore "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/miniapptodo"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	todohttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/miniapptodo"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func registerMiniappTodo(engine *gin.Engine, db *gorm.DB, cfg *config.Runtime, auth middleware.SessionValidator, limiter middleware.RateLimiter) error {
	store, err := todostore.NewStore(db)
	if err != nil {
		return err
	}
	service := todoapp.NewService(store, todoapp.Config{CurrentAppID: func() string { return cfg.Snapshot().WeChatMiniAppAppID }})
	group := engine.Group("/api/v1", middleware.AuthMiddleware(cfg.Snapshot().JWTSecret, auth), middleware.RateLimit(limiter, cfg))
	todohttp.NewHandler(service).RegisterRoutes(group)
	return nil
}
