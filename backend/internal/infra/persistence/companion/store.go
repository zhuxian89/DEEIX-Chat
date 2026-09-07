// Package companion persists only companion-owned tables.
package companion

import (
	"context"
	"errors"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrBusy     = domain.ErrBusy
	ErrNotFound = domain.ErrNotFound
	ErrInvalid  = domain.ErrInvalid
)

type profileRecord struct {
	UserID               uint   `gorm:"primaryKey;autoIncrement:false"`
	ConversationID       uint   `gorm:"index"`
	ConversationPublicID string `gorm:"size:64"`
	Model                string `gorm:"size:200"`
	Quiet                bool
	Summary              string `gorm:"type:text"`
	SummaryAt            time.Time
	ThroughID            uint
	ForgetThroughID      uint
	Revision             uint
	LeaseToken           string `gorm:"size:64"`
	LeaseUntil           int64
	RefreshToken         string `gorm:"size:64"`
	RefreshAfter         int64
	LastGreeting         string `gorm:"type:text"`
	GreetingID           string `gorm:"size:64"`
	GreetingAt           time.Time
	GreetingAfterUserID  uint
	ReadThroughID        uint
	LastTopicJSON        string `gorm:"type:text"`
	LastTopicAt          time.Time
	SeenTopicsJSON       string `gorm:"type:text"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (profileRecord) TableName() string { return "companion_profiles" }

type memoryRecord struct {
	ID              string `gorm:"primaryKey;size:64"`
	UserID          uint   `gorm:"uniqueIndex:companion_memory_owner_key;index"`
	Key             string `gorm:"size:80;uniqueIndex:companion_memory_owner_key"`
	Value           string `gorm:"type:text"`
	Evidence        string `gorm:"type:text"`
	SourceMessageID uint
	SourceAt        time.Time
	ExpiresAt       time.Time `gorm:"index"`
	UpdatedAt       time.Time
}

func (memoryRecord) TableName() string { return "companion_memories" }

// Keep explicit topic controls while bounding the combined memory collection.
func trimMemories(tx *gorm.DB, userID uint) error {
	var obsolete []string
	if err := tx.Model(&memoryRecord{}).Where("user_id = ?", userID).
		Order("CASE WHEN key LIKE 'topic:%' THEN 0 ELSE 1 END, updated_at DESC, id ASC").
		Offset(60).Limit(16).Pluck("id", &obsolete).Error; err != nil {
		return err
	}
	if len(obsolete) == 0 {
		return nil
	}
	return tx.Where("user_id = ? AND id IN ?", userID, obsolete).Delete(&memoryRecord{}).Error
}

type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) (*Store, error) {
	if err := db.AutoMigrate(&profileRecord{}, &memoryRecord{}, &topicCacheRecord{}); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Profile(ctx context.Context, userID uint) (*domain.Profile, error) {
	var p profileRecord
	err := s.db.WithContext(ctx).First(&p, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	result := domain.Profile(p)
	return &result, err
}

func (s *Store) Ensure(ctx context.Context, userID uint) (*domain.Profile, error) {
	if userID == 0 {
		return nil, ErrInvalid
	}
	p := profileRecord{UserID: userID}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&p).Error; err != nil {
		return nil, err
	}
	return s.Profile(ctx, userID)
}

// The database lease serializes chat and memory edits across devices/instances.
func (s *Store) Acquire(ctx context.Context, userID uint) (string, error) {
	token := uuid.NewString()
	result := s.db.WithContext(ctx).Model(&profileRecord{}).Where("user_id = ? AND lease_until < ?", userID, time.Now().Unix()).Updates(map[string]interface{}{
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
	result := s.db.WithContext(ctx).Model(&profileRecord{}).Where("user_id = ? AND lease_token = ?", userID, token).Update("lease_until", time.Now().Add(90*time.Second).Unix())
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
	s.db.WithContext(ctx).Model(&profileRecord{}).Where("user_id = ? AND lease_token = ?", userID, token).Updates(map[string]interface{}{"lease_token": "", "lease_until": 0})
}

func (s *Store) Memories(ctx context.Context, userID uint) ([]domain.Memory, error) {
	items := []memoryRecord{}
	err := s.db.WithContext(ctx).Where("user_id = ? AND expires_at > ?", userID, time.Now()).Order("updated_at DESC, id ASC").Limit(60).Find(&items).Error
	memories := make([]domain.Memory, len(items))
	for i, item := range items {
		memories[i] = domain.Memory(item)
	}
	return memories, err
}

// Forget resets the rolling summary and moves the extraction/context boundary
// past all old messages. Revision checking rejects already-running extraction.
func (s *Store) Forget(ctx context.Context, userID, through uint, memoryID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("user_id = ?", userID)
		if memoryID != "" {
			query = query.Where("id = ?", memoryID)
		}
		result := query.Delete(&memoryRecord{})
		if result.Error != nil {
			return result.Error
		}
		if memoryID != "" && result.RowsAffected != 1 {
			return ErrNotFound
		}
		return resetMemoryContext(tx, userID, through)
	})
}

func (s *Store) Edit(ctx context.Context, userID, through uint, memoryID, value string) error {
	if memoryID == "" || value == "" || len([]rune(value)) > 120 {
		return ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&memoryRecord{}).Where("user_id = ? AND id = ?", userID, memoryID).Updates(map[string]interface{}{
			"value": value, "evidence": "由你修改", "source_message_id": 0, "source_at": time.Now(),
			"expires_at": time.Now().Add(90 * 24 * time.Hour), "updated_at": time.Now(),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return resetMemoryContext(tx, userID, through)
	})
}

// Both memory edits and deletions invalidate context within the same transaction.
func resetMemoryContext(tx *gorm.DB, userID, through uint) error {
	return tx.Model(&profileRecord{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
		"summary":           "",
		"forget_through_id": through,
		"through_id":        through,
		"revision":          gorm.Expr("revision + 1"),
		"refresh_token":     "",
		"last_greeting":     "",
		"greeting_id":       "",
		"last_topic_json":   "",
		"seen_topics_json":  "",
	}).Error
}

type topicCacheRecord struct {
	Category     string `gorm:"primaryKey;size:40"`
	TopicsJSON   string `gorm:"type:text"`
	RefreshToken string `gorm:"size:64"`
	RefreshAfter int64
	FetchedAt    time.Time
}

func (topicCacheRecord) TableName() string { return "companion_topic_caches" }
