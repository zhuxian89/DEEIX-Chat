package miniappaccess

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	tododomain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	todo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/miniapptodo"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func database(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "access.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err = db.AutoMigrate(&model.UserSession{}, &model.UserAuthEvent{}, &model.WeChatMiniAppBinding{}); err != nil {
		t.Fatal(err)
	}
	if _, err = todo.NewStore(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCutoverPreservesProvenWebAndBlocksUnknownOnlyOnce(t *testing.T) {
	db := database(t)
	for _, sid := range []string{"web", "unknown", "mini", "forged-event", "malformed"} {
		if err := db.Create(&model.UserSession{SessionID: sid, UserID: 7, ExpiresAt: time.Now().Add(time.Hour), UserAgent: "DEEIX-WeChat-MiniApp"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []model.UserAuthEvent{
		{UserID: 7, EventType: "login", Result: "success", DetailJSON: `{"session_id":"web"}`},
		{UserID: 7, EventType: "wechat_miniapp_login", Result: "success", DetailJSON: `{"session_id":"mini"}`},
		{UserID: 8, EventType: "login", Result: "success", DetailJSON: `{"session_id":"forged-event"}`},
		{UserID: 7, EventType: "login", Result: "success", DetailJSON: `{broken`},
	} {
		if err := db.Create(&e).Error; err != nil {
			t.Fatal(err)
		}
	}
	s, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, sid := range []string{"web", "unknown", "mini", "forged-event", "malformed"} {
		d, err := s.Access(t.Context(), 7, sid)
		if err != nil {
			t.Fatal(err)
		}
		if d.Reauthenticate != (sid != "web") {
			t.Fatalf("%s: %+v", sid, d)
		}
	}
	if err := db.Create(&model.UserSession{SessionID: "new-web", UserID: 7, ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	// Audit retention and restart cannot turn blocked sessions into Web or re-block new Web.
	if err := db.Where("1=1").Unscoped().Delete(&model.UserAuthEvent{}).Error; err != nil {
		t.Fatal(err)
	}
	s, err = NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, sid := range []string{"web", "new-web", "unknown"} {
		d, err := s.Access(t.Context(), 7, sid)
		if err != nil {
			t.Fatal(err)
		}
		if d.Reauthenticate != (sid == "unknown") {
			t.Fatalf("restart %s: %+v", sid, d)
		}
	}
}

func TestMiniappIdentityUnlockReloginAndWebIsolation(t *testing.T) {
	db := database(t)
	s, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.WeChatMiniAppBinding{AppID: "app", OpenID: "open", UserID: 7}).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.Register(t.Context(), 7, "mini", "app", "open"); err != nil {
		t.Fatal(err)
	}
	d, err := s.Access(t.Context(), 7, "mini")
	if err != nil || !d.MiniApp || d.Unlocked {
		t.Fatalf("locked %+v %v", d, err)
	}
	todos, err := todo.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = todos.Unlock(t.Context(), tododomain.Owner{AppID: "other", OpenID: "open"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	d, err = s.Access(t.Context(), 7, "mini")
	if err != nil || d.Unlocked {
		t.Fatalf("cross app %+v %v", d, err)
	}
	if _, err = todos.Unlock(t.Context(), tododomain.Owner{AppID: "app", OpenID: "open"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	d, err = s.Access(t.Context(), 7, "mini")
	if err != nil || !d.Unlocked {
		t.Fatalf("unlocked %+v %v", d, err)
	}
	d, err = s.Access(t.Context(), 8, "mini")
	if err != nil || !d.Reauthenticate {
		t.Fatalf("cross user %+v %v", d, err)
	}
	if err = s.Register(t.Context(), 7, "mini-new", "app", "open"); err != nil {
		t.Fatal(err)
	}
	d, err = s.Access(t.Context(), 7, "mini")
	if err != nil || !d.Reauthenticate {
		t.Fatalf("old session %+v %v", d, err)
	}
	d, err = s.Access(t.Context(), 7, "mini-new")
	if err != nil || !d.Unlocked {
		t.Fatalf("new session %+v %v", d, err)
	}
	d, err = s.Access(t.Context(), 7, "web")
	if err != nil || d.MiniApp || d.Reauthenticate {
		t.Fatalf("web affected %+v %v", d, err)
	}
	// Duplicate registration rolls back invalidation of the current session.
	if err = s.Register(t.Context(), 7, "mini-new", "app", "open"); err == nil {
		t.Fatal("duplicate accepted")
	}
	d, err = s.Access(t.Context(), 7, "mini-new")
	if err != nil || d.Reauthenticate {
		t.Fatalf("rollback %+v %v", d, err)
	}
	if err = db.Model(&model.WeChatMiniAppBinding{}).Where("user_id = ?", 7).Update("revoked_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	d, err = s.Access(t.Context(), 7, "mini-new")
	if err != nil || d.Unlocked {
		t.Fatalf("revoked binding %+v %v", d, err)
	}
}

func TestAccessDatabaseFailureDoesNotBecomeWeb(t *testing.T) {
	db := database(t)
	s, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	if _, err = s.Access(context.Background(), 7, "missing"); err == nil {
		t.Fatal("database failure allowed")
	}
}

func TestCutoverBatchesAndNoUpstreamSchemaChanges(t *testing.T) {
	db := database(t)
	before, err := db.Migrator().ColumnTypes(&model.UserSession{})
	if err != nil {
		t.Fatal(err)
	}
	var rows []model.UserSession
	for i := 0; i < 501; i++ {
		rows = append(rows, model.UserSession{SessionID: fmt.Sprintf("old-%d", i), UserID: 7, ExpiresAt: time.Now().Add(time.Hour)})
	}
	if err = db.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Access(t.Context(), 7, "old-500")
	if err != nil || !d.Reauthenticate {
		t.Fatalf("last batch %+v %v", d, err)
	}
	after, err := db.Migrator().ColumnTypes(&model.UserSession{})
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatal("upstream session schema changed")
	}
}
