package companion

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type reminderOwner struct {
	UserID   uint `gorm:"primaryKey;autoIncrement:false"`
	Revision uint
}

func (reminderOwner) TableName() string { return "companion_reminder_owners" }

type reminderRecord struct {
	ID     string    `gorm:"primaryKey;size:64"`
	UserID uint      `gorm:"index"`
	Status string    `gorm:"size:20;index:companion_reminder_cursor,priority:1"`
	Cursor time.Time `gorm:"index:companion_reminder_cursor,priority:2"`
	Data   string    `gorm:"type:text"`
}

func (reminderRecord) TableName() string { return "companion_reminders" }

type reminderEvent struct {
	ID                 string    `gorm:"primaryKey;size:64"`
	UserID             uint      `gorm:"index"`
	ReminderID         string    `gorm:"size:64;uniqueIndex:companion_reminder_event,priority:1"`
	ScheduledAt        time.Time `gorm:"uniqueIndex:companion_reminder_event,priority:2"`
	ScheduleVersion    uint      `gorm:"uniqueIndex:companion_reminder_event,priority:3"`
	Revision           uint
	DueAt              time.Time `gorm:"index:companion_reminder_event_due,priority:2"`
	Status             string    `gorm:"size:20;index:companion_reminder_event_due,priority:1"`
	NotificationStatus string    `gorm:"size:24;index"`
	DispatchStartedAt  time.Time
	Data               string `gorm:"type:text"`
}

func (reminderEvent) TableName() string { return "companion_reminder_events" }

type reminderTurnRecord struct {
	ID             string    `gorm:"primaryKey;size:100"`
	UserID         uint      `gorm:"primaryKey;autoIncrement:false"`
	ConversationID string    `gorm:"size:64;index"`
	Hash           string    `gorm:"size:64"`
	CreatedAt      time.Time `gorm:"index"`
	Data           string    `gorm:"type:text"`
}

func (reminderTurnRecord) TableName() string { return "companion_reminder_turns" }

func reminderJSON(v interface{}) string { b, _ := json.Marshal(v); return string(b) }
func saveReminder(tx *gorm.DB, p domain.Reminder) error {
	return tx.Save(&reminderRecord{ID: p.ID, UserID: p.UserID, Status: p.Status, Cursor: p.Cursor, Data: reminderJSON(p)}).Error
}
func saveReminderEvent(tx *gorm.DB, e domain.ReminderOccurrence) error {
	return tx.Save(&reminderEvent{ID: e.ID, UserID: e.UserID, ReminderID: e.ReminderID, ScheduledAt: e.ScheduledAt, ScheduleVersion: e.ScheduleVersion, Revision: e.Revision, DueAt: e.DueAt, Status: e.Status, NotificationStatus: e.NotificationStatus, Data: reminderJSON(e)}).Error
}

