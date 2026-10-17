package companion

import (
	"context"
	"errors"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Receipts survive consumption, disabling and restarts so an HTTP retry cannot
// grant another send. These records belong only to the companion feature.
type notificationConsentRecord struct {
	UserID     uint   `gorm:"primaryKey;autoIncrement:false"`
	RequestID  string `gorm:"primaryKey;size:100"`
	TemplateID string `gorm:"size:128"`
	AppID      string `gorm:"size:128"`
	OpenID     string `gorm:"size:128"`
	Automatic  bool
	CreatedAt  time.Time
}

func (notificationConsentRecord) TableName() string { return "companion_notification_consents" }

func (s *Store) GrantNotification(ctx context.Context, user uint, n domain.NotificationSettings, consent domain.NotificationConsent) error {
	if !n.Enabled || len(consent.RequestID) < 8 || len(consent.RequestID) > 100 {
		return ErrInvalid
	}
	return s.writeNotificationSettings(ctx, user, n, true, consent)
}

func (s *Store) writeNotificationSettings(ctx context.Context, user uint, n domain.NotificationSettings, grant bool, consent domain.NotificationConsent) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&notificationRecord{UserID: user}).Error; err != nil {
			return err
		}
		// Serializes grants with disabling and the dispatcher's credit decrement.
		if err := tx.Model(&notificationRecord{}).Where("user_id = ?", user).Update("credits", gorm.Expr("credits")).Error; err != nil {
			return err
		}
		var current notificationRecord
		if err := tx.First(&current, "user_id = ?", user).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{"enabled": n.Enabled, "template_id": n.TemplateID}
		if !n.Enabled {
			updates["credits"] = 0
			updates["bound_app_id"], updates["bound_open_id"] = "", ""
			return tx.Model(&notificationRecord{}).Where("user_id = ?", user).Updates(updates).Error
		}
		binding, err := (&Store{db: tx, appID: s.appID}).binding(ctx, user)
		if err != nil {
			return err
		}
		if n.AppID != "" && n.AppID != binding.AppID {
			return ErrBusy
		}
		if consent.RequestID != "" {
			var prior notificationConsentRecord
			err := tx.First(&prior, "user_id = ? AND request_id = ?", user, consent.RequestID).Error
			if err == nil {
				if prior.TemplateID != n.TemplateID || prior.AppID != binding.AppID || prior.OpenID != binding.OpenID || prior.Automatic != consent.Automatic {
					return ErrInvalid
				}
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		sameBinding := current.TemplateID == n.TemplateID && current.BoundAppID == binding.AppID && current.BoundOpenID == binding.OpenID
		if consent.Automatic && (!current.Enabled || !sameBinding) {
			return ErrBusy
		}
		if grant {
			credits := current.Credits
			if !sameBinding || !current.Enabled {
				credits = 0
			}
			if consent.RequestID != "" {
				if err := tx.Create(&notificationConsentRecord{UserID: user, RequestID: consent.RequestID, TemplateID: n.TemplateID, AppID: binding.AppID, OpenID: binding.OpenID, Automatic: consent.Automatic, CreatedAt: time.Now().UTC()}).Error; err != nil {
					return err
				}
				credits++
			} else if credits < 1 {
				// Old clients have no receipt ID. Preserve compatibility without
				// multiplying unidentifiable retries or erasing accumulated grants.
				credits = 1
			}
			updates["credits"] = credits
			updates["last_consent_at"] = time.Now().UTC()
			updates["bound_app_id"], updates["bound_open_id"] = binding.AppID, binding.OpenID
		}
		return tx.Model(&notificationRecord{}).Where("user_id = ?", user).Updates(updates).Error
	})
}
