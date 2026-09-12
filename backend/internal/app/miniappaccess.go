package app

import (
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	accessstore "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/miniappaccess"
	accesshttp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/miniappaccess"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func buildMiniappAccess(db *gorm.DB, cfg *config.Runtime) (*accessstore.Store, gin.HandlerFunc, error) {
	store, err := accessstore.NewStore(db)
	if err != nil {
		return nil, nil, err
	}
	return store, accesshttp.Middleware(cfg.Snapshot().JWTSecret, store), nil
}
