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
	registerSpeech(engine, db, cfg, auth, limiter)
	if err := registerMiniappTodo(engine, db, cfg, auth, limiter); err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DEEIX_COMPANION_ENABLED")), "false") {
		return nil
	}
	store, err := companionstore.NewStore(db)
	if err != nil {
		return err
	}
	service := &companionapp.Service{Store: store, Chat: chat}
	store.SetAppIDProvider(func() string { return cfg.Snapshot().WeChatMiniAppAppID })
	service.NotificationProvider = companionNotificationProvider(cfg)
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			select {
			case <-shutdown.Done():
				cancel()
			case <-ctx.Done():
			}
		}()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		var cursor uint
		for {
			batch, stop := context.WithTimeout(ctx, 4*time.Minute)
			next, err := service.TickAssistant(batch, cursor)
			stop()
			cursor = next
			if err != nil && ctx.Err() == nil && log != nil {
				log.Warn("companion_followup_tick_failed")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("DEEIX_COMPANION_TOPICS_ENABLED")), "false") {
		service.TopicProvider = &companionapp.LiveTopicProvider{Chat: chat}
	}
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			select {
			case <-shutdown.Done():
				cancel()
			case <-ctx.Done():
			}
		}()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			batch, stop := context.WithTimeout(ctx, 30*time.Second)
			err := service.TickReminders(batch)
			stop()
			if err != nil && ctx.Err() == nil && log != nil {
				log.Warn("companion_reminder_tick_failed")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	group := engine.Group("/api/v1", middleware.AuthMiddleware(cfg.Snapshot().JWTSecret, auth), middleware.RateLimit(limiter, cfg))
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			select {
			case <-shutdown.Done():
				cancel()
			case <-ctx.Done():
			}
		}()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			batch, stop := context.WithTimeout(ctx, 30*time.Second)
			err := service.DispatchReminders(batch)
			stop()
			if err != nil && ctx.Err() == nil && log != nil {
				log.Warn("companion_reminder_dispatch_failed")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
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
