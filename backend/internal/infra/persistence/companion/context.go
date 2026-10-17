package companion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	todo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	models "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) ContextPreferences(ctx context.Context, user uint) (domain.ContextPreferences, error) {
	row := contextPreferencesRecord{UserID: user, Enabled: true, ExcludedJSON: "[]"}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return domain.ContextPreferences{}, err
	}
	if err := s.db.WithContext(ctx).First(&row, "user_id = ?", user).Error; err != nil {
		return domain.ContextPreferences{}, err
	}
	p := domain.ContextPreferences{Enabled: row.Enabled, Excluded: []string{}, ForgetBefore: row.ForgetBefore}
	if err := json.Unmarshal([]byte(row.ExcludedJSON), &p.Excluded); err != nil {
		return p, err
	}
	if p.Excluded == nil {
		p.Excluded = []string{}
	}
	return p, nil
}

func (s *Store) SetContextPreferences(ctx context.Context, user uint, p domain.ContextPreferences) error {
	if p.Excluded == nil {
		p.Excluded = []string{}
	}
	if len(p.Excluded) > 500 {
		return ErrInvalid
	}
	profile, err := s.Profile(ctx, user)
	if err != nil {
		return err
	}
	for _, id := range p.Excluded {
		if id == profile.ConversationPublicID {
			return ErrInvalid
		}
	}
	previous, err := s.ContextPreferences(ctx, user)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, id := range previous.Excluded {
		known[id] = true
	}
	newIDs := []string{}
	seen := map[string]bool{}
	for _, id := range p.Excluded {
		if id == "" || len(id) > 64 || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
		if !known[id] {
			newIDs = append(newIDs, id)
		}
	}
	var count int64
	if len(newIDs) > 0 {
		if err := s.db.WithContext(ctx).Model(&models.Conversation{}).Where("user_id = ? AND public_id IN ?", user, newIDs).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(newIDs)) {
			return ErrInvalid
		}
	}
	encoded, _ := json.Marshal(p.Excluded)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Invalidate in-flight inference and old material that may contain excluded content.
		if err := tx.Model(&profileRecord{}).Where("user_id = ?", user).Updates(map[string]interface{}{"revision": gorm.Expr("revision + 1"), "summary": "", "refresh_token": "", "last_greeting": "", "greeting_id": ""}).Error; err != nil {
			return err
		}
		if err := tx.Model(&contextPreferencesRecord{}).Where("user_id = ?", user).Updates(map[string]interface{}{"enabled": p.Enabled, "excluded_json": string(encoded), "refresh_after": time.Time{}, "success_digest": ""}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", user).Delete(&initiativeRecord{}).Error; err != nil {
			return err
		}
		q := tx.Model(&followupRecord{}).Where("user_id = ? AND source_kind IN ?", user, []string{"conversation", "todo"})
		if p.Enabled {
			if len(p.Excluded) == 0 {
				return nil
			}
			q = q.Where("source_conversation_id IN ?", p.Excluded)
		} else {
			q = q.Where("source_kind = 'todo' OR source_conversation_id <> ?", profile.ConversationPublicID)
		}
		return q.Updates(map[string]interface{}{"status": "canceled", "revision": gorm.Expr("revision + 1"), "updated_at": time.Now()}).Error
	})
}

func (s *Store) ContextConversations(ctx context.Context, user uint, query string, page int) ([]domain.ContextConversation, error) {
	p, err := s.ContextPreferences(ctx, user)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		return nil, ErrInvalid
	}
	q := s.db.WithContext(ctx).Model(&models.Conversation{}).Where("user_id = ?", user)
	profile, err := s.Profile(ctx, user)
	if err != nil {
		return nil, err
	}
	q = q.Where("id <> ?", profile.ConversationID)
	if strings.TrimSpace(query) != "" {
		q = q.Where("LOWER(title) LIKE ?", "%"+escapeLike(strings.ToLower(query))+"%")
	}
	var rows []models.Conversation
	if err := q.Order("updated_at DESC, id DESC").Offset((page - 1) * 50).Limit(50).Find(&rows).Error; err != nil {
		return nil, err
	}
	excluded := map[string]bool{}
	for _, id := range p.Excluded {
		excluded[id] = true
	}
	items := make([]domain.ContextConversation, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.ContextConversation{ID: row.PublicID, Title: row.Title, Excluded: excluded[row.PublicID]})
	}
	return items, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer("%", "", "_", "", "\\", "").Replace(s)
}

