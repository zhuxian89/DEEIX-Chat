package app

import (
	"context"
	"os"
	"strings"
	"time"

	companionapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	companionstore "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/lifecycle"
	companionhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// registerCompanion is the entire additive integration seam. All migrations
// belong to this feature; disabling it leaves the ordinary application intact.
func registerCompanion(engine *gin.Engine, db *gorm.DB, cfg *config.Runtime, auth middleware.SessionValidator, chat *conversation.Service, limiter middleware.RateLimiter, shutdown *lifecycle.Shutdown, log *zap.Logger) error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DEEIX_COMPANION_ENABLED")), "false") {
		return nil
	}
	store, err := companionstore.NewStore(db)
	if err != nil {
		return err
	}
	service := &companionapp.Service{Store: store, Chat: chat}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("DEEIX_COMPANION_TOPICS_ENABLED")), "false") {
		service.TopicProvider = &companionapp.LiveTopicProvider{Chat: chat}
	}
	group := engine.Group("/api/v1", middleware.AuthMiddleware(cfg.Snapshot().JWTSecret, auth), middleware.RateLimit(limiter, cfg))
	companionhttp.NewHandler(service, cfg, shutdown, log).Register(group)
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := store.PurgeDeletedUsers(ctx)
			cancel()
			if err != nil && log != nil {
				log.Warn("companion_orphan_cleanup_failed", zap.Error(err))
			}
			select {
			case <-shutdown.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}
