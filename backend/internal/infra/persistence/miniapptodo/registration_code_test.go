package miniapptodo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func registrationTestStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	var s *Store
	var db *gorm.DB
	if dsn := os.Getenv("MINIAPP_TEST_POSTGRES_DSN"); dsn != "" {
		// Opt-in integration run against a test database, isolated in a disposable schema.
		config, err := pgx.ParseConfig(dsn)
		if err != nil {
			t.Fatal("invalid test database configuration")
		}
		config.ConnectTimeout = 10 * time.Second
		admin := stdlib.OpenDB(*config)
		schema := "miniapp_test_" + fmt.Sprintf("%x", uuid.New())
		if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
			_ = admin.Close()
			t.Fatal("cannot create isolated test schema")
		}
		t.Cleanup(func() {
			_, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
			_ = admin.Close()
			if err != nil {
				t.Error("cannot clean up isolated test schema", schema)
			}
		})
		config.RuntimeParams["search_path"] = schema
		conn := stdlib.OpenDB(*config)
		t.Cleanup(func() { _ = conn.Close() })
		db, err = gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{Logger: logger.Discard})
		if err != nil {
			t.Fatal("cannot open isolated test database")
		}
		s, err = NewStore(db)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		s, db = testStore(t)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// Multiple connections exercise database arbitration, not just Go serialization.
	sqlDB.SetMaxOpenConns(8)
	if err := db.AutoMigrate(&model.RegistrationCode{}, &model.WeChatRegistrationIssuance{}); err != nil {
		t.Fatal(err)
	}
	return s, db
}

