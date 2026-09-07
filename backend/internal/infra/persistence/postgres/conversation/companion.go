package conversation

import (
	"context"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	models "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
)

// ListCompanionMemoryMessages is an additive, bounded forward cursor. Unlike a
// recent-message window it never skips unprocessed turns during a busy chat.
func (r *Repo) ListCompanionMemoryMessages(ctx context.Context, userID, conversationID, afterID uint, limit int) ([]domain.Message, error) {
	if limit < 1 || limit > 32 {
		limit = 32
	}
	items := []models.Message{}
	err := r.db.WithContext(ctx).Where("user_id = ? AND conversation_id = ? AND id > ? AND status = ?", userID, conversationID, afterID, "success").
		Where("role IN ?", []string{"user", "assistant"}).Order("id ASC").Limit(limit).Find(&items).Error
	if err != nil {
		return nil, translateError(err)
	}
	return toMessageDomains(items), nil
}
