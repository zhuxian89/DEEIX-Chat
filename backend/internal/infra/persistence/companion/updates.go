package companion

import (
	"context"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) SaveConversation(ctx context.Context, p *domain.Profile, token string) error {
	return s.db.WithContext(ctx).Model(&profileRecord{}).Where("user_id = ? AND lease_token = ?", p.UserID, token).Updates(map[string]interface{}{
		"conversation_id": p.ConversationID, "conversation_public_id": p.ConversationPublicID, "model": p.Model,
	}).Error
}

func (s *Store) UpdateModel(ctx context.Context, userID uint, token, model string) error {
	return s.db.WithContext(ctx).Model(&profileRecord{}).Where("user_id = ? AND lease_token = ?", userID, token).Update("model", model).Error
}

func (s *Store) SaveGreeting(ctx context.Context, userID uint, token string, greeting domain.GreetingUpdate) error {
	updates := map[string]interface{}{
		"last_greeting": greeting.Text, "greeting_id": greeting.ID, "greeting_at": greeting.At,
		"greeting_after_user_id": greeting.AfterUserID, "last_topic_json": greeting.TopicJSON,
	}
	if greeting.TopicJSON != "" {
		updates["last_topic_at"] = greeting.TopicAt
		updates["seen_topics_json"] = greeting.SeenTopicsJSON
	}
	return s.db.WithContext(ctx).Model(&profileRecord{}).Where("user_id = ? AND lease_token = ?", userID, token).Updates(updates).Error
}

func (s *Store) SetQuiet(ctx context.Context, userID uint, quiet bool) error {
	result := s.db.WithContext(ctx).Model(&profileRecord{}).Where("user_id = ?", userID).Update("quiet", quiet)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkRead(ctx context.Context, userID, messageID uint) error {
	return s.db.WithContext(ctx).Model(&profileRecord{}).Where("user_id = ? AND read_through_id < ?", userID, messageID).Update("read_through_id", messageID).Error
}

func (s *Store) ClaimRefresh(ctx context.Context, userID, revision uint, token string, now time.Time) (bool, error) {
	claim := s.db.WithContext(ctx).Model(&profileRecord{}).Where("user_id = ? AND revision = ? AND refresh_after < ?", userID, revision, now.Unix()).Updates(map[string]interface{}{
		"refresh_token": token, "refresh_after": now.Add(5 * time.Minute).Unix(),
	})
	return claim.RowsAffected == 1, claim.Error
}

func (s *Store) SaveExtraction(ctx context.Context, extraction domain.Extraction) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&profileRecord{}).Where("user_id = ? AND revision = ? AND refresh_token = ?", extraction.UserID, extraction.Revision, extraction.Token).Updates(map[string]interface{}{
			"summary": extraction.Summary, "summary_at": extraction.At, "through_id": extraction.ThroughID, "refresh_token": "",
		})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return nil
		}
		if err := tx.Where("user_id = ? AND expires_at <= ?", extraction.UserID, extraction.At).Delete(&memoryRecord{}).Error; err != nil {
			return err
		}
		for _, memory := range extraction.Memories {
			if err := upsertMemory(tx, memory); err != nil {
				return err
			}
		}
		return trimMemories(tx, extraction.UserID)
	})
}

func upsertMemory(tx *gorm.DB, memory domain.Memory) error {
	record := memoryRecord(memory)
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"value", "evidence", "source_message_id", "source_at", "expires_at", "updated_at",
		}),
	}).Create(&record).Error
}

func (s *Store) TopicCaches(ctx context.Context, since time.Time, limit int) ([]domain.TopicCache, error) {
	var rows []topicCacheRecord
	if err := s.db.WithContext(ctx).Where("fetched_at >= ?", since).Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	caches := make([]domain.TopicCache, len(rows))
	for i, row := range rows {
		caches[i] = domain.TopicCache(row)
	}
	return caches, nil
}

func (s *Store) ClaimTopics(ctx context.Context, category, token string, now time.Time) (bool, error) {
	row := topicCacheRecord{Category: category}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return false, err
	}
	claim := s.db.WithContext(ctx).Model(&topicCacheRecord{}).Where("category = ? AND refresh_after <= ?", category, now.Unix()).Updates(map[string]interface{}{
		"refresh_token": token, "refresh_after": now.Add(5 * time.Minute).Unix(),
	})
	return claim.RowsAffected == 1, claim.Error
}

func (s *Store) SaveTopics(ctx context.Context, category, token, topicsJSON string, now time.Time) error {
	return s.db.WithContext(ctx).Model(&topicCacheRecord{}).Where("category = ? AND refresh_token = ?", category, token).Updates(map[string]interface{}{
		"topics_json": topicsJSON, "fetched_at": now, "refresh_after": now.Add(time.Hour).Unix(), "refresh_token": "",
	}).Error
}

func (s *Store) SaveTopicFeedback(ctx context.Context, userID uint, token string, memory domain.Memory) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := upsertMemory(tx, memory); err != nil {
			return err
		}
		if err := tx.Model(&profileRecord{}).Where("user_id = ? AND lease_token = ?", userID, token).Updates(map[string]interface{}{
			"revision": gorm.Expr("revision + 1"), "refresh_token": "",
		}).Error; err != nil {
			return err
		}
		return trimMemories(tx, userID)
	})
}