func seedRegistration(t *testing.T, db *gorm.DB, code, status string, issued bool) model.RegistrationCode {
	t.Helper()
	row := model.RegistrationCode{Code: code, CodeHint: "TEST", Status: status}
	if status == "used" {
		at := time.Now().UTC().Truncate(time.Second)
		row.UsedByUserID, row.UsedAt = 999999, &at // The website user need not still exist.
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if issued {
		if err := db.Create(&model.WeChatRegistrationIssuance{OpenID: "official-" + code, RegistrationCodeID: row.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return row
}

func countRecords(t *testing.T, db *gorm.DB, value interface{}, want int64) {
	t.Helper()
	var n int64
	if err := db.Model(value).Count(&n).Error; err != nil || n != want {
		t.Fatalf("%T count=%d want=%d error=%v", value, n, want, err)
	}
}

func TestRegistrationRedemptionIndependentOfWebsite(t *testing.T) {
	for _, status := range []string{"active", "used"} {
		t.Run(status, func(t *testing.T) {
			s, db := registrationTestStore(t)
			seed := seedRegistration(t, db, "REG-TEST", status, true)
			var before model.RegistrationCode
			if err := db.First(&before, seed.ID).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC().Truncate(time.Second)
			at, err := s.RedeemRegistrationCode(t.Context(), testOwner, " reg-test ", now)
			if err != nil || !at.Equal(now) {
				t.Fatal(at, err)
			}
			var after model.RegistrationCode
			if err := db.First(&after, seed.ID).Error; err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("website code changed", before, after, err)
			}
			if status == "active" {
				// The existing website's conditional consumption still succeeds exactly once.
				for attempt := 0; attempt < 2; attempt++ {
					result := db.Model(&model.RegistrationCode{}).Where("id = ? AND status = ?", seed.ID, "active").
						Updates(map[string]interface{}{"status": "used", "used_by_user_id": 123, "used_at": now})
					if result.Error != nil || result.RowsAffected != int64(1-attempt) {
						t.Fatal("website consumption affected", result.RowsAffected, result.Error)
					}
				}
			}
			seedRegistration(t, db, "REG-SECOND", "active", true)
			for _, code := range []string{"REG-TEST", "REG-SECOND"} {
				repeated, err := s.RedeemRegistrationCode(t.Context(), testOwner, code, now.Add(time.Hour))
				if err != nil || !repeated.Equal(at) {
					t.Fatal("repeat changed permanent grant", repeated, err)
				}
			}
			countRecords(t, db, &registrationCodeBinding{}, 1)
			for _, other := range []domain.Owner{{AppID: "app", OpenID: "other"}, {AppID: "another", OpenID: "open"}} {
				if _, err := s.RedeemRegistrationCode(t.Context(), other, "REG-TEST", now); !errors.Is(err, domain.ErrInvalidCode) {
					t.Fatal("code shared across identities", other, err)
				}
				if grant, err := s.UnlockedAt(t.Context(), other); err != nil || grant != nil {
					t.Fatal("losing identity granted access", grant, err)
				}
			}
			// A new code submitted after activation was not consumed.
			if _, err := s.RedeemRegistrationCode(t.Context(), domain.Owner{AppID: "app", OpenID: "second"}, "REG-SECOND", now); err != nil {
				t.Fatal(err)
			}
			restarted, err := NewStore(db)
			if err != nil {
				t.Fatal(err)
			}
			if grant, err := restarted.UnlockedAt(t.Context(), testOwner); err != nil || grant == nil || !grant.Equal(at) {
				t.Fatal("grant lost after restart", grant, err)
			}
		})
	}
}

func TestRegistrationRejectsUnissuedAndInvalidCodes(t *testing.T) {
	s, db := registrationTestStore(t)
	seedRegistration(t, db, "REG-ADMIN", "active", false)
	seedRegistration(t, db, "REG-DISABLED", "disabled", true)
	for _, code := range []string{"", "888", "REG-", "REG-MISSING", "REG-ADMIN", "REG-DISABLED"} {
		if _, err := s.RedeemRegistrationCode(t.Context(), testOwner, code, time.Now()); !errors.Is(err, domain.ErrInvalidCode) {
			t.Fatal(code, err)
		}
	}
	if _, err := s.RedeemRegistrationCode(t.Context(), domain.Owner{}, "REG-ADMIN", time.Now()); !errors.Is(err, domain.ErrIdentity) {
		t.Fatal(err)
	}
	countRecords(t, db, &registrationCodeBinding{}, 0)
	countRecords(t, db, &unlockRecord{}, 0)
	countRecords(t, db, &listRecord{}, 0)
}

func TestRegistrationBindingRollsBackWhenUnlockFails(t *testing.T) {
	s, db := registrationTestStore(t)
	seedRegistration(t, db, "REG-ROLLBACK", "active", true)
	if err := db.Callback().Create().Before("gorm:create").Register("test:reject_unlock", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "miniapp_entry_unlocks" {
			tx.AddError(errors.New("test unlock failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Create().Remove("test:reject_unlock") })
	if _, err := s.RedeemRegistrationCode(t.Context(), testOwner, "REG-ROLLBACK", time.Now()); err == nil {
		t.Fatal("expected unlock failure")
	}
	countRecords(t, db, &registrationCodeBinding{}, 0)
	countRecords(t, db, &unlockRecord{}, 0)
	if err := db.Callback().Create().Remove("test:reject_unlock"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RedeemRegistrationCode(t.Context(), testOwner, "REG-ROLLBACK", time.Now()); err != nil {
		t.Fatal("rollback consumed code", err)
	}
}

func TestRegistrationConcurrentRedemption(t *testing.T) {
	for _, sameOwner := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_owner_%v", sameOwner), func(t *testing.T) {
			s, db := registrationTestStore(t)
			const attempts = 8
			for i := range attempts {
				seedRegistration(t, db, fmt.Sprintf("REG-%d", i), "active", true)
			}
			start := make(chan struct{})
			errs := make(chan error, attempts)
			var wg sync.WaitGroup
			for i := range attempts {
				wg.Add(1)
				go func() {
					defer wg.Done()
					o, code := domain.Owner{AppID: "app", OpenID: fmt.Sprint(i)}, "REG-0"
					if sameOwner {
						o, code = testOwner, fmt.Sprintf("REG-%d", i)
					}
					<-start
					_, err := s.RedeemRegistrationCode(context.Background(), o, code, time.Now().UTC())
					errs <- err
				}()
			}
			close(start)
			wg.Wait()
			close(errs)
			var successes int
			for err := range errs {
				if err == nil {
					successes++
				} else if !errors.Is(err, domain.ErrInvalidCode) {
					t.Fatal(err)
				}
			}
			want := 1
			if sameOwner {
				want = attempts
			}
			if successes != want {
				t.Fatalf("successes=%d want=%d", successes, want)
			}
			countRecords(t, db, &registrationCodeBinding{}, 1)
			countRecords(t, db, &unlockRecord{}, 1)
		})
	}
}
