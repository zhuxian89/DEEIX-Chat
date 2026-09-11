package miniapptodo

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/csv"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
)

type Config struct {
	AppID        string
	CurrentAppID func() string
	FeedbackCode func(context.Context) (string, error)
}
type Service struct {
	store  domain.Repository
	config Config
}

func NewService(store domain.Repository, config Config) *Service {
	return &Service{store: store, config: config}
}
func (s *Service) owner(ctx context.Context, userID uint) (domain.Owner, error) {
	appID := s.config.AppID
	if s.config.CurrentAppID != nil {
		appID = s.config.CurrentAppID()
	}
	return s.store.ResolveOwner(ctx, userID, appID)
}
func (s *Service) Status(ctx context.Context, userID uint) (domain.EntryStatus, error) {
	o, err := s.owner(ctx, userID)
	if err != nil {
		return domain.EntryStatus{}, err
	}
	at, err := s.store.UnlockedAt(ctx, o)
	return domain.EntryStatus{OwnerKey: o.Key(), Unlocked: at != nil, UnlockedAt: at}, err
}
func (s *Service) Feedback(ctx context.Context, userID uint, content string) (domain.EntryStatus, error) {
	content = strings.TrimSpace(content)
	if content == "" || utf8.RuneCountInString(content) > 2000 {
		return domain.EntryStatus{}, domain.ErrInvalid
	}
	o, err := s.owner(ctx, userID)
	if err != nil {
		return domain.EntryStatus{}, err
	}
	at, err := s.store.UnlockedAt(ctx, o)
	if err != nil {
		return domain.EntryStatus{}, err
	}
	status := domain.EntryStatus{OwnerKey: o.Key(), Unlocked: at != nil, UnlockedAt: at}
	if status.Unlocked || s.config.FeedbackCode == nil {
		return status, nil
	}
	code, err := s.config.FeedbackCode(ctx)
	if err != nil {
		return domain.EntryStatus{}, err
	}
	code = strings.TrimSpace(code)
	want, got := sha256.Sum256([]byte(code)), sha256.Sum256([]byte(content))
	if code != "" && subtle.ConstantTimeCompare(want[:], got[:]) == 1 {
		unlockedAt, err := s.store.Unlock(ctx, o, time.Now().UTC())
		if err != nil {
			return domain.EntryStatus{}, err
		}
		status.Unlocked, status.UnlockedAt = true, &unlockedAt
	}
	return status, nil
}
func (s *Service) Snapshot(ctx context.Context, userID uint) (domain.Snapshot, error) {
	o, err := s.owner(ctx, userID)
	if err != nil {
		return domain.Snapshot{}, err
	}
	return s.store.Snapshot(ctx, o)
}
func (s *Service) Sync(ctx context.Context, userID uint, operations []domain.Operation) (domain.SyncResult, error) {
	result := domain.SyncResult{Results: []domain.OperationResult{}}
	if len(operations) == 0 || len(operations) > 50 {
		return result, domain.ErrInvalid
	}
	o, err := s.owner(ctx, userID)
	if err != nil {
		return result, err
	}
	for _, op := range operations {
		receipt, err := s.store.Apply(ctx, o, op, time.Now().UTC())
		if err != nil {
			return result, err
		}
		result.Results = append(result.Results, receipt)
		if receipt.Status == "invalid" || receipt.Status == "conflict" {
			break
		}
	}
	result.Snapshot, err = s.store.Snapshot(ctx, o)
	return result, err
}
func (s *Service) Query(ctx context.Context, userID uint, query domain.Query) (domain.TaskPage, error) {
	if err := validateQuery(query); err != nil {
		return domain.TaskPage{}, err
	}
	o, err := s.owner(ctx, userID)
	if err != nil {
		return domain.TaskPage{}, err
	}
	return s.store.Query(ctx, o, query)
}
func validateQuery(query domain.Query) error {
	if utf8.RuneCountInString(query.Q) > 200 || query.Page < 0 || query.Page > 100000 || query.PageSize < 0 || query.PageSize > 100 ||
		(query.Completed != "" && query.Completed != "true" && query.Completed != "false") ||
		(query.ListID != "" && query.ListID != "inbox" && !domain.ValidID(query.ListID)) {
		return domain.ErrInvalid
	}
	return nil
}
func (s *Service) Export(ctx context.Context, userID uint, query domain.Query, format string) (domain.Export, error) {
	if format != "text" && format != "csv" {
		return domain.Export{}, domain.ErrInvalid
	}
	if err := validateQuery(query); err != nil {
		return domain.Export{}, err
	}
	o, err := s.owner(ctx, userID)
	if err != nil {
		return domain.Export{}, err
	}
	tasks, err := s.store.ExportTasks(ctx, o, query)
	if err != nil {
		return domain.Export{}, err
	}
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	if format == "csv" {
		builder.WriteString("\ufeff")
		_ = writer.Write([]string{"标题", "备注", "日期", "时间", "时区", "重要", "状态", "清单 ID", "父任务 ID"})
	}
	for _, task := range tasks {
		status := "未完成"
		mark := "[ ]"
		if task.CompletedAt != nil {
			status = "已完成"
			mark = "[x]"
		}
		if format == "text" {
			indent := ""
			if task.ParentID != "" {
				indent = "  "
			}
			fmt.Fprintf(&builder, "%s%s %s %s %s\n", indent, mark, task.Title, task.DueDate, task.DueTime)
			if task.Notes != "" {
				fmt.Fprintf(&builder, "  %s\n", task.Notes)
			}
		} else {
			record := []string{task.Title, task.Notes, task.DueDate, task.DueTime, task.Timezone, fmt.Sprint(task.Important), status, task.ListID, task.ParentID}
			for i, value := range record {
				trimmed := strings.TrimLeft(value, " \t\r\n")
				if strings.ContainsAny(value, "\t\r\n") || (trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
					record[i] = "'" + value
				}
			}
			if err := writer.Write(record); err != nil {
				return domain.Export{}, err
			}
		}
		if builder.Len() > 8*1024*1024 {
			return domain.Export{}, domain.ErrExportLimit
		}
	}
	extension := "txt"
	mime := "text/plain;charset=utf-8"
	if format == "csv" {
		writer.Flush()
		if err := writer.Error(); err != nil {
			return domain.Export{}, err
		}
		extension = "csv"
		mime = "text/csv;charset=utf-8"
	}
	if builder.Len() > 8*1024*1024 {
		return domain.Export{}, domain.ErrExportLimit
	}
	return domain.Export{Filename: "todo-" + time.Now().UTC().Format("20060102") + "." + extension, Content: builder.String(), MimeType: mime}, nil
}
