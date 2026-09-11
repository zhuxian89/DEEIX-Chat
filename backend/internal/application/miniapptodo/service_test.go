package miniapptodo

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
)

type fakeStore struct {
	domain.Repository
	mu     sync.Mutex
	grants map[string]time.Time
}

func (f *fakeStore) ExportTasks(context.Context, domain.Owner, domain.Query) ([]domain.Task, error) {
	return []domain.Task{{Title: "=SUM(A1)", Notes: "line one\nline two", ListID: "inbox", Timezone: "Asia/Shanghai"}}, nil
}
func TestExportEscapesCSVAndUsesSingleRepositoryRead(t *testing.T) {
	service := NewService(&fakeStore{grants: map[string]time.Time{}}, Config{AppID: "app"})
	csv, err := service.Export(t.Context(), 1, domain.Query{}, "csv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(csv.Content, "'=SUM(A1)") || !strings.Contains(csv.Content, "line two") || csv.MimeType != "text/csv;charset=utf-8" {
		t.Fatal(csv)
	}
	if _, err := service.Export(t.Context(), 1, domain.Query{Completed: "invalid"}, "csv"); err == nil {
		t.Fatal("invalid filter accepted")
	}
}

func (f *fakeStore) ResolveOwner(_ context.Context, id uint, appID string) (domain.Owner, error) {
	if id == 0 {
		return domain.Owner{}, domain.ErrIdentity
	}
	return domain.Owner{AppID: appID, OpenID: time.Unix(int64(id), 0).String()}, nil
}
func (f *fakeStore) UnlockedAt(_ context.Context, o domain.Owner) (*time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	at, ok := f.grants[o.Key()]
	if !ok {
		return nil, nil
	}
	return &at, nil
}
func (f *fakeStore) Unlock(_ context.Context, o domain.Owner, at time.Time) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if first, ok := f.grants[o.Key()]; ok {
		return first, nil
	}
	f.grants[o.Key()] = at
	return at, nil
}
func TestFeedbackUsesCurrentConfigurationAndPreservesExistingGrants(t *testing.T) {
	store := &fakeStore{grants: map[string]time.Time{}}
	code := "888"
	service := NewService(store, Config{AppID: "app", FeedbackCode: func(context.Context) (string, error) { return code, nil }})
	if status, err := service.Feedback(t.Context(), 1, "666"); err != nil || status.Unlocked {
		t.Fatal("ordinary feedback must succeed without granting access", status, err)
	}
	if len(store.grants) != 0 {
		t.Fatal("wrong code persisted")
	}
	first, err := service.Feedback(t.Context(), 1, " 888 ")
	if err != nil || !first.Unlocked {
		t.Fatal(first, err)
	}
	code = "new-code"
	if status, err := service.Feedback(t.Context(), 2, "888"); err != nil || status.Unlocked {
		t.Fatal("old configuration still accepted", status, err)
	}
	second, err := service.Feedback(t.Context(), 2, "new-code")
	if err != nil || !second.Unlocked || first.OwnerKey == second.OwnerKey {
		t.Fatal(second, err)
	}
	restored, err := NewService(store, Config{AppID: "app"}).Status(t.Context(), 1)
	if err != nil || !restored.Unlocked || !restored.UnlockedAt.Equal(*first.UnlockedAt) {
		t.Fatal(restored, err)
	}
	code = ""
	repeat, err := service.Feedback(t.Context(), 1, "ordinary feedback")
	if err != nil || !repeat.UnlockedAt.Equal(*first.UnlockedAt) {
		t.Fatal(repeat, err)
	}
	other, err := NewService(store, Config{AppID: "another-app"}).Status(t.Context(), 1)
	if err != nil || other.Unlocked {
		t.Fatal(other, err)
	}
	if status, err := service.Feedback(t.Context(), 3, "new-code"); err != nil || status.Unlocked {
		t.Fatal("empty configuration must disable new grants", status, err)
	}
	if _, err := service.Feedback(t.Context(), 3, "  "); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("blank feedback accepted", err)
	}
}

func TestFeedbackConfigurationFailureDoesNotGrantAccess(t *testing.T) {
	store := &fakeStore{grants: map[string]time.Time{}}
	service := NewService(store, Config{AppID: "app", FeedbackCode: func(context.Context) (string, error) { return "888", errors.New("configuration unavailable") }})
	if _, err := service.Feedback(t.Context(), 1, "888"); err == nil || len(store.grants) != 0 {
		t.Fatal("configuration failure must not grant access", err)
	}
}
