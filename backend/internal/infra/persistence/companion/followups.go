package companion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	todo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	models "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Store) Followups(ctx context.Context, user uint) ([]domain.Followup, error) {
	var rows []followupRecord
	err := s.db.WithContext(ctx).Where("user_id = ?", user).Order("CASE WHEN status = 'pending' THEN 0 ELSE 1 END, due_at ASC, id ASC").Limit(200).Find(&rows).Error
	items := make([]domain.Followup, 0, len(rows))
	for _, r := range rows {
		items = append(items, domain.Followup(r))
	}
	return items, err
}

// Context must not resurrect deleted or excluded conversations through a saved reminder.
func (s *Store) ContextFollowups(ctx context.Context, user uint) ([]domain.Followup, error) {
	items, err := s.Followups(ctx, user)
	if err != nil {
		return nil, err
	}
	result := []domain.Followup{}
	for _, f := range items {
		if f.SourceKind == "todo" {
			continue
		}
		available, err := s.sourceAvailable(ctx, f)
		if err != nil {
			return nil, err
		}
		if available {
			result = append(result, f)
		}
	}
	return result, nil
}

func (s *Store) SaveFollowup(ctx context.Context, f domain.Followup) (domain.Followup, error) {
	if f.UserID == 0 || strings.TrimSpace(f.Title) == "" || len([]rune(f.Title)) > 120 || (f.Status != "pending" && f.Status != "completed" && f.Status != "canceled") || f.DueAt.IsZero() {
		return f, ErrInvalid
	}
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lock := tx.Model(&profileRecord{}).Where("user_id = ?", f.UserID).Update("user_id", f.UserID)
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return ErrNotFound
		}
		local := &Store{db: tx, appID: s.appID}
		if f.Status == "pending" && f.SourceKind != "manual" {
			available, err := local.sourceAvailable(ctx, f)
			if err != nil {
				return err
			}
			if !available {
				return ErrBusy
			}
		}
		var old followupRecord
		err := tx.Where("user_id = ? AND id = ?", f.UserID, f.ID).First(&old).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if f.Revision != 0 {
				return ErrBusy
			}
			var count int64
			if err := tx.Model(&followupRecord{}).Where("user_id = ? AND status = 'pending'", f.UserID).Count(&count).Error; err != nil {
				return err
			}
			if count >= 100 {
				return ErrBusy
			}
			f.Revision = 1
			f.UpdatedAt = time.Now()
			return tx.Create((*followupRecord)(&f)).Error
		}
		if err != nil {
			return err
		}
		if old.Revision != f.Revision {
			return ErrBusy
		}
		f.Revision++
		f.UpdatedAt = time.Now()
		// Only a new schedule rearms a delivered event. Renaming is not another reminder.
		f.DeliveredAt = old.DeliveredAt
		f.NotificationStatus = old.NotificationStatus
		if !f.DueAt.Equal(old.DueAt) || old.Status != "pending" && f.Status == "pending" {
			f.DeliveredAt = time.Time{}
			f.NotificationStatus = ""
		}
		result := tx.Model(&followupRecord{}).Where("id = ? AND user_id = ? AND revision = ?", f.ID, f.UserID, old.Revision).Select("*").Updates(followupRecord(f))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrBusy
		}
		return nil
	})
	return f, err
}

func todoDue(t todo.Task) (time.Time, error) {
	if t.DueDate == "" {
		return time.Time{}, ErrInvalid
	}
	zone := t.Timezone
	if zone == "" {
		zone = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, err
	}
	clock := t.DueTime
	if clock == "" {
		clock = "09:00"
	}
	return time.ParseInLocation("2006-01-02 15:04", t.DueDate+" "+clock, loc)
}