// An independent owner row serializes mutations and scheduling across instances.
// It never takes the conversation lease or waits for model inference.
func lockReminderOwner(tx *gorm.DB, user uint) error {
	if user == 0 {
		return ErrInvalid
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&reminderOwner{UserID: user}).Error; err != nil {
		return err
	}
	if err := tx.Model(&reminderOwner{}).Where("user_id = ?", user).Update("revision", gorm.Expr("revision + 1")).Error; err != nil {
		return err
	}
	var n int64
	if err := tx.Model(&profileRecord{}).Where("user_id = ?", user).Count(&n).Error; err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	if tx.Migrator().HasTable("identity_users") {
		if err := tx.Table("identity_users").Where("id = ? AND deleted_at IS NULL AND status <> 'deactivated'", user).Count(&n).Error; err != nil {
			return err
		}
		if n != 1 {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Store) ReminderSnapshot(ctx context.Context, user uint, conversation string, now time.Time) (domain.ReminderSnapshot, error) {
	state := domain.ReminderSnapshot{Plans: []domain.Reminder{}, Occurrences: []domain.ReminderOccurrence{}, Turns: []domain.ReminderTurn{}, ServerTime: now}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var plans []reminderRecord
		if err := tx.Where("user_id = ? AND status <> 'deleted'", user).Order("CASE WHEN status = 'active' THEN 0 WHEN status = 'paused' THEN 1 ELSE 2 END, cursor ASC, id ASC").Limit(200).Find(&plans).Error; err != nil {
			return err
		}
		for _, r := range plans {
			var p domain.Reminder
			if err := json.Unmarshal([]byte(r.Data), &p); err != nil {
				return err
			}
			state.Plans = append(state.Plans, p)
		}
		var events []reminderEvent
		if err := tx.Where("user_id = ? AND status <> 'canceled'", user).Order("CASE WHEN status = 'pending' THEN 0 WHEN status = 'due' THEN 1 ELSE 2 END, due_at DESC").Limit(200).Find(&events).Error; err != nil {
			return err
		}
		for _, r := range events {
			var e domain.ReminderOccurrence
			if err := json.Unmarshal([]byte(r.Data), &e); err != nil {
				return err
			}
			e.NotificationStatus = r.NotificationStatus
			state.Occurrences = append(state.Occurrences, e)
		}
		for i := range state.Plans {
			p := &state.Plans[i]
			p.NextAt = p.Cursor
			if p.Status != "active" {
				p.NextAt = time.Time{}
				continue
			}
			for _, e := range state.Occurrences {
				if e.ReminderID == p.ID && (e.Status == "pending" || e.Status == "due") && (p.NextAt.IsZero() || e.DueAt.Before(p.NextAt)) {
					p.NextAt = e.DueAt
				}
			}
		}
		var turns []reminderTurnRecord
		q := tx.Where("user_id = ?", user)
		if conversation != "" {
			q = q.Where("conversation_id = ?", conversation)
		}
		if err := q.Order("created_at DESC, id DESC").Limit(100).Find(&turns).Error; err != nil {
			return err
		}
		for i := len(turns) - 1; i >= 0; i-- {
			var t domain.ReminderTurn
			if err := json.Unmarshal([]byte(turns[i].Data), &t); err != nil {
				return err
			}
			state.Turns = append(state.Turns, t)
		}
		return nil
	})
	return state, err
}

func loadReminder(tx *gorm.DB, user uint, id string) (domain.Reminder, error) {
	var r reminderRecord
	var p domain.Reminder
	err := tx.Where("user_id = ? AND id = ? AND status <> 'deleted'", user, id).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal([]byte(r.Data), &p)
	return p, err
}

// Keep canceled instances for history; old schedules can never be delivered again.
func setReminderEventsState(tx *gorm.DB, p domain.Reminder, status string) error {
	var rows []reminderEvent
	q := tx.Where("user_id = ? AND reminder_id = ? AND status IN ?", p.UserID, p.ID, []string{"pending", "due", "suspended"})
	if err := q.Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		var e domain.ReminderOccurrence
		if err := json.Unmarshal([]byte(r.Data), &e); err != nil {
			return err
		}
		e.Status = status
		e.NotificationStatus = "canceled"
		e.Revision++
		if err := saveReminderEvent(tx, e); err != nil {
			return err
		}
	}
	return nil
}

func materializeReminder(tx *gorm.DB, p *domain.Reminder) (domain.ReminderOccurrence, error) {
	e := domain.ReminderOccurrence{ID: uuid.NewString(), ReminderID: p.ID, UserID: p.UserID, Title: p.Title, Status: "pending", ScheduledAt: p.Cursor, DueAt: p.Cursor, Revision: 1, ScheduleVersion: p.ScheduleVersion}
	if p.Cursor.IsZero() {
		return e, ErrInvalid
	}
	if err := saveReminderEvent(tx, e); err != nil {
		return e, err
	}
	p.Generated++
	p.Cursor = p.Schedule.Next(p.Cursor, p.Generated)
	return e, nil
}

