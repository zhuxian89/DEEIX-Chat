// Package companion owns only companion_* tables. Existing chat tables and
// account/billing schemas are never migrated by this module.
package companion

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrBusy     = errors.New("助手正在回复，请稍后再试")
	ErrNotFound = errors.New("没有找到这位用户的助手")
	ErrInvalid  = errors.New("助手请求参数不正确")
)

type Profile struct {
	UserID               uint      `gorm:"primaryKey;autoIncrement:false" json:"-"`
	ConversationID       uint      `gorm:"index" json:"-"`
	ConversationPublicID string    `gorm:"size:64" json:"conversationPublicID"`
	Model                string    `gorm:"size:200" json:"-"`
	Quiet                bool      `json:"quiet"`
	Summary              string    `gorm:"type:text" json:"-"`
	SummaryAt            time.Time `json:"-"`
	ThroughID            uint      `json:"-"`
	ForgetThroughID      uint      `json:"-"`
	Revision             uint      `json:"-"`
	LeaseToken           string    `gorm:"size:64" json:"-"`
	LeaseUntil           int64     `json:"-"`
	RefreshToken         string    `gorm:"size:64" json:"-"`
	RefreshAfter         int64     `json:"-"`
	LastGreeting         string    `gorm:"type:text" json:"greeting"`
	GreetingID           string    `gorm:"size:64" json:"greetingID"`
	GreetingAt           time.Time `json:"greetingAt"`
	GreetingAfterUserID  uint      `json:"-"`
	ReadThroughID        uint      `json:"-"`
	CreatedAt            time.Time `json:"-"`
	UpdatedAt            time.Time `json:"-"`
}

func (Profile) TableName() string { return "companion_profiles" }

type Memory struct {
	ID              string    `gorm:"primaryKey;size:64" json:"id"`
	UserID          uint      `gorm:"uniqueIndex:companion_memory_owner_key;index" json:"-"`
	Key             string    `gorm:"size:80;uniqueIndex:companion_memory_owner_key" json:"key"`
	Value           string    `gorm:"type:text" json:"value"`
	Evidence        string    `gorm:"type:text" json:"evidence"`
	SourceMessageID uint      `json:"-"`
	ExpiresAt       time.Time `gorm:"index" json:"expiresAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (Memory) TableName() string { return "companion_memories" }

type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) (*Store, error) {
	if err := db.AutoMigrate(&Profile{}, &Memory{}); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Profile(ctx context.Context, userID uint) (*Profile, error) {
	var p Profile
	err := s.db.WithContext(ctx).First(&p, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (s *Store) Ensure(ctx context.Context, userID uint) (*Profile, error) {
	if userID == 0 {
		return nil, ErrInvalid
	}
	p := Profile{UserID: userID}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error; err != nil {
		return nil, err
	}
	return s.Profile(ctx, userID)
}

// The database lease serializes chat and memory edits across devices/instances.
func (s *Store) Acquire(ctx context.Context, userID uint) (string, error) {
	token := uuid.NewString()
	result := s.db.WithContext(ctx).Model(&Profile{}).Where("user_id = ? AND lease_until < ?", userID, time.Now().Unix()).Updates(map[string]interface{}{
		"lease_token": token, "lease_until": time.Now().Add(90 * time.Second).Unix(),
	})
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected != 1 {
		return "", ErrBusy
	}
	return token, nil
}

func (s *Store) Renew(ctx context.Context, userID uint, token string) error {
	result := s.db.WithContext(ctx).Model(&Profile{}).Where("user_id = ? AND lease_token = ?", userID, token).Update("lease_until", time.Now().Add(90*time.Second).Unix())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrBusy
	}
	return nil
}

func (s *Store) Release(userID uint, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.db.WithContext(ctx).Model(&Profile{}).Where("user_id = ? AND lease_token = ?", userID, token).Updates(map[string]interface{}{"lease_token": "", "lease_until": 0})
}

func (s *Store) Memories(ctx context.Context, userID uint) ([]Memory, error) {
	items := []Memory{}
	err := s.db.WithContext(ctx).Where("user_id = ? AND expires_at > ?", userID, time.Now()).Order("updated_at DESC, id ASC").Limit(60).Find(&items).Error
	return items, err
}

// Forget resets the rolling summary and moves the extraction/context boundary
// past all old messages. Revision checking rejects already-running extraction.
func (s *Store) Forget(ctx context.Context, userID, through uint, memoryID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("user_id = ?", userID)
		if memoryID != "" {
			query = query.Where("id = ?", memoryID)
		}
		result := query.Delete(&Memory{})
		if result.Error != nil {
			return result.Error
		}
		if memoryID != "" && result.RowsAffected != 1 {
			return ErrNotFound
		}
		return tx.Model(&Profile{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
			"summary": "", "forget_through_id": through, "through_id": through,
			"revision": gorm.Expr("revision + 1"), "refresh_token": "",
			"last_greeting": "", "greeting_id": "",
		}).Error
	})
}

func (s *Store) Edit(ctx context.Context, userID, through uint, memoryID, value string) error {
	if memoryID == "" || value == "" || len([]rune(value)) > 120 {
		return ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Memory{}).Where("user_id = ? AND id = ?", userID, memoryID).Updates(map[string]interface{}{
			"value": value, "evidence": "由你修改", "source_message_id": 0,
			"expires_at": time.Now().Add(90 * 24 * time.Hour), "updated_at": time.Now(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return tx.Model(&Profile{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
			"summary": "", "forget_through_id": through, "through_id": through,
			"revision": gorm.Expr("revision + 1"), "refresh_token": "",
			"last_greeting": "", "greeting_id": "",
		}).Error
	})
}
