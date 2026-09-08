package app

import (
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/settings"
	speechapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/speech"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	settingsrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/settings"
	speechinfra "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/speech"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	speechhttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/speech"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Use the fork-owned registration seam; no changes to upstream routing or tables.
func registerSpeech(engine *gin.Engine, db *gorm.DB, cfg *config.Runtime, auth middleware.SessionValidator, limiter middleware.RateLimiter) {
	if db == nil || cfg == nil {
		return
	}
	settingsService := settings.NewService(settingsrepo.NewRepo(db), cfg.Snapshot().DataEncryptionKey)
	service := speechapp.NewService(settingsService, speechinfra.NewTencent(), limiter)
	group := engine.Group("/api/v1", middleware.AuthMiddleware(cfg.Snapshot().JWTSecret, auth))
	speechhttp.NewHandler(service).Register(group)
}
