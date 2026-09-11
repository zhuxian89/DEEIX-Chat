package app

import (
	"context"

	todoapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/miniapptodo"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/settings"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	todostore "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/miniapptodo"
	settingsrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/settings"
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
	parameters := settings.NewService(settingsrepo.NewRepo(db), cfg.Snapshot().DataEncryptionKey)
	service := todoapp.NewService(store, todoapp.Config{
		CurrentAppID: func() string { return cfg.Snapshot().WeChatMiniAppAppID },
		FeedbackCode: func(ctx context.Context) (string, error) {
			values, err := parameters.RuntimeValuesByNamespace(ctx, "miniapp_todo")
			return values["feedback_code"], err
		},
	})
	group := engine.Group("/api/v1", middleware.AuthMiddleware(cfg.Snapshot().JWTSecret, auth), middleware.RateLimit(limiter, cfg))
	todohttp.NewHandler(service).RegisterRoutes(group)
	return nil
}