func (s *Store) ExecuteReminder(ctx context.Context, user uint, c domain.ReminderCommand, now time.Time) (domain.ReminderResult, error) {
	out := domain.ReminderResult{Handled: true}
	if len(c.RequestID) < 8 || len(c.RequestID) > 100 || len([]rune(c.Input)) > 8000 {
		return out, ErrInvalid
	}
	raw := reminderJSON(c)
	if c.Input != "" {
		raw = c.ConversationID + "\n" + c.Input
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockReminderOwner(tx, user); err != nil {
			return err
		}
		var prior reminderTurnRecord
		err := tx.Where("user_id = ? AND id = ?", user, c.RequestID).First(&prior).Error
		if err == nil {
			if prior.Hash != hash {
				return ErrInvalid
			}
			return json.Unmarshal([]byte(prior.Data), &out.Turn)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		t := domain.ReminderTurn{ID: c.RequestID, UserID: user, ConversationID: c.ConversationID, Input: c.Input, CreatedAt: now, ReminderIDs: []string{}}
		switch c.Action {
		case "clarify":
			t.Reply = c.Reply
			t.ReminderIDs = c.IDs
		case "list":
			var rows []reminderRecord
			q := tx.Where("user_id = ? AND status <> 'deleted'", user)
			if len(c.IDs) > 0 {
				q = q.Where("id IN ?", c.IDs)
			}
			if err := q.Order("cursor ASC, id ASC").Limit(200).Find(&rows).Error; err != nil {
				return err
			}
			t.Reply = "你还没有提醒。可以说“一个小时后提醒我吃药”。"
			if len(rows) > 0 {
				t.Reply = "你的提醒如下，卡片会显示最新状态和倒计时。"
				for _, r := range rows {
					t.ReminderIDs = append(t.ReminderIDs, r.ID)
				}
			}
		case "create":
			if !validReminderSchedule(c, now) {
				return ErrInvalid
			}
			var n int64
			if err := tx.Model(&reminderRecord{}).Where("user_id = ? AND status IN ?", user, []string{"active", "paused"}).Count(&n).Error; err != nil {
				return err
			}
			if n >= 100 {
				return ErrBusy
			}
			p := domain.Reminder{ID: uuid.NewString(), UserID: user, Title: strings.TrimSpace(c.Title), Status: "active", Schedule: c.Schedule, CreatedAt: now, UpdatedAt: now, Revision: 1}
			p.Cursor = p.Schedule.Next(now, 0)
			if p.Cursor.IsZero() {
				return ErrInvalid
			}
			if err := saveReminder(tx, p); err != nil {
				return err
			}
			t.ReminderIDs = []string{p.ID}
			t.Reply = "已设置「" + p.Title + "」，" + p.Schedule.Description() + "，下次 " + domain.ReminderTime(p.Cursor) + "。"
		case "update", "pause", "resume", "delete", "complete", "skip", "snooze":
			p, err := loadReminder(tx, user, c.ID)
			if err != nil {
				return err
			}
			if c.Revision == 0 || p.Revision != c.Revision {
				return ErrBusy
			}
			t.ReminderIDs = []string{p.ID}
			switch c.Action {
			case "delete", "pause":
				p.Status = "deleted"
				verb := "已删除"
				if c.Action == "pause" {
					p.Status = "paused"
					verb = "已暂停"
				}
				eventStatus := "canceled"
				if c.Action == "pause" {
					eventStatus = "suspended"
				}
				if err := setReminderEventsState(tx, p, eventStatus); err != nil {
					return err
				}
				t.Reply = verb + "「" + p.Title + "」。"
			case "resume":
				if p.Status != "paused" {
					return ErrInvalid
				}
				p.Status = "active"
				if !p.Cursor.IsZero() && !p.Cursor.After(now) {
					p.Cursor = p.Schedule.Next(now, p.Generated)
				}
				var suspended []reminderEvent
				if err := tx.Where("user_id = ? AND reminder_id = ? AND status = 'suspended'", user, p.ID).Find(&suspended).Error; err != nil {
					return err
				}
				next := p.Cursor
				for _, r := range suspended {
					var e domain.ReminderOccurrence
					if err := json.Unmarshal([]byte(r.Data), &e); err != nil {
						return err
					}
					e.Status = "missed"
					e.NotificationStatus = "expired"
					if e.DueAt.After(now) {
						e.Status = "pending"
						e.NotificationStatus = ""
						e.NotifiedAt = time.Time{}
						if next.IsZero() || e.DueAt.Before(next) {
							next = e.DueAt
						}
					} else {
						e.NotifiedAt = now
					}
					e.Revision++
					if err := saveReminderEvent(tx, e); err != nil {
						return err
					}
				}
				if next.IsZero() {
					return ErrInvalid
				}
				t.Reply = "已恢复「" + p.Title + "」，下次 " + domain.ReminderTime(next) + "。"
			case "update":
				if c.Title == "" {
					c.Title = p.Title
				}
				if !validReminderSchedule(c, now) {
					return ErrInvalid
				}
				if err := setReminderEventsState(tx, p, "canceled"); err != nil {
					return err
				}
				p.Title = strings.TrimSpace(c.Title)
				p.Schedule = c.Schedule
				p.Generated = 0
				p.ScheduleVersion++
				p.Cursor = p.Schedule.Next(now, 0)
				p.Status = "active"
				if p.Cursor.IsZero() {
					return ErrInvalid
				}
				t.Reply = "已修改「" + p.Title + "」，" + p.Schedule.Description() + "，下次 " + domain.ReminderTime(p.Cursor) + "。"
			default:
				if p.Status != "active" {
					return ErrInvalid
				}
				var row reminderEvent
				q := tx.Where("user_id = ? AND reminder_id = ? AND status IN ?", user, p.ID, []string{"pending", "due"})
				if c.OccurrenceID != "" {
					q = q.Where("id = ?", c.OccurrenceID)
				}
				err := q.Order("due_at ASC, id ASC").First(&row).Error
				var e domain.ReminderOccurrence
				// A snoozed occurrence can fall after the next calendar time.
				// Untargeted actions must use the same earliest time shown by NextAt.
				calendarFirst := !p.Cursor.IsZero() && (errors.Is(err, gorm.ErrRecordNotFound) || err == nil && p.Cursor.Before(row.DueAt))
				if c.OccurrenceID == "" && calendarFirst {
					e, err = materializeReminder(tx, &p)
				} else if err == nil {
					err = json.Unmarshal([]byte(row.Data), &e)
				}
				if err != nil {
					return err
				}
				if c.Action == "snooze" {
					if c.DelayMinutes < 1 || c.DelayMinutes > 10080 {
						return ErrInvalid
					}
					base := now
					if e.DueAt.After(base) {
						base = e.DueAt
					}
					e.DueAt = base.Add(time.Duration(c.DelayMinutes) * time.Minute)
					e.Status = "pending"
					e.NotificationStatus = ""
					e.NotifiedAt = time.Time{}
					t.Reply = "已将本次「" + p.Title + "」推迟到 " + domain.ReminderTime(e.DueAt) + "。"
				} else {
					e.Status = "completed"
					verb := "已完成"
					if c.Action == "skip" {
						e.Status = "skipped"
						verb = "已跳过"
					}
					e.CompletedAt = now
					e.NotificationStatus = "canceled"
					t.Reply = verb + "本次「" + p.Title + "」。"
					if p.Schedule.Kind != "once" {
						t.Reply += "后续重复提醒保留。"
					}
				}
				e.Revision++
				if err := saveReminderEvent(tx, e); err != nil {
					return err
				}
			}
			p.Revision++
			p.UpdatedAt = now
			if err := endReminderIfFinished(tx, &p); err != nil {
				return err
			}
			if err := saveReminder(tx, p); err != nil {
				return err
			}
		default:
			return ErrInvalid
		}
		if len(t.ReminderIDs) > 200 || len([]rune(t.Reply)) > 2000 {
			return ErrInvalid
		}
		if err := tx.Create(&reminderTurnRecord{ID: t.ID, UserID: user, ConversationID: t.ConversationID, Hash: hash, CreatedAt: now, Data: reminderJSON(t)}).Error; err != nil {
			return err
		}
		out.Turn = t
		return nil
	})
	if err != nil {
		return out, err
	}
	out.Snapshot, err = s.ReminderSnapshot(ctx, user, c.ConversationID, now)
	return out, err
}
func validReminderSchedule(c domain.ReminderCommand, now time.Time) bool {
	return strings.TrimSpace(c.Title) != "" && len([]rune(c.Title)) <= 120 && c.Schedule.Validate() && c.Schedule.Next(now, 0).After(now)
}
func endReminderIfFinished(tx *gorm.DB, p *domain.Reminder) error {
	if p.Status != "active" || !p.Cursor.IsZero() {
		return nil
	}
	var n int64
	if err := tx.Model(&reminderEvent{}).Where("user_id = ? AND reminder_id = ? AND status IN ?", p.UserID, p.ID, []string{"pending", "due"}).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		p.Status = "ended"
	}
	return nil
}

