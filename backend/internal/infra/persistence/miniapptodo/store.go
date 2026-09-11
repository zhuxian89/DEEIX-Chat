package miniapptodo

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("TODO database unavailable")
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(734920163001)).Error; err != nil {
				return err
			}
		}
		return tx.AutoMigrate(&unlockRecord{}, &listRecord{}, &taskRecord{}, &operationRecord{}, &feedbackRecord{})
	})
	if err != nil {
		return nil, fmt.Errorf("migrate miniapp TODO tables: %w", err)
	}
	return &Store{db: db}, nil
}
func own(o domain.Owner) Ownership { return Ownership{AppID: o.AppID, OwnerOpenID: o.OpenID} }
func owned(db *gorm.DB, o domain.Owner) *gorm.DB {
	return db.Where("app_id = ? AND owner_open_id = ?", o.AppID, o.OpenID)
}
func (s *Store) ResolveOwner(ctx context.Context, userID uint, appID string) (domain.Owner, error) {
	if userID == 0 || strings.TrimSpace(appID) == "" {
		return domain.Owner{}, domain.ErrIdentity
	}
	var binding struct{ OpenID string }
	err := s.db.WithContext(ctx).Table("wechat_miniapp_bindings").Select("open_id").Where("user_id = ? AND app_id = ? AND revoked_at IS NULL", userID, appID).Take(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Owner{}, domain.ErrIdentity
	}
	if err != nil {
		return domain.Owner{}, err
	}
	owner := domain.Owner{AppID: appID, OpenID: binding.OpenID}
	if !owner.Valid() {
		return domain.Owner{}, domain.ErrIdentity
	}
	return owner, nil
}
func (s *Store) UnlockedAt(ctx context.Context, o domain.Owner) (*time.Time, error) {
	var row unlockRecord
	err := s.db.WithContext(ctx).Where("app_id = ? AND open_id = ?", o.AppID, o.OpenID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row.UnlockedAt, nil
}
func (s *Store) Unlock(ctx context.Context, o domain.Owner, now time.Time) (time.Time, error) {
	if !o.Valid() {
		return time.Time{}, domain.ErrIdentity
	}
	row := unlockRecord{AppID: o.AppID, OpenID: o.OpenID, UnlockedAt: now}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return time.Time{}, err
	}
	at, err := s.UnlockedAt(ctx, o)
	if err != nil {
		return time.Time{}, err
	}
	return *at, nil
}

