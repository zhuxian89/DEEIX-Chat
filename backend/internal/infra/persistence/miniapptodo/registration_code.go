package miniapptodo

import (
	"context"
	"strings"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/registrationcode"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// Scalar references deliberately avoid foreign keys to website account/code tables.
type registrationCodeBinding struct {
	RegistrationCodeID uint      `gorm:"primaryKey;autoIncrement:false"`
	AppID              string    `gorm:"size:64;not null;uniqueIndex:ux_miniapp_registration_owner"`
	OpenID             string    `gorm:"size:128;not null;uniqueIndex:ux_miniapp_registration_owner"`
	BoundAt            time.Time `gorm:"not null"`
}

func (registrationCodeBinding) TableName() string { return "miniapp_registration_code_bindings" }

// RedeemRegistrationCode consumes only the miniapp entitlement. Website redemption
// state is read-only here, including codes already used to register a website user.
func (s *Store) RedeemRegistrationCode(ctx context.Context, o domain.Owner, code string, now time.Time) (time.Time, error) {
	if !o.Valid() {
		return time.Time{}, domain.ErrIdentity
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if !strings.HasPrefix(code, "REG-") || len(code) <= 4 || len(code) > 128 {
		return time.Time{}, domain.ErrInvalidCode
	}
	var unlockedAt time.Time
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ownerLock(tx, o); err != nil {
			return err
		}
		store := &Store{db: tx}
		at, err := store.UnlockedAt(ctx, o)
		if err != nil {
			return err
		}
		if at != nil {
			unlockedAt = *at
			return nil
		}
		var registration struct{ ID uint }
		// Do not put plaintext registration codes in SQL logs, including slow/error logs.
		result := tx.Session(&gorm.Session{Logger: logger.Discard}).
			Table("registration_codes AS c").Select("c.id").
			Where("c.code = ? AND c.status IN ?", code, []string{registrationcode.StatusActive, registrationcode.StatusUsed}).
			Where("EXISTS (SELECT 1 FROM wechat_registration_issuances AS i WHERE i.registration_code_id = c.id)").
			Limit(1).Find(&registration)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrInvalidCode
		}
		binding := registrationCodeBinding{RegistrationCodeID: registration.ID, AppID: o.AppID, OpenID: o.OpenID, BoundAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&binding).Error; err != nil {
			return err
		}
		var existing registrationCodeBinding
		result = tx.Where("registration_code_id = ?", registration.ID).Limit(1).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 || existing.AppID != o.AppID || existing.OpenID != o.OpenID {
			return domain.ErrInvalidCode
		}
		// Binding and permanent access commit together; preserve the original time on retry.
		unlockedAt, err = store.Unlock(ctx, o, existing.BoundAt)
		return err
	})
	if err != nil {
		return time.Time{}, err
	}
	return unlockedAt, nil
}
