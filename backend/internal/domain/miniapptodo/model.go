package miniapptodo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalid       = errors.New("invalid TODO request")
	ErrIdentity      = errors.New("verified miniapp identity required")
	ErrConflict      = errors.New("TODO version conflict")
	ErrSnapshotLimit = errors.New("TODO snapshot limit exceeded")
)

const SnapshotLimit = 5000

type Owner struct {
	AppID  string
	OpenID string
}

func (o Owner) Key() string {
	sum := sha256.Sum256([]byte(o.AppID + "\x00" + o.OpenID))
	return hex.EncodeToString(sum[:])
}
func (o Owner) Valid() bool {
	return strings.TrimSpace(o.AppID) != "" && strings.TrimSpace(o.OpenID) != ""
}

type EntryStatus struct {
	OwnerKey   string
	Unlocked   bool
	UnlockedAt *time.Time
}
type List struct {
	ID        string
	Name      string
	Position  int
	Version   int64
	DeletedAt *time.Time
}
type Task struct {
	ID          string
	ListID      string
	ParentID    string
	Title       string
	Notes       string
	DueDate     string
	DueTime     string
	Timezone    string
	Important   bool
	Position    int
	RepeatKind  string
	RepeatDay   int
	SeriesID    string
	Occurrence  int
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
	DeletedAt   *time.Time
}
type TaskDraft struct {
	ID         string
	ListID     string
	ParentID   string
	Title      string
	Notes      string
	DueDate    string
	DueTime    string
	Timezone   string
	Important  bool
	Position   int
	RepeatKind string
	RepeatDay  int
}
type ListDraft struct {
	ID       string
	Name     string
	Position int
}
type Operation struct {
	ID          string
	Kind        string
	EntityID    string
	BaseVersion int64
	Task        *TaskDraft
	List        *ListDraft
	Completed   bool
}
type OperationResult struct {
	ID      string
	Status  string
	Message string
}
type Snapshot struct {
	Lists []List
	Tasks []Task
}
type SyncResult struct {
	Results  []OperationResult
	Snapshot Snapshot
}
type Query struct {
	Q         string
	Completed string
	ListID    string
	Page      int
	PageSize  int
}
type TaskPage struct {
	Results []Task
	Total   int64
}
type Export struct {
	Filename string
	Content  string
	MimeType string
}
type Repository interface {
	ResolveOwner(context.Context, uint, string) (Owner, error)
	UnlockedAt(context.Context, Owner) (*time.Time, error)
	Unlock(context.Context, Owner, time.Time) (time.Time, error)
	Snapshot(context.Context, Owner) (Snapshot, error)
	Apply(context.Context, Owner, Operation, time.Time) (OperationResult, error)
	Query(context.Context, Owner, Query) (TaskPage, error)
	ExportTasks(context.Context, Owner, Query) ([]Task, error)
	Feedback(context.Context, Owner, string, time.Time) error
}

func ValidID(id string) bool { _, err := uuid.Parse(id); return err == nil }
func ValidateDraft(d *TaskDraft) error {
	if d == nil {
		return fmt.Errorf("%w: missing task", ErrInvalid)
	}
	d.Title = strings.TrimSpace(d.Title)
	if d.Title == "" || utf8.RuneCountInString(d.Title) > 200 || utf8.RuneCountInString(d.Notes) > 10000 {
		return fmt.Errorf("%w: title or notes length", ErrInvalid)
	}
	if !ValidID(d.ID) || (d.ListID != "inbox" && !ValidID(d.ListID)) || (d.ParentID != "" && !ValidID(d.ParentID)) || d.ParentID == d.ID {
		return fmt.Errorf("%w: task identifier", ErrInvalid)
	}
	if d.Timezone == "" {
		d.Timezone = "Asia/Shanghai"
	}
	if _, err := time.LoadLocation(d.Timezone); err != nil {
		return fmt.Errorf("%w: timezone", ErrInvalid)
	}
	if d.DueDate != "" {
		if _, err := time.Parse("2006-01-02", d.DueDate); err != nil {
			return fmt.Errorf("%w: date", ErrInvalid)
		}
	}
	if d.DueTime != "" {
		if _, err := time.Parse("15:04", d.DueTime); err != nil || d.DueDate == "" {
			return fmt.Errorf("%w: time requires a date", ErrInvalid)
		}
	}
	if d.RepeatKind == "" {
		d.RepeatKind = "none"
	}
	switch d.RepeatKind {
	case "none":
		d.RepeatDay = 0
	case "daily", "weekdays":
		d.RepeatDay = 0
	case "weekly":
		if d.RepeatDay < 0 || d.RepeatDay > 6 {
			return fmt.Errorf("%w: weekly day", ErrInvalid)
		}
	case "monthly":
		if d.RepeatDay < 1 || d.RepeatDay > 31 {
			return fmt.Errorf("%w: monthly day", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: repeat kind", ErrInvalid)
	}
	if d.RepeatKind != "none" && (d.DueDate == "" || d.ParentID != "") {
		return fmt.Errorf("%w: repeating task requires a date and cannot be a subtask", ErrInvalid)
	}
	return nil
}

// NextDate advances from the calendar rule, skipping missed dates without
// losing a monthly 29/30/31 anchor in short months.
func NextDate(task Task, now time.Time) (string, error) {
	loc, err := time.LoadLocation(task.Timezone)
	if err != nil {
		return "", err
	}
	original, err := time.ParseInLocation("2006-01-02", task.DueDate, loc)
	if err != nil {
		return "", err
	}
	today, err := time.ParseInLocation("2006-01-02", now.In(loc).Format("2006-01-02"), loc)
	if err != nil {
		return "", err
	}
	after := original
	if today.After(after) {
		after = today
	}
	next := after.AddDate(0, 0, 1)
	switch task.RepeatKind {
	case "daily":
	case "weekdays":
		for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
			next = next.AddDate(0, 0, 1)
		}
	case "weekly":
		for int(next.Weekday()) != task.RepeatDay {
			next = next.AddDate(0, 0, 1)
		}
	case "monthly":
		month := time.Date(after.Year(), after.Month(), 1, 0, 0, 0, 0, loc)
		for {
			last := month.AddDate(0, 1, -1).Day()
			day := task.RepeatDay
			if day > last {
				day = last
			}
			next = time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, loc)
			if next.After(after) {
				break
			}
			month = month.AddDate(0, 1, 0)
		}
	default:
		return "", ErrInvalid
	}
	return next.Format("2006-01-02"), nil
}
