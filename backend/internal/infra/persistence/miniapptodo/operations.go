package miniapptodo

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func applyOperation(tx *gorm.DB, o domain.Owner, op domain.Operation, now time.Time) error {
	if strings.HasPrefix(op.Kind, "list.") {
		return applyList(tx, o, op, now)
	}
	task, err := getTask(tx, o, op.EntityID)
	exists := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if (!exists && op.BaseVersion != 0) || (exists && task.Version != op.BaseVersion) {
		return fmt.Errorf("%w: 内容已被修改，请选择保留方式", domain.ErrConflict)
	}
	if op.Kind == "task.save" {
		if op.Task == nil || op.Task.ID != op.EntityID {
			return domain.ErrInvalid
		}
		draft := *op.Task
		if err := domain.ValidateDraft(&draft); err != nil {
			return err
		}
		if exists && (task.DeletedAt != nil || task.ParentID != draft.ParentID) {
			return fmt.Errorf("%w: 不能编辑已删除任务或改变层级", domain.ErrInvalid)
		}
		var list listRecord
		if err := owned(tx, o).Where("id = ? AND deleted_at IS NULL", draft.ListID).Take(&list).Error; err != nil {
			return err
		}
		if draft.ParentID != "" {
			parent, err := getTask(tx, o, draft.ParentID)
			if err != nil {
				return err
			}
			if parent.ParentID != "" || parent.DeletedAt != nil || parent.CompletedAt != nil || parent.ListID != draft.ListID {
				return fmt.Errorf("%w: 只能为未完成任务添加一层步骤", domain.ErrInvalid)
			}
		}
		if !exists {
			var count int64
			if err := owned(tx.Model(&taskRecord{}), o).Where("completed_at IS NULL AND deleted_at IS NULL").Count(&count).Error; err != nil {
				return err
			}
			if count >= domain.SnapshotLimit {
				return fmt.Errorf("%w: 最多保留 5000 项未完成任务，请先完成或删除部分任务", domain.ErrInvalid)
			}
			task = domain.Task{ID: draft.ID, CreatedAt: now}
		}
		oldList := task.ListID
		task.ListID = draft.ListID
		task.ParentID = draft.ParentID
		task.Title = draft.Title
		task.Notes = draft.Notes
		task.DueDate = draft.DueDate
		task.DueTime = draft.DueTime
		task.Timezone = draft.Timezone
		task.Important = draft.Important
		task.Position = draft.Position
		task.RepeatKind = draft.RepeatKind
		task.RepeatDay = draft.RepeatDay
		if task.RepeatKind != "none" && task.SeriesID == "" {
			task.SeriesID = task.ID
		}
		task.Version++
		task.UpdatedAt = now
		if exists && oldList != task.ListID {
			children, err := childrenOf(tx, o, task.ID)
			if err != nil {
				return err
			}
			for _, child := range children {
				child.ListID = task.ListID
				child.Version++
				child.UpdatedAt = now
				if err := putTask(tx, o, child); err != nil {
					return err
				}
			}
		}
	} else {
		if !exists {
			return domain.ErrInvalid
		}
		switch op.Kind {
		case "task.complete":
			if task.DeletedAt != nil {
				return domain.ErrInvalid
			}
			if (task.CompletedAt != nil) == op.Completed {
				return nil
			}
			children, err := childrenOf(tx, o, task.ID)
			if err != nil {
				return err
			}
			if op.Completed {
				task.CompletedAt = &now
				for _, child := range children {
					if child.CompletedAt == nil && child.DeletedAt == nil {
						child.CompletedAt = &now
						child.Version++
						child.UpdatedAt = now
						if err := putTask(tx, o, child); err != nil {
							return err
						}
					}
				}
				if task.RepeatKind != "none" {
					if err := createNext(tx, o, task, children, now); err != nil {
						return err
					}
				}
			} else {
				if task.RepeatKind != "none" {
					if err := undoNext(tx, o, task, now); err != nil {
						return err
					}
				}
				completedAt := task.CompletedAt
				for _, child := range children {
					// Only undo steps completed by this exact parent action, and untouched since.
					if child.CompletedAt != nil && child.CompletedAt.Equal(*completedAt) && child.UpdatedAt.Equal(*completedAt) && child.DeletedAt == nil {
						child.CompletedAt = nil
						child.Version++
						child.UpdatedAt = now
						if err := putTask(tx, o, child); err != nil {
							return err
						}
					}
				}
				task.CompletedAt = nil
			}
		case "task.delete", "task.restore":
			if op.Kind == "task.restore" && task.ParentID != "" {
				parent, err := getTask(tx, o, task.ParentID)
				if err != nil {
					return err
				}
				if parent.DeletedAt != nil {
					return fmt.Errorf("%w: 请先恢复所属任务", domain.ErrInvalid)
				}
			}
			if op.Kind == "task.delete" && task.DeletedAt != nil {
				return nil
			}
			if op.Kind == "task.restore" && task.DeletedAt == nil {
				return nil
			}
			deletedAt := task.DeletedAt
			if op.Kind == "task.delete" {
				task.DeletedAt = &now
			} else {
				task.DeletedAt = nil
			}
			children, err := childrenOf(tx, o, task.ID)
			if err != nil {
				return err
			}
			for _, child := range children {
				if (op.Kind == "task.delete" && child.DeletedAt == nil) || (op.Kind == "task.restore" && child.DeletedAt != nil && child.DeletedAt.Equal(*deletedAt)) {
					child.DeletedAt = task.DeletedAt
					child.Version++
					child.UpdatedAt = now
					if err := putTask(tx, o, child); err != nil {
						return err
					}
				}
			}
		default:
			return domain.ErrInvalid
		}
		task.Version++
		task.UpdatedAt = now
	}
	if err := putTask(tx, o, task); err != nil {
		return err
	}
	if task.ParentID != "" {
		parent, err := getTask(tx, o, task.ParentID)
		if err != nil {
			return err
		}
		parent.Version++
		parent.UpdatedAt = now
		if err := putTask(tx, o, parent); err != nil {
			return err
		}
	}
	return nil
}