func (s *Store) SyncTodoFollowups(ctx context.Context, user uint, now time.Time) error {
	p, err := s.ContextPreferences(ctx, user)
	if err != nil {
		return err
	}
	if !p.Enabled {
		return nil
	}
	tasks, err := s.todoTasks(ctx, user)
	if err != nil {
		return err
	}
	ownerKey := ""
	if binding, err := s.binding(ctx, user); err == nil {
		ownerKey = (todo.Owner{AppID: binding.AppID, OpenID: binding.OpenID}).Key()
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	var rows []followupRecord
	if err := s.db.WithContext(ctx).Where("user_id = ? AND source_kind = 'todo'", user).Find(&rows).Error; err != nil {
		return err
	}
	bySource := map[string]domain.Followup{}
	for _, f := range rows {
		if f.SourceOwnerKey != ownerKey {
			if f.Status == "pending" {
				f.Status = "canceled"
				if _, err := s.SaveFollowup(ctx, domain.Followup(f)); err != nil && !errors.Is(err, ErrBusy) {
					return err
				}
			}
			continue
		}
		bySource[f.SourceID] = domain.Followup(f)
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		key := "todo:" + task.ID
		seen[key] = true
		old, ok := bySource[key]
		due, parseErr := todoDue(task)
		status := "pending"
		if task.CompletedAt != nil {
			status = "completed"
		}
		if task.DeletedAt != nil || parseErr != nil {
			status = "canceled"
		}
		if ok && old.UserManaged {
			if old.Status != "pending" || status == "pending" {
				continue
			}
			old.Status = status
			if _, err := s.SaveFollowup(ctx, old); err != nil && !errors.Is(err, ErrBusy) {
				return err
			}
			continue
		}
		if !ok && (status != "pending" || !task.UpdatedAt.After(p.ForgetBefore) || due.Before(now.Add(-24*time.Hour))) {
			continue
		}
		if ok && old.Title == task.Title && old.DueAt.Equal(due) && old.Status == status {
			continue
		}
		f := old
		if !ok {
			f = domain.Followup{ID: "todo-" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%d:%s:%s", user, ownerKey, task.ID))).String(), UserID: user, SourceKind: "todo", SourceID: key, SourceOwnerKey: ownerKey}
		}
		f.Title = bounded(task.Title, 120)
		if parseErr == nil {
			f.DueAt = due
		}
		f.Status = status
		f.SourceAt = task.UpdatedAt
		f.Evidence = bounded(task.Title, 300)
		if _, err := s.SaveFollowup(ctx, f); err != nil && !errors.Is(err, ErrBusy) {
			return err
		}
	}
	for key, f := range bySource {
		if !seen[key] && f.Status == "pending" {
			f.Status = "canceled"
			if _, err := s.SaveFollowup(ctx, f); err != nil && !errors.Is(err, ErrBusy) {
				return err
			}
		}
	}
	return nil
}

func (s *Store) RecordDecision(ctx context.Context, user uint, d domain.Decision) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "kind"}}, DoUpdates: clause.AssignmentColumns([]string{"reason", "at"})}).Create(&decisionRecord{UserID: user, Kind: d.Kind, Reason: d.Reason, At: d.At}).Error
}
func (s *Store) Decisions(ctx context.Context, user uint) ([]domain.Decision, error) {
	var rows []decisionRecord
	err := s.db.WithContext(ctx).Where("user_id = ?", user).Order("kind ASC").Find(&rows).Error
	items := []domain.Decision{}
	for _, r := range rows {
		items = append(items, domain.Decision{Kind: r.Kind, Reason: r.Reason, At: r.At})
	}
	return items, err
}

func (s *Store) NotificationSettings(ctx context.Context, user uint) (domain.NotificationSettings, error) {
	var row notificationRecord
	err := s.db.WithContext(ctx).First(&row, "user_id = ?", user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.NotificationSettings{}, nil
	}
	if err != nil {
		return domain.NotificationSettings{}, err
	}
	if row.Enabled {
		binding, err := s.binding(ctx, user)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return domain.NotificationSettings{}, err
		}
		if binding == nil || binding.AppID != row.BoundAppID || binding.OpenID != row.BoundOpenID {
			row.Enabled = false
			row.Credits = 0
		}
	}
	return domain.NotificationSettings{Enabled: row.Enabled, Credits: row.Credits, TemplateID: row.TemplateID}, err
}
func (s *Store) SetNotificationSettings(ctx context.Context, user uint, n domain.NotificationSettings, grant bool) error {
	var binding *models.WeChatMiniAppBinding
	if n.Enabled {
		var err error
		if binding, err = s.binding(ctx, user); err != nil {
			return err
		}
		if n.AppID != "" && n.AppID != binding.AppID {
			return ErrBusy
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&notificationRecord{UserID: user}).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{"enabled": n.Enabled, "template_id": n.TemplateID}
		if !n.Enabled {
			updates["credits"] = 0
			updates["bound_app_id"], updates["bound_open_id"] = "", ""
		}
		if grant && n.Enabled {
			// One accepted subscription request grants at most one send. Do not accumulate
			// unverifiable client counts; the platform remains the final consent authority.
			updates["credits"] = 1
			updates["last_consent_at"] = time.Now()
			updates["bound_app_id"], updates["bound_open_id"] = binding.AppID, binding.OpenID
		}
		return tx.Model(&notificationRecord{}).Where("user_id = ?", user).Updates(updates).Error
	})
}
func (s *Store) AssistantUsers(ctx context.Context, after uint, limit int) ([]uint, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var ids []uint
	q := s.db.WithContext(ctx).Model(&profileRecord{}).Where("user_id > ? AND conversation_id > 0", after)
	if s.db.Migrator().HasTable("identity_users") {
		q = q.Where("EXISTS (SELECT 1 FROM identity_users WHERE identity_users.id = companion_profiles.user_id AND identity_users.deleted_at IS NULL AND identity_users.status = 'active')")
	}
	err := q.Order("user_id ASC").Limit(limit).Pluck("user_id", &ids).Error
	return ids, err
}
func (s *Store) ClaimFollowupRefresh(ctx context.Context, user uint, now time.Time, digest string) (bool, error) {
	if _, err := s.ContextPreferences(ctx, user); err != nil {
		return false, err
	}
	r := s.db.WithContext(ctx).Model(&contextPreferencesRecord{}).Where("user_id = ? AND refresh_after < ? AND (success_digest IS NULL OR success_digest <> ?)", user, now, digest).Update("refresh_after", now.Add(10*time.Minute))
	return r.RowsAffected == 1, r.Error
}
func (s *Store) CompleteFollowupRefresh(ctx context.Context, user uint, digest string) error {
	return s.db.WithContext(ctx).Model(&contextPreferencesRecord{}).Where("user_id = ?", user).Update("success_digest", digest).Error
}
func (s *Store) FollowupRefreshCurrent(ctx context.Context, user uint, digest string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&contextPreferencesRecord{}).Where("user_id = ? AND success_digest = ?", user, digest).Count(&count).Error
	return count == 1, err
}