// All mutations of one owner serialize in the database, including across servers.
// SQLite obtains its write reservation before reading versions; PostgreSQL uses
// an advisory transaction lock without touching the existing identity tables.
func ownerLock(tx *gorm.DB, o domain.Owner) error {
	if !o.Valid() {
		return domain.ErrIdentity
	}
	if tx.Dialector.Name() == "postgres" {
		sum := sha256.Sum256([]byte("miniapp-todo:" + o.Key()))
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(binary.BigEndian.Uint64(sum[:8]))).Error; err != nil {
			return err
		}
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&listRecord{Ownership: own(o), ID: "inbox", Name: "收件箱", Version: 1}).Error
}
func (s *Store) Snapshot(ctx context.Context, o domain.Owner) (domain.Snapshot, error) {
	result := domain.Snapshot{Lists: []domain.List{}, Tasks: []domain.Task{}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ownerLock(tx, o); err != nil {
			return err
		}
		var lists []listRecord
		if err := owned(tx, o).Where("deleted_at IS NULL").Order("position, id").Find(&lists).Error; err != nil {
			return err
		}
		for _, l := range lists {
			result.Lists = append(result.Lists, domain.List{ID: l.ID, Name: l.Name, Position: l.Position, Version: l.Version, DeletedAt: l.DeletedAt})
		}
		var rows []taskRecord
		// All active tasks plus recent history/tombstones support immediate undo.
		if err := owned(tx, o).Where("completed_at IS NULL AND deleted_at IS NULL").Order("id").Limit(domain.SnapshotLimit + 1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > domain.SnapshotLimit {
			return domain.ErrSnapshotLimit
		}
		for _, row := range rows {
			task, err := decode(row)
			if err != nil {
				return err
			}
			result.Tasks = append(result.Tasks, task)
		}
		rows = nil
		if err := owned(tx, o).Where("completed_at IS NOT NULL OR deleted_at IS NOT NULL").Order("updated_at DESC, id").Limit(100).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			task, err := decode(row)
			if err != nil {
				return err
			}
			result.Tasks = append(result.Tasks, task)
		}
		return nil
	})
	return result, err
}
func decode(row taskRecord) (domain.Task, error) {
	var task domain.Task
	err := json.Unmarshal([]byte(row.Data), &task)
	return task, err
}
func getTask(tx *gorm.DB, o domain.Owner, id string) (domain.Task, error) {
	var row taskRecord
	err := owned(tx, o).Where("id = ?", id).Take(&row).Error
	if err != nil {
		return domain.Task{}, err
	}
	return decode(row)
}
func putTask(tx *gorm.DB, o domain.Owner, task domain.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	row := taskRecord{Ownership: own(o), ID: task.ID, ListID: task.ListID, ParentID: task.ParentID, Title: task.Title, Notes: task.Notes, Version: task.Version, Data: string(data), UpdatedAt: task.UpdatedAt, CompletedAt: task.CompletedAt, DeletedAt: task.DeletedAt}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "app_id"}, {Name: "owner_open_id"}, {Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"list_id", "parent_id", "title", "notes", "version", "data", "updated_at", "completed_at", "deleted_at"})}).Create(&row).Error
}
func (s *Store) Apply(ctx context.Context, o domain.Owner, op domain.Operation, now time.Time) (domain.OperationResult, error) {
	result := domain.OperationResult{ID: op.ID, Status: "applied"}
	if !domain.ValidID(op.ID) || (!domain.ValidID(op.EntityID) && op.EntityID != "inbox") || op.BaseVersion < 0 {
		result.Status = "invalid"
		result.Message = "操作标识或版本无效"
		return result, nil
	}
	raw, err := json.Marshal(op)
	if err != nil {
		return result, err
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ownerLock(tx, o); err != nil {
			return err
		}
		var receipt operationRecord
		err := owned(tx, o).Where("id = ?", op.ID).Take(&receipt).Error
		if err == nil {
			if receipt.Digest != digest {
				result.Status = "invalid"
				result.Message = "操作标识不能用于不同内容"
				return nil
			}
			if err := json.Unmarshal([]byte(receipt.Result), &result); err != nil {
				return err
			}
			if result.Status == "applied" {
				result.Status = "duplicate"
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// A savepoint keeps a rejected multi-row change from partially applying.
		changeErr := tx.Transaction(func(change *gorm.DB) error {
			if err := applyOperation(change, o, op, now); err != nil {
				return err
			}
			var active []taskRecord
			if err := owned(change.Select("data"), o).Where("completed_at IS NULL AND deleted_at IS NULL").Limit(domain.SnapshotLimit + 1).Find(&active).Error; err != nil {
				return err
			}
			size := 0
			for _, row := range active {
				size += len(row.Data)
			}
			if len(active) > domain.SnapshotLimit || size > 4*1024*1024 {
				return fmt.Errorf("%w: 未完成任务超过容量，请先完成或删除部分任务", domain.ErrInvalid)
			}
			return nil
		})
		if changeErr != nil {
			switch {
			case errors.Is(changeErr, domain.ErrConflict):
				result.Status = "conflict"
			case errors.Is(changeErr, domain.ErrInvalid), errors.Is(changeErr, gorm.ErrRecordNotFound):
				result.Status = "invalid"
			default:
				return changeErr
			}
			result.Message = changeErr.Error()
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.Create(&operationRecord{Ownership: own(o), ID: op.ID, Digest: digest, Result: string(encoded), CreatedAt: now}).Error
	})
	return result, err
}
func (s *Store) Query(ctx context.Context, o domain.Owner, q domain.Query) (domain.TaskPage, error) {
	result := domain.TaskPage{Results: []domain.Task{}}
	db := queryTasks(s.db.WithContext(ctx), o, q)
	if err := db.Count(&result.Total).Error; err != nil {
		return result, err
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		q.PageSize = 50
	}
	var rows []taskRecord
	if err := db.Order("updated_at DESC, id").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		task, err := decode(row)
		if err != nil {
			return result, err
		}
		result.Results = append(result.Results, task)
	}
	return result, nil
}

func queryTasks(db *gorm.DB, o domain.Owner, q domain.Query) *gorm.DB {
	db = owned(db.Model(&taskRecord{}), o).Where("deleted_at IS NULL")
	if q.ListID != "" {
		db = db.Where("list_id = ?", q.ListID)
	}
	if q.Completed == "true" {
		db = db.Where("completed_at IS NOT NULL")
	} else if q.Completed == "false" {
		db = db.Where("completed_at IS NULL")
	}
	if q.Q != "" {
		pattern := "%" + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(q.Q), "!", "!!"), "%", "!%"), "_", "!_") + "%"
		db = db.Where("LOWER(title) LIKE ? ESCAPE '!' OR LOWER(notes) LIKE ? ESCAPE '!'", pattern, pattern)
	}
	return db
}

// One SELECT gives one database snapshot. Iteration is bounded by both count
// and bytes, so concurrent edits cannot shift an OFFSET and silently lose rows.
func (s *Store) ExportTasks(ctx context.Context, o domain.Owner, q domain.Query) ([]domain.Task, error) {
	rows, err := queryTasks(s.db.WithContext(ctx), o, q).Order("updated_at DESC, id").Limit(10001).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Task, 0)
	size := 0
	for rows.Next() {
		var row taskRecord
		if err := s.db.ScanRows(rows, &row); err != nil {
			return nil, err
		}
		size += len(row.Data)
		if len(result) >= 10000 || size > 8*1024*1024 {
			return nil, domain.ErrExportLimit
		}
		task, err := decode(row)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
	}
	return result, rows.Err()
}
