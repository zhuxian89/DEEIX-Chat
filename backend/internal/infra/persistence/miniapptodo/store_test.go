package miniapptodo

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "todo.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	s, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return s, db
}

var testOwner = domain.Owner{AppID: "app", OpenID: "open"}

func saveOp(id, title string) domain.Operation {
	return domain.Operation{ID: uuid.NewString(), Kind: "task.save", EntityID: id, Task: &domain.TaskDraft{ID: id, ListID: "inbox", Title: title, Timezone: "Asia/Shanghai", RepeatKind: "none"}}
}
func apply(t *testing.T, s *Store, o domain.Operation) domain.OperationResult {
	t.Helper()
	r, e := s.Apply(t.Context(), testOwner, o, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestMigrationUnlockAndOwnerIsolation(t *testing.T) {
	s, db := testStore(t)
	if _, err := s.Unlock(t.Context(), testOwner, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	original, err := s.Unlock(t.Context(), testOwner, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(db); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Unlock(t.Context(), testOwner, time.Now().Add(2*time.Hour))
	if err != nil || !restored.Equal(original) {
		t.Fatalf("first unlock changed %v %v", restored, err)
	}
	if at, err := s.UnlockedAt(t.Context(), domain.Owner{AppID: "another", OpenID: "open"}); err != nil || at != nil {
		t.Fatalf("cross-app unlock %v %v", at, err)
	}
	var tables []string
	if err := db.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	if len(tables) != 5 {
		t.Fatalf("expected exactly five isolated tables, got %v", tables)
	}
}

func TestOperationsIdempotencyConflictAndReceiptRollback(t *testing.T) {
	s, db := testStore(t)
	id := uuid.NewString()
	op := saveOp(id, "first")
	if r := apply(t, s, op); r.Status != "applied" {
		t.Fatal(r)
	}
	if r := apply(t, s, op); r.Status != "duplicate" {
		t.Fatal(r)
	}
	op.Task.Title = "other"
	if r := apply(t, s, op); r.Status != "invalid" {
		t.Fatal(r)
	}
	op.ID = uuid.NewString()
	if r := apply(t, s, op); r.Status != "conflict" {
		t.Fatal(r)
	}
	snapshot, err := s.Snapshot(t.Context(), testOwner)
	if err != nil || len(snapshot.Tasks) != 1 || snapshot.Tasks[0].Title != "first" {
		t.Fatalf("snapshot %+v %v", snapshot, err)
	}
	other := domain.Owner{AppID: "app", OpenID: "other"}
	snapshot, err = s.Snapshot(t.Context(), other)
	if err != nil || len(snapshot.Tasks) != 0 {
		t.Fatal("cross-user data", snapshot, err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_receipt BEFORE INSERT ON miniapp_todo_operations BEGIN SELECT RAISE(ABORT, 'receipt failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	op = saveOp(uuid.NewString(), "rolled back")
	if _, err := s.Apply(t.Context(), testOwner, op, time.Now()); err == nil {
		t.Fatal("expected receipt failure")
	}
	snapshot, err = s.Snapshot(t.Context(), testOwner)
	if err != nil || len(snapshot.Tasks) != 1 {
		t.Fatalf("task without receipt %+v %v", snapshot, err)
	}
}

func TestConcurrentCompletionCreatesOneNextInstanceAndUndoPreservesChanges(t *testing.T) {
	s, _ := testStore(t)
	id := uuid.NewString()
	op := saveOp(id, "monthly")
	op.Task.DueDate = "2026-01-31"
	op.Task.RepeatKind = "monthly"
	op.Task.RepeatDay = 31
	now := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	if _, err := s.Apply(t.Context(), testOwner, op, now); err != nil {
		t.Fatal(err)
	}
	complete := domain.Operation{ID: uuid.NewString(), EntityID: id, BaseVersion: 1, Kind: "task.complete", Completed: true}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.Apply(context.Background(), testOwner, complete, now); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	page, err := s.Query(t.Context(), testOwner, domain.Query{Page: 1, PageSize: 100})
	if err != nil || len(page.Results) != 2 {
		t.Fatalf("next instances %+v %v", page, err)
	}
	var next domain.Task
	for _, task := range page.Results {
		if task.ID != id {
			next = task
		}
	}
	if next.DueDate != "2026-02-28" {
		t.Fatal(next)
	}
	edit := saveOp(next.ID, "next was edited")
	edit.BaseVersion = next.Version
	edit.Task.DueDate = next.DueDate
	edit.Task.RepeatKind = "monthly"
	edit.Task.RepeatDay = 31
	if r := apply(t, s, edit); r.Status != "applied" {
		t.Fatal(r)
	}
	undo := domain.Operation{ID: uuid.NewString(), EntityID: id, BaseVersion: 2, Kind: "task.complete", Completed: false}
	if r := apply(t, s, undo); r.Status != "conflict" {
		t.Fatalf("unsafe undo: %+v", r)
	}
}

func TestChildChangesPreventRepeatingUndoEvenWithSameClock(t *testing.T) {
	s, _ := testStore(t)
	ctx := t.Context()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	id := uuid.NewString()
	op := saveOp(id, "daily")
	op.Task.RepeatKind = "daily"
	op.Task.DueDate = "2026-01-01"
	if r, e := s.Apply(ctx, testOwner, op, now); e != nil || r.Status != "applied" {
		t.Fatal(r, e)
	}
	if r, e := s.Apply(ctx, testOwner, domain.Operation{ID: uuid.NewString(), EntityID: id, Kind: "task.complete", BaseVersion: 1, Completed: true}, now); e != nil || r.Status != "applied" {
		t.Fatal(r, e)
	}
	page, e := s.Query(ctx, testOwner, domain.Query{PageSize: 100})
	if e != nil {
		t.Fatal(e)
	}
	var next domain.Task
	for _, task := range page.Results {
		if task.ID != id {
			next = task
		}
	}
	child := saveOp(uuid.NewString(), "new step")
	child.Task.ParentID = next.ID
	if r, e := s.Apply(ctx, testOwner, child, now); e != nil || r.Status != "applied" {
		t.Fatal(r, e)
	}
	r, e := s.Apply(ctx, testOwner, domain.Operation{ID: uuid.NewString(), EntityID: id, Kind: "task.complete", BaseVersion: 2}, now)
	if e != nil || r.Status != "conflict" {
		t.Fatalf("edited successor lost: %+v %v", r, e)
	}
}

func TestSearchAndMutationsNeverCrossOwners(t *testing.T) {
	s, _ := testStore(t)
	op := saveOp(uuid.NewString(), "needle")
	op.Task.Notes = "secret note"
	if r := apply(t, s, op); r.Status != "applied" {
		t.Fatal(r)
	}
	other := domain.Owner{AppID: "app", OpenID: "other"}
	for _, q := range []string{"needle", "secret", "%", "_"} {
		page, err := s.Query(t.Context(), other, domain.Query{Q: q})
		if err != nil || page.Total != 0 {
			t.Fatal(page, err)
		}
	}
	attempted := domain.Operation{ID: uuid.NewString(), Kind: "task.delete", EntityID: op.EntityID, BaseVersion: 1}
	r, err := s.Apply(t.Context(), other, attempted, time.Now())
	if err != nil || r.Status == "applied" {
		t.Fatal(r, err)
	}
	page, err := s.Query(t.Context(), testOwner, domain.Query{})
	if err != nil || page.Total != 1 {
		t.Fatal(page, err)
	}
}

func TestSubtasksMoveCompleteUndoAndDeleteRestore(t *testing.T) {
	s, _ := testStore(t)
	parentID := uuid.NewString()
	childID := uuid.NewString()
	apply(t, s, saveOp(parentID, "parent"))
	child := saveOp(childID, "step")
	child.Task.ParentID = parentID
	if r := apply(t, s, child); r.Status != "applied" {
		t.Fatal(r)
	}
	parent, err := getTask(s.db, testOwner, parentID)
	if err != nil || parent.Version != 2 {
		t.Fatal(parent, err)
	}
	if r := apply(t, s, domain.Operation{ID: uuid.NewString(), Kind: "task.complete", EntityID: parentID, BaseVersion: 2, Completed: true}); r.Status != "applied" {
		t.Fatal(r)
	}
	step, err := getTask(s.db, testOwner, childID)
	if err != nil || step.CompletedAt == nil {
		t.Fatal(step, err)
	}
	if r := apply(t, s, domain.Operation{ID: uuid.NewString(), Kind: "task.complete", EntityID: parentID, BaseVersion: 3}); r.Status != "applied" {
		t.Fatal(r)
	}
	step, err = getTask(s.db, testOwner, childID)
	if err != nil || step.CompletedAt != nil {
		t.Fatal(step, err)
	}
	if r := apply(t, s, domain.Operation{ID: uuid.NewString(), Kind: "task.delete", EntityID: parentID, BaseVersion: 4}); r.Status != "applied" {
		t.Fatal(r)
	}
	if r := apply(t, s, domain.Operation{ID: uuid.NewString(), Kind: "task.restore", EntityID: parentID, BaseVersion: 5}); r.Status != "applied" {
		t.Fatal(r)
	}
	step, err = getTask(s.db, testOwner, childID)
	if err != nil || step.DeletedAt != nil {
		t.Fatal(step, err)
	}
}

func TestExportReadsOneSnapshotAndHonorsOwnerAndByteBounds(t *testing.T) {
	s, db := testStore(t)
	records := make([]taskRecord, 0, 1000)
	for i := 0; i < 1000; i++ {
		task := domain.Task{ID: uuid.NewString(), Title: "item", ListID: "inbox", Version: 1, UpdatedAt: time.Now()}
		data, err := json.Marshal(task)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, taskRecord{Ownership: own(testOwner), ID: task.ID, ListID: task.ListID, Title: task.Title, Version: 1, Data: string(data), UpdatedAt: task.UpdatedAt})
	}
	if err := db.CreateInBatches(records, 100).Error; err != nil {
		t.Fatal(err)
	}
	tasks, err := s.ExportTasks(t.Context(), testOwner, domain.Query{})
	if err != nil || len(tasks) != 1000 {
		t.Fatal(len(tasks), err)
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		if seen[task.ID] {
			t.Fatal("duplicate export row")
		}
		seen[task.ID] = true
	}
	other, err := s.ExportTasks(t.Context(), domain.Owner{AppID: "other", OpenID: testOwner.OpenID}, domain.Query{})
	if err != nil || len(other) != 0 {
		t.Fatal(other, err)
	}
	// Storage-level fixtures can represent large historical data; export must
	// reject it rather than exhaust memory or silently truncate the result.
	huge := domain.Task{ID: uuid.NewString(), Title: "large history", Notes: strings.Repeat("x", 8*1024*1024), Version: 1}
	if err := putTask(db, testOwner, huge); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExportTasks(t.Context(), testOwner, domain.Query{}); err == nil {
		t.Fatal("unbounded export accepted")
	}
}