func (s *Store) sourceAvailable(ctx context.Context, f domain.Followup) (bool, error) {
	if f.SourceKind == "manual" {
		return true, nil
	}
	p, err := s.ContextPreferences(ctx, f.UserID)
	if err != nil {
		return false, err
	}
	if !f.SourceAt.After(p.ForgetBefore) {
		return false, nil
	}
	if f.SourceKind == "todo" {
		if !p.Enabled {
			return false, nil
		}
		binding, err := s.binding(ctx, f.UserID)
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if f.SourceOwnerKey != (todo.Owner{AppID: binding.AppID, OpenID: binding.OpenID}).Key() {
			return false, nil
		}
		tasks, err := s.todoTasks(ctx, f.UserID)
		if err != nil {
			return false, err
		}
		for _, t := range tasks {
			if "todo:"+t.ID == f.SourceID && t.DeletedAt == nil && t.CompletedAt == nil {
				d, e := todoDue(t)
				return e == nil && (f.UserManaged || d.Equal(f.DueAt)), nil
			}
		}
		return false, nil
	}
	profile, err := s.Profile(ctx, f.UserID)
	if err != nil {
		return false, err
	}
	if !p.Enabled && f.SourceConversationID != profile.ConversationPublicID {
		return false, nil
	}
	for _, id := range p.Excluded {
		if id == f.SourceConversationID {
			return false, nil
		}
	}
	if f.SourceConversationID == profile.ConversationPublicID && f.SourceMessageID <= profile.ForgetThroughID {
		return false, nil
	}
	var count int64
	err = s.db.WithContext(ctx).Raw(`WITH RECURSIVE active AS (
	 SELECT m.id,m.parent_message_id,m.conversation_id,m.role,m.status FROM chat_messages m
 JOIN chat_conversations c ON c.id=m.conversation_id AND c.user_id=? AND c.public_id=? AND c.deleted_at IS NULL
 WHERE m.user_id=? AND m.deleted_at IS NULL AND m.id=(SELECT MAX(id) FROM chat_messages WHERE user_id=? AND conversation_id=c.id AND deleted_at IS NULL)
	 UNION SELECT m.id,m.parent_message_id,m.conversation_id,m.role,m.status FROM chat_messages m JOIN active a ON m.id=a.parent_message_id
	 WHERE m.user_id=? AND m.conversation_id=a.conversation_id AND m.deleted_at IS NULL AND a.id > ?)
	 SELECT COUNT(*) FROM active WHERE id=? AND role='user' AND status='success'`, f.UserID, f.SourceConversationID, f.UserID, f.UserID, f.UserID, f.SourceMessageID, f.SourceMessageID).Scan(&count).Error
	return count == 1, err
}