func (s *Store) TickReminders(ctx context.Context, now time.Time, template string) error {
	var users []uint
	q := s.db.WithContext(ctx).Model(&reminderRecord{}).Where("status = 'active' AND ((cursor > ? AND cursor <= ?) OR id IN (SELECT reminder_id FROM companion_reminder_events WHERE status = 'pending' AND due_at <= ?))", time.Time{}, now, now)
	if s.db.Migrator().HasTable("identity_users") {
		q = q.Where("EXISTS (SELECT 1 FROM identity_users WHERE identity_users.id = companion_reminders.user_id AND identity_users.deleted_at IS NULL AND identity_users.status <> 'deactivated')")
	}
	if err := q.Distinct().Order("user_id ASC").Limit(1000).Pluck("user_id", &users).Error; err != nil {
		return err
	}
	var errs []error
	for _, user := range users {
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := lockReminderOwner(tx, user); err != nil {
				return err
			}
			var rows []reminderRecord
			if err := tx.Where("user_id = ? AND status = 'active' AND ((cursor > ? AND cursor <= ?) OR id IN (SELECT reminder_id FROM companion_reminder_events WHERE user_id = ? AND status = 'pending' AND due_at <= ?))", user, time.Time{}, now, user, now).Find(&rows).Error; err != nil {
				return err
			}
			for _, r := range rows {
				var p domain.Reminder
				if err := json.Unmarshal([]byte(r.Data), &p); err != nil {
					return err
				}
				if !p.Cursor.IsZero() && !p.Cursor.After(now) {
					// A restart records one missed instance, then skips the stale backlog.
					if p.Cursor.Before(now.Add(-10 * time.Minute)) {
						e, err := materializeReminder(tx, &p)
						if err != nil {
							return err
						}
						e.Status = "missed"
						e.NotifiedAt = now
						e.NotificationStatus = "expired"
						if err := saveReminderEvent(tx, e); err != nil {
							return err
						}
						p.Cursor = p.Schedule.Next(now.Add(-10*time.Minute), p.Generated)
					}
					for i := 0; i < 11 && !p.Cursor.IsZero() && !p.Cursor.After(now); i++ {
						if _, err := materializeReminder(tx, &p); err != nil {
							return err
						}
					}
				}
				var events []reminderEvent
				if err := tx.Where("user_id = ? AND reminder_id = ? AND status = 'pending' AND due_at <= ?", user, p.ID, now).Find(&events).Error; err != nil {
					return err
				}
				for _, r := range events {
					var e domain.ReminderOccurrence
					if err := json.Unmarshal([]byte(r.Data), &e); err != nil {
						return err
					}
					e.Status = "due"
					e.NotifiedAt = now
					e.NotificationStatus = "queued"
					if template == "" {
						e.NotificationStatus = "not_configured"
					}
					if e.DueAt.Before(now.Add(-10 * time.Minute)) {
						e.Status = "missed"
						e.NotificationStatus = "expired"
					}
					if err := saveReminderEvent(tx, e); err != nil {
						return err
					}
				}
				if err := endReminderIfFinished(tx, &p); err != nil {
					return err
				}
				if err := saveReminder(tx, p); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil && !errors.Is(err, ErrNotFound) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *Store) ClaimReminderNotification(ctx context.Context, now time.Time, template string) (*domain.NotificationDelivery, bool, error) {
	if template == "" {
		return nil, false, nil
	}
	// Sending may have reached WeChat before a crash. Never blindly resend it.
	if err := s.db.WithContext(ctx).Model(&reminderEvent{}).Where("notification_status = 'dispatching' AND dispatch_started_at < ?", now.Add(-time.Minute)).Update("notification_status", "uncertain").Error; err != nil {
		return nil, false, err
	}
	var candidates []reminderEvent
	q := s.db.WithContext(ctx).Where("notification_status = 'queued'")
	if s.db.Migrator().HasTable("identity_users") {
		q = q.Where("EXISTS (SELECT 1 FROM identity_users WHERE identity_users.id = companion_reminder_events.user_id AND identity_users.deleted_at IS NULL AND identity_users.status <> 'deactivated')")
	}
	if err := q.Order("due_at ASC, id ASC").Limit(16).Find(&candidates).Error; err != nil {
		return nil, false, err
	}
	for _, candidate := range candidates {
		var delivery *domain.NotificationDelivery
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := lockReminderOwner(tx, candidate.UserID); err != nil {
				return err
			}
			var row reminderEvent
			if err := tx.Where("id = ? AND notification_status = 'queued'", candidate.ID).First(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			} else if err != nil {
				return err
			}
			status := "not_authorized"
			p, err := loadReminder(tx, row.UserID, row.ReminderID)
			if errors.Is(err, ErrNotFound) {
				status = "canceled"
			} else if err != nil {
				return err
			} else if p.Status != "active" || row.Status != "due" {
				status = "canceled"
			} else if row.DueAt.Before(now.Add(-10 * time.Minute)) {
				status = "expired"
			} else {
				local := &Store{db: tx, appID: s.appID}
				n, err := local.NotificationSettings(ctx, row.UserID)
				if err != nil {
					return err
				}
				if n.Enabled && n.Credits > 0 && n.TemplateID == template {
					b, err := local.binding(ctx, row.UserID)
					if err != nil {
						return err
					}
					result := tx.Model(&notificationRecord{}).Where("user_id = ? AND enabled = ? AND credits > 0 AND template_id = ? AND bound_app_id = ? AND bound_open_id = ?", row.UserID, true, template, b.AppID, b.OpenID).Update("credits", gorm.Expr("credits - 1"))
					if result.Error != nil {
						return result.Error
					}
					if result.RowsAffected == 1 {
						status = "dispatching"
						delivery = &domain.NotificationDelivery{ID: fmt.Sprintf("%s:%d", row.ID, row.Revision), UserID: row.UserID, AppID: b.AppID, OpenID: b.OpenID, TemplateID: template, Title: p.Title, DueAt: row.DueAt}
					}
				}
			}
			return tx.Model(&reminderEvent{}).Where("id = ?", row.ID).Updates(map[string]interface{}{"notification_status": status, "dispatch_started_at": now}).Error
		})
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, false, err
		}
		if delivery != nil {
			return delivery, true, nil
		}
	}
	// A full batch of skipped candidates does not mean the queue is empty.
	return nil, len(candidates) == 16, nil
}
func (s *Store) FinishReminderNotification(ctx context.Context, id, status string) error {
	if status != "sent" && status != "uncertain" {
		return ErrInvalid
	}
	parts := strings.Split(id, ":")
	if len(parts) != 2 {
		return ErrInvalid
	}
	return s.db.WithContext(ctx).Model(&reminderEvent{}).Where("id = ? AND revision = ? AND notification_status = 'dispatching'", parts[0], parts[1]).Update("notification_status", status).Error
}