func childrenOf(tx *gorm.DB, o domain.Owner, id string) ([]domain.Task, error) {
	var rows []taskRecord
	if err := owned(tx, o).Where("parent_id = ?", id).Find(&rows).Error; err != nil {
		return nil, err
	}
	tasks := make([]domain.Task, 0, len(rows))
	for _, row := range rows {
		task, err := decode(row)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}
func nextID(o domain.Owner, task domain.Task) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(o.Key()+":"+task.SeriesID+fmt.Sprintf(":%d", task.Occurrence+1))).String()
}
func createNext(tx *gorm.DB, o domain.Owner, task domain.Task, children []domain.Task, now time.Time) error {
	date, err := domain.NextDate(task, now)
	if err != nil {
		return err
	}
	next := task
	next.ID = nextID(o, task)
	next.Occurrence++
	next.DueDate = date
	next.CompletedAt = nil
	next.DeletedAt = nil
	next.Version = 1
	next.CreatedAt = now
	next.UpdatedAt = now
	prior, err := getTask(tx, o, next.ID)
	if err == nil {
		if prior.DeletedAt == nil {
			return domain.ErrConflict
		}
		next.Version = prior.Version + 1
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := putTask(tx, o, next); err != nil {
		return err
	}
	if err := owned(tx.Model(&taskRecord{}), o).Where("id = ?", next.ID).Update("generation_version", next.Version).Error; err != nil {
		return err
	}
	for _, child := range children {
		if child.DeletedAt != nil {
			continue
		}
		copy := child
		copy.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(next.ID+":"+child.ID)).String()
		copy.ParentID = next.ID
		copy.CompletedAt = nil
		copy.DeletedAt = nil
		copy.Version = 1
		copy.CreatedAt = now
		copy.UpdatedAt = now
		copy.DueDate = ""
		copy.DueTime = ""
		prior, err := getTask(tx, o, copy.ID)
		if err == nil {
			copy.Version = prior.Version + 1
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := putTask(tx, o, copy); err != nil {
			return err
		}
		if err := owned(tx.Model(&taskRecord{}), o).Where("id = ?", copy.ID).Update("generation_version", copy.Version).Error; err != nil {
			return err
		}
	}
	return nil
}
func undoNext(tx *gorm.DB, o domain.Owner, task domain.Task, now time.Time) error {
	next, err := getTask(tx, o, nextID(o, task))
	if err != nil {
		return err
	}
	var record taskRecord
	if err := owned(tx, o).Where("id = ?", next.ID).Take(&record).Error; err != nil {
		return err
	}
	if next.DeletedAt != nil || next.CompletedAt != nil || record.GenerationVersion != next.Version {
		return fmt.Errorf("%w: 下一次任务已被修改，可保留为普通待办", domain.ErrConflict)
	}
	children, err := childrenOf(tx, o, next.ID)
	if err != nil {
		return err
	}
	for _, child := range children {
		var childRecord taskRecord
		if err := owned(tx, o).Where("id = ?", child.ID).Take(&childRecord).Error; err != nil {
			return err
		}
		if childRecord.GenerationVersion == 0 || childRecord.GenerationVersion != child.Version || child.CompletedAt != nil || child.DeletedAt != nil {
			return domain.ErrConflict
		}
	}
	for _, child := range children {
		child.DeletedAt = &now
		child.Version++
		child.UpdatedAt = now
		if err := putTask(tx, o, child); err != nil {
			return err
		}
	}
	next.DeletedAt = &now
	next.Version++
	next.UpdatedAt = now
	return putTask(tx, o, next)
}
func applyList(tx *gorm.DB, o domain.Owner, op domain.Operation, now time.Time) error {
	if op.EntityID == "inbox" {
		return fmt.Errorf("%w: 收件箱不可修改或删除", domain.ErrInvalid)
	}
	var row listRecord
	err := owned(tx, o).Where("id = ?", op.EntityID).Take(&row).Error
	exists := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if (!exists && op.BaseVersion != 0) || (exists && row.Version != op.BaseVersion) {
		return domain.ErrConflict
	}
	if exists && row.DeletedAt != nil {
		return domain.ErrInvalid
	}
	switch op.Kind {
	case "list.save":
		if op.List == nil || op.List.ID != op.EntityID || strings.TrimSpace(op.List.Name) == "" || utf8.RuneCountInString(op.List.Name) > 100 {
			return domain.ErrInvalid
		}
		row.Ownership = own(o)
		row.ID = op.EntityID
		row.Name = strings.TrimSpace(op.List.Name)
		row.Position = op.List.Position
		row.Version++
		if !exists {
			return tx.Create(&row).Error
		}
	case "list.delete":
		if !exists {
			return domain.ErrInvalid
		}
		row.DeletedAt = &now
		row.Version++
		var tasks []taskRecord
		if err := owned(tx, o).Where("list_id = ?", op.EntityID).Find(&tasks).Error; err != nil {
			return err
		}
		for _, record := range tasks {
			task, err := decode(record)
			if err != nil {
				return err
			}
			task.ListID = "inbox"
			task.Version++
			task.UpdatedAt = now
			if err := putTask(tx, o, task); err != nil {
				return err
			}
		}
	default:
		return domain.ErrInvalid
	}
	return owned(tx.Model(&listRecord{}), o).Where("id = ?", row.ID).Select("name", "position", "version", "deleted_at").Updates(&row).Error
}
