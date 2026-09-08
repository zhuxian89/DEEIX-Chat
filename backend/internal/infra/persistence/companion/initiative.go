package companion

import (
	"context"
	"errors"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type initiativeStateRecord struct {
	UserID             uint   `gorm:"primaryKey;autoIncrement:false"`
	Mode               string `gorm:"size:16"`
	LastAttemptAt      time.Time
	IdleAttemptAt      time.Time
	LastHomeAt         time.Time
	HomeDay            string `gorm:"size:10"`
	HomeCount          uint
	UnansweredHomes    uint
	HomeAfterUserID    uint
	IdleVisitID        string `gorm:"size:64"`
	IdleAfterMessageID uint
	LastIdleAt         time.Time
}

func (initiativeStateRecord) TableName() string { return "companion_initiative_states" }

type initiativeRecord struct {
	ID             string `gorm:"primaryKey;size:64"`
	UserID         uint   `gorm:"index:companion_initiative_history,priority:1"`
	ConversationID uint   `gorm:"index:companion_initiative_history,priority:2"`
	Kind           string `gorm:"size:16"`
	VisitID        string `gorm:"size:64"`
	AfterMessageID uint
	AfterUserID    uint
	Revision       uint
	Text           string `gorm:"type:text"`
	TopicJSON      string `gorm:"type:text"`
	CreatedAt      time.Time
	ExpiresAt      time.Time `gorm:"index"`
	AcceptedAt     time.Time `gorm:"index:companion_initiative_history,priority:3"`
}

func (initiativeRecord) TableName() string { return "companion_initiatives" }

func (s *Store) InitiativeState(ctx context.Context, userID uint) (*domain.InitiativeState, error) {
	row := initiativeStateRecord{UserID: userID}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).First(&row, "user_id = ?", userID).Error; err != nil {
		return nil, err
	}
	state := domain.InitiativeState(row)
	return &state, nil
}

func (s *Store) SetProactivity(ctx context.Context, userID uint, mode string) error {
	if _, err := s.Profile(ctx, userID); err != nil {
		return err
	}
	if _, err := s.InitiativeState(ctx, userID); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&profileRecord{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
			"quiet": mode == "off", "revision": gorm.Expr("revision + 1"), "refresh_token": "",
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return tx.Model(&initiativeStateRecord{}).Where("user_id = ?", userID).Update("mode", mode).Error
	})
}

// Reserve before model work, under the short companion lease. Failed or declined
// candidates still consume the attempt, bounding platform-paid calls and retries.
func (s *Store) ReserveInitiative(ctx context.Context, item domain.Initiative, token string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := guardInitiativeLease(tx, item, token); err != nil {
			return err
		}
		updates := map[string]interface{}{"last_attempt_at": item.CreatedAt}
		if item.Kind == "idle" {
			updates = map[string]interface{}{"idle_attempt_at": item.CreatedAt}
			updates["idle_visit_id"], updates["idle_after_message_id"] = item.VisitID, item.AfterMessageID
		}
		if err := tx.Model(&initiativeStateRecord{}).Where("user_id = ?", item.UserID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND accepted_at = ? AND expires_at < ?", item.UserID, time.Time{}, item.CreatedAt).Delete(&initiativeRecord{}).Error; err != nil {
			return err
		}
		row := initiativeRecord(item)
		return tx.Create(&row).Error
	})
}

func (s *Store) CompleteInitiative(ctx context.Context, item domain.Initiative) error {
	return s.db.WithContext(ctx).Model(&initiativeRecord{}).
		Where("id = ? AND user_id = ? AND revision = ? AND accepted_at = ?", item.ID, item.UserID, item.Revision, time.Time{}).
		Updates(map[string]interface{}{"text": item.Text, "topic_json": item.TopicJSON}).Error
}

func (s *Store) Initiative(ctx context.Context, userID uint, id string) (*domain.Initiative, error) {
	var row initiativeRecord
	err := s.db.WithContext(ctx).First(&row, "id = ? AND user_id = ?", id, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	item := domain.Initiative(row)
	return &item, err
}

func (s *Store) Initiatives(ctx context.Context, userID, conversationID uint) ([]domain.Initiative, error) {
	var rows []initiativeRecord
	err := s.db.WithContext(ctx).Where("user_id = ? AND conversation_id = ? AND accepted_at > ?", userID, conversationID, time.Time{}).
		Order("accepted_at DESC, id DESC").Limit(100).Find(&rows).Error
	items := make([]domain.Initiative, len(rows))
	for i, row := range rows {
		items[len(rows)-1-i] = domain.Initiative(row)
	}
	return items, err
}

// The caller rechecks the conversation boundary while owning the chat lease.
// This transaction makes acceptance, pacing and history insertion indivisible.
func (s *Store) AcceptInitiative(ctx context.Context, item domain.Initiative, token string, pace domain.InitiativeState) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := guardInitiativeLease(tx, item, token); err != nil {
			return err
		}
		result := tx.Model(&initiativeRecord{}).Where("id = ? AND user_id = ? AND accepted_at = ? AND expires_at > ?", item.ID, item.UserID, time.Time{}, item.AcceptedAt).Update("accepted_at", item.AcceptedAt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrBusy
		}
		row := initiativeStateRecord(pace)
		if err := tx.Model(&initiativeStateRecord{}).Where("user_id = ?", item.UserID).Select("*").Updates(&row).Error; err != nil {
			return err
		}
		if item.Kind == "home" {
			if err := tx.Model(&profileRecord{}).Where("user_id = ?", item.UserID).Updates(map[string]interface{}{
				"last_greeting": item.Text, "greeting_id": item.ID, "greeting_at": item.AcceptedAt,
				"greeting_after_user_id": item.AfterUserID, "last_topic_json": item.TopicJSON,
			}).Error; err != nil {
				return err
			}
		}
		if item.TopicJSON != "" {
			if err := tx.Model(&profileRecord{}).Where("user_id = ?", item.UserID).Updates(map[string]interface{}{"last_topic_json": item.TopicJSON, "last_topic_at": item.AcceptedAt}).Error; err != nil {
				return err
			}
		}
		var obsolete []string
		if err := tx.Model(&initiativeRecord{}).Where("user_id = ? AND accepted_at > ?", item.UserID, time.Time{}).Order("accepted_at DESC, id DESC").Offset(100).Pluck("id", &obsolete).Error; err != nil {
			return err
		}
		if len(obsolete) > 0 {
			return tx.Where("user_id = ? AND id IN ?", item.UserID, obsolete).Delete(&initiativeRecord{}).Error
		}
		return nil
	})
}

// Updating the lease row also takes a database write lock. A plain COUNT would
// allow a preference update to commit between revision validation and acceptance.
func guardInitiativeLease(tx *gorm.DB, item domain.Initiative, token string) error {
	result := tx.Model(&profileRecord{}).Where("user_id = ? AND lease_token = ? AND revision = ? AND quiet = ?", item.UserID, token, item.Revision, false).Update("lease_token", token)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrBusy
	}
	return nil
}