func bounded(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

func (s *Store) ContextSources(ctx context.Context, user uint, query string) ([]domain.ContextSource, error) {
	p, err := s.ContextPreferences(ctx, user)
	if err != nil {
		return nil, err
	}
	profile, err := s.Profile(ctx, user)
	if err != nil {
		return nil, err
	}
	items := []domain.ContextSource{}
	if s.db.Migrator().HasTable(&models.Conversation{}) && s.db.Migrator().HasTable(&models.Message{}) {
		q := s.db.WithContext(ctx).Model(&models.Conversation{}).Where("user_id = ?", user)
		if !p.Enabled {
			q = q.Where("id = ?", profile.ConversationID)
		}
		if len(p.Excluded) > 0 {
			q = q.Where("public_id NOT IN ?", p.Excluded)
		}
		// Locate candidate conversations across the account before loading bounded active branches.
		terms := contextTerms(query)
		if len(terms) > 0 {
			parts, args := []string{}, []interface{}{}
			for _, term := range terms {
				parts = append(parts, "LOWER(title) LIKE ? OR id IN (SELECT conversation_id FROM chat_messages WHERE user_id = ? AND deleted_at IS NULL AND role = 'user' AND status = 'success' AND LOWER(content) LIKE ?)")
				args = append(args, "%"+term+"%", user, "%"+term+"%")
			}
			q = q.Where("("+strings.Join(parts, " OR ")+")", args...)
		}
		var conversations []models.Conversation
		if err := q.Order("updated_at DESC, id DESC").Limit(24).Find(&conversations).Error; err != nil {
			return nil, err
		}
		for _, conv := range conversations {
			var messages []models.Message
			args := []interface{}{user, conv.ID, user, conv.ID, user, conv.ID}
			depth := 127
			if len(terms) > 0 {
				depth = 2047
			}
			args = append(args, depth, p.ForgetBefore)
			filter := ""
			if len(terms) > 0 && !matchesTerms(conv.Title, terms) {
				parts := []string{}
				for _, term := range terms {
					parts = append(parts, "LOWER(content) LIKE ?")
					args = append(args, "%"+term+"%")
				}
				filter = " AND (" + strings.Join(parts, " OR ") + ")"
			}
			err := s.db.WithContext(ctx).Raw(`WITH RECURSIVE active AS (
			 SELECT id, parent_message_id, content, created_at, role, status, 0 AS depth FROM chat_messages
			 WHERE user_id = ? AND conversation_id = ? AND deleted_at IS NULL AND id = (SELECT MAX(id) FROM chat_messages WHERE user_id = ? AND conversation_id = ? AND deleted_at IS NULL)
			 UNION ALL SELECT m.id, m.parent_message_id, m.content, m.created_at, m.role, m.status, a.depth + 1
			 FROM chat_messages m JOIN active a ON m.id = a.parent_message_id
			 WHERE m.user_id = ? AND m.conversation_id = ? AND m.deleted_at IS NULL AND a.depth < ?)
			 SELECT id, content, created_at FROM active WHERE role = 'user' AND status = 'success' AND created_at > ?`+filter+` ORDER BY id DESC LIMIT 8`, args...).Scan(&messages).Error
			if err != nil {
				return nil, err
			}
			for _, m := range messages {
				if conv.ID == profile.ConversationID && m.ID <= profile.ForgetThroughID {
					continue
				}
				if len(terms) > 0 && !matchesTerms(conv.Title+" "+m.Content, terms) {
					continue
				}
				items = append(items, domain.ContextSource{ID: fmt.Sprintf("message:%d", m.ID), Kind: "conversation", Title: conv.Title, Text: bounded(m.Content, 800), ConversationID: conv.PublicID, MessageID: m.ID, At: m.CreatedAt})
				if len(items) >= 24 {
					break
				}
			}
			if len(items) >= 24 {
				break
			}
		}
	}
	if p.Enabled {
		tasks, err := s.todoTasks(ctx, user)
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			if task.DeletedAt != nil || !task.UpdatedAt.After(p.ForgetBefore) {
				continue
			}
			if query != "" && !matchesTerms(task.Title+" "+task.Notes, contextTerms(query)) {
				continue
			}
			state := "待完成"
			if task.CompletedAt != nil {
				state = "已完成"
			}
			items = append(items, domain.ContextSource{ID: "todo:" + task.ID, Kind: "todo", Title: task.Title, Text: bounded(task.Title+"；"+task.Notes+"；状态："+state+"；截止："+task.DueDate+" "+task.DueTime+" "+task.Timezone, 800), At: task.UpdatedAt})
			if len(items) >= 32 {
				break
			}
		}
	}
	return items, nil
}

func contextTerms(query string) []string {
	query = strings.ToLower(bounded(query, 100))
	terms := []string{}
	for _, word := range strings.Fields(query) {
		word = escapeLike(word)
		if len([]rune(word)) >= 2 {
			terms = append(terms, word)
		}
		if len(terms) >= 6 {
			break
		}
	}
	// Chinese queries have no spaces. Use a few meaningful contiguous fragments as a fallback.
	if len(terms) == 1 && len([]rune(query)) > 6 {
		r := []rune(query)
		terms = nil
		for i := 0; i+2 <= len(r) && len(terms) < 8; i += 2 {
			part := escapeLike(string(r[i : i+2]))
			if len([]rune(part)) == 2 {
				terms = append(terms, part)
			}
		}
	}
	return terms
}
func matchesTerms(text string, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	text = strings.ToLower(text)
	for _, t := range terms {
		if strings.Contains(text, t) {
			return true
		}
	}
	return false
}

func (s *Store) binding(ctx context.Context, user uint) (*models.WeChatMiniAppBinding, error) {
	if s.appID == nil || s.appID() == "" || !s.db.Migrator().HasTable(&models.WeChatMiniAppBinding{}) {
		return nil, ErrNotFound
	}
	var row models.WeChatMiniAppBinding
	err := s.db.WithContext(ctx).Where("user_id = ? AND app_id = ? AND revoked_at IS NULL", user, s.appID()).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &row, err
}

func (s *Store) todoTasks(ctx context.Context, user uint) ([]todo.Task, error) {
	if !s.db.Migrator().HasTable("miniapp_todo_tasks") {
		return nil, nil
	}
	b, err := s.binding(ctx, user)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []struct{ Data string }
	if err := s.db.WithContext(ctx).Table("miniapp_todo_tasks").Select("data").Where("app_id = ? AND owner_open_id = ? AND deleted_at IS NULL", b.AppID, b.OpenID).Order("updated_at DESC").Limit(todo.SnapshotLimit).Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]todo.Task, 0, len(rows))
	for _, row := range rows {
		var task todo.Task
		if err := json.Unmarshal([]byte(row.Data), &task); err != nil {
			return nil, err
		}
		items = append(items, task)
	}
	return items, nil
}