func (s *Store) PublishDueFollowup(ctx context.Context, user uint, now time.Time, template string) (*domain.NotificationDelivery, error) {
	token, err := s.Acquire(ctx, user)
	if errors.Is(err, ErrBusy) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer s.Release(user, token)
	var delivery *domain.NotificationDelivery
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lock := tx.Model(&profileRecord{}).Where("user_id = ? AND lease_token = ?", user, token).Update("lease_token", token)
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return ErrBusy
		}
		local := &Store{db: tx, appID: s.appID}
		p, err := local.Profile(ctx, user)
		if err != nil {
			return err
		}
		if p.Quiet {
			return local.RecordDecision(ctx, user, domain.Decision{Kind: "followup", Reason: "off", At: now})
		}
		if tx.Migrator().HasTable(&models.Conversation{}) {
			var count int64
			if err := tx.Model(&models.Conversation{}).Where("id = ? AND user_id = ?", p.ConversationID, user).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return ErrNotFound
			}
		}
		if tx.Migrator().HasTable("identity_users") {
			var n int64
			if err := tx.Table("identity_users").Where("id = ? AND deleted_at IS NULL AND status = 'active'", user).Count(&n).Error; err != nil {
				return err
			}
			if n == 0 {
				return ErrNotFound
			}
		}
		// Never retry an ambiguous send after a process crash.
		if err := tx.Model(&followupRecord{}).Where("user_id = ? AND notification_status = 'dispatching' AND delivered_at < ?", user, now.Add(-time.Minute)).Update("notification_status", "uncertain").Error; err != nil {
			return err
		}
		sources, err := local.ContextSources(ctx, user, "")
		if err != nil {
			return err
		}
		fresh, err := local.FollowupRefreshCurrent(ctx, user, domain.ContextDigest(sources))
		if err != nil {
			return err
		}
		var f followupRecord
		q := tx.Where("user_id = ? AND status = 'pending' AND due_at <= ? AND delivered_at = ?", user, now, time.Time{})
		if !fresh {
			q = q.Where("source_kind <> 'conversation' OR user_managed = ?", true)
		}
		err = q.Order("due_at ASC, id ASC").First(&f).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		available, err := local.sourceAvailable(ctx, domain.Followup(f))
		if err != nil {
			return err
		}
		if !available {
			return tx.Model(&f).Update("status", "canceled").Error
		}
		if f.DueAt.Before(now.Add(-24 * time.Hour)) {
			return tx.Model(&f).Updates(map[string]interface{}{"delivered_at": now, "notification_status": "expired"}).Error
		}
		id := fmt.Sprintf("followup:%s:%d", f.ID, f.Revision)
		// Due follow-ups use the same persisted timeline. No browser acceptance is needed.
		var boundary uint
		if tx.Migrator().HasTable(&models.Message{}) {
			if err := tx.Model(&models.Message{}).Where("user_id = ? AND conversation_id = ?", user, p.ConversationID).Select("COALESCE(MAX(id), 0)").Scan(&boundary).Error; err != nil {
				return err
			}
		}
		message := initiativeRecord{ID: "followup:" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(id)).String(), UserID: user, ConversationID: p.ConversationID, Kind: "followup", AfterMessageID: boundary, Revision: p.Revision, Text: "你之前提到的「" + bounded(f.Title, 60) + "」到了约定时间。需要一起看看进展吗？", CreatedAt: now, AcceptedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&message).Error; err != nil {
			return err
		}
		state := "not_authorized"
		if template == "" {
			state = "not_configured"
		}
		n, err := local.NotificationSettings(ctx, user)
		if err != nil {
			return err
		}
		if template != "" && n.Enabled && n.Credits > 0 && n.TemplateID == template {
			b, err := local.binding(ctx, user)
			if err == nil {
				r := tx.Model(&notificationRecord{}).Where("user_id = ? AND enabled = ? AND credits > 0 AND template_id = ? AND bound_app_id = ? AND bound_open_id = ?", user, true, template, b.AppID, b.OpenID).Update("credits", gorm.Expr("credits - 1"))
				if r.Error != nil {
					return r.Error
				}
				if r.RowsAffected == 1 {
					state = "dispatching"
					delivery = &domain.NotificationDelivery{ID: id, UserID: user, AppID: b.AppID, OpenID: b.OpenID, TemplateID: template, Title: f.Title, DueAt: f.DueAt}
				}
			}
		}
		r := tx.Model(&followupRecord{}).Where("id = ? AND revision = ? AND delivered_at = ?", f.ID, f.Revision, time.Time{}).Updates(map[string]interface{}{"delivered_at": now, "notification_status": state})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrBusy
		}
		var obsolete []string
		if err := tx.Model(&initiativeRecord{}).Where("user_id = ? AND accepted_at > ?", user, time.Time{}).Order("accepted_at DESC, id DESC").Offset(100).Pluck("id", &obsolete).Error; err != nil {
			return err
		}
		if len(obsolete) > 0 {
			if err := tx.Where("user_id = ? AND id IN ?", user, obsolete).Delete(&initiativeRecord{}).Error; err != nil {
				return err
			}
		}
		return local.RecordDecision(ctx, user, domain.Decision{Kind: "followup", Reason: "delivered", At: now})
	})
	if err != nil {
		return nil, err
	}
	return delivery, nil
}
func (s *Store) FinishNotification(ctx context.Context, id, status string) error {
	parts := strings.Split(id, ":")
	if len(parts) != 3 || parts[0] != "followup" {
		return ErrInvalid
	}
	return s.db.WithContext(ctx).Model(&followupRecord{}).Where("id = ? AND revision = ? AND notification_status = 'dispatching'", parts[1], parts[2]).Update("notification_status", status).Error
}
