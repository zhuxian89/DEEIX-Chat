package miniappaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniappaccess"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type session struct {
	SessionID      string `gorm:"primaryKey;size:64"`
	UserID         uint   `gorm:"not null;index"`
	AppID          string `gorm:"size:64;not null"`
	OpenID         string `gorm:"size:128;not null"`
	Reauthenticate bool   `gorm:"not null"`
	CreatedAt      time.Time
}

func (session) TableName() string { return "miniapp_access_sessions" }

type migration struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
}

func (migration) TableName() string { return "miniapp_access_migrations" }

type Store struct{ db *gorm.DB }

// NewStore performs the one-time cutover before this process accepts requests.
// Deploy with old application processes stopped: mixed old/new login issuers are unsupported.
func NewStore(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("access database unavailable")
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(734920163002)).Error; err != nil {
				return err
			}
		}
		if err := tx.AutoMigrate(&session{}, &migration{}); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&migration{}).Where("id = ?", 1).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return nil
		}
		if err := cutover(tx); err != nil {
			return err
		}
		return tx.Create(&migration{ID: 1}).Error
	})
	if err != nil {
		return nil, fmt.Errorf("initialize miniapp access: %w", err)
	}
	return &Store{db: db}, nil
}

func cutover(tx *gorm.DB) error {
	// Only successful login events establish Web provenance; refresh/activity do not.
	web := make(map[string]uint)
	mini := make(map[string]bool)
	var events []model.UserAuthEvent
	err := tx.Where("result = ? AND event_type IN ?", "success", []string{"login", "provider_login", "two_factor_verify", "email_register", "wechat_miniapp_login", "wechat_miniapp_register"}).FindInBatches(&events, 500, func(_ *gorm.DB, _ int) error {
		for _, e := range events {
			var detail struct {
				SessionID string `json:"session_id"`
			}
			if json.Unmarshal([]byte(e.DetailJSON), &detail) != nil || detail.SessionID == "" {
				continue
			}
			if e.EventType == "wechat_miniapp_login" || e.EventType == "wechat_miniapp_register" {
				mini[detail.SessionID] = true
			} else {
				web[detail.SessionID] = e.UserID
			}
		}
		return nil
	}).Error
	if err != nil {
		return err
	}
	var sessions []model.UserSession
	return tx.Where("revoked_at IS NULL AND expires_at > ?", time.Now()).FindInBatches(&sessions, 500, func(_ *gorm.DB, _ int) error {
		var rows []session
		for _, s := range sessions {
			if s.UserID != 0 && web[s.SessionID] == s.UserID && !mini[s.SessionID] {
				continue
			}
			rows = append(rows, session{SessionID: s.SessionID, UserID: s.UserID, Reauthenticate: true})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	}).Error
}

// Register is completed before any newly signed miniapp credentials leave the server.
func (s *Store) Register(ctx context.Context, uid uint, sid, appID, openID string) error {
	if uid == 0 || sid == "" || appID == "" || openID == "" {
		return errors.New("invalid miniapp session identity")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(uid)).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&session{}).Where("user_id = ? AND app_id = ? AND open_id = ? AND reauthenticate = ?", uid, appID, openID, false).Update("reauthenticate", true).Error; err != nil {
			return err
		}
		return tx.Create(&session{SessionID: sid, UserID: uid, AppID: appID, OpenID: openID}).Error
	})
}

func (s *Store) Access(ctx context.Context, uid uint, sid string) (domain.Decision, error) {
	var row session
	err := s.db.WithContext(ctx).Where("session_id = ?", sid).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Decision{}, nil
	}
	if err != nil {
		return domain.Decision{}, err
	}
	if row.UserID != uid || row.Reauthenticate {
		return domain.Decision{Reauthenticate: true}, nil
	}
	var count int64
	err = s.db.WithContext(ctx).Table("wechat_miniapp_bindings AS b").Joins("JOIN miniapp_entry_unlocks AS u ON u.app_id = b.app_id AND u.open_id = b.open_id").Where("b.user_id = ? AND b.app_id = ? AND b.open_id = ? AND b.revoked_at IS NULL", uid, row.AppID, row.OpenID).Count(&count).Error
	return domain.Decision{MiniApp: true, Unlocked: count > 0}, err
}
