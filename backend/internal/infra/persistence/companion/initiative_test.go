package companion

import (
	"errors"
	"testing"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
)

func TestCompanionInitiativeAcceptanceRollsBackHistoryAndGreetingWithPacing(t *testing.T) {
	db := testDB(t)
	s, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.Ensure(t.Context(), 1)
	pace, _ := s.InitiativeState(t.Context(), 1)
	token, err := s.Acquire(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Release(1, token)
	now := time.Now()
	item := domain.Initiative{ID: "one", UserID: 1, Kind: "home", VisitID: "visit-123", Text: "你好", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := s.ReserveInitiative(t.Context(), item, token); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_pacing BEFORE UPDATE ON companion_initiative_states BEGIN SELECT RAISE(ABORT, 'simulated pacing failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	pace.HomeCount = 1
	item.AcceptedAt = now.Add(time.Second)
	if err := s.AcceptInitiative(t.Context(), item, token, *pace); err == nil {
		t.Fatal("expected pacing failure")
	}
	stored, _ := s.Initiative(t.Context(), 1, item.ID)
	profile, _ := s.Profile(t.Context(), 1)
	if !stored.AcceptedAt.IsZero() || profile.GreetingID != "" {
		t.Fatal("partially accepted message escaped rollback")
	}
	if err := db.Exec(`DROP TRIGGER reject_pacing`).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.AcceptInitiative(t.Context(), item, token, *pace); err != nil {
		t.Fatal(err)
	}
	if err := s.AcceptInitiative(t.Context(), item, token, *pace); !errors.Is(err, ErrBusy) {
		t.Fatal("store accepted the same candidate twice")
	}
	profile, _ = s.Profile(t.Context(), 1)
	if profile.GreetingID != item.ID {
		t.Fatal("legacy greeting compatibility lost")
	}
}

func TestCompanionInitiativePreferenceRevisionAndForgetRejectLateGeneration(t *testing.T) {
	for _, action := range []string{"less", "off", "forget"} {
		t.Run(action, func(t *testing.T) {
			s, err := NewStore(testDB(t))
			if err != nil {
				t.Fatal(err)
			}
			_, _ = s.Ensure(t.Context(), 1)
			pace, _ := s.InitiativeState(t.Context(), 1)
			token, _ := s.Acquire(t.Context(), 1)
			defer s.Release(1, token)
			now := time.Now()
			item := domain.Initiative{ID: "one", UserID: 1, Kind: "home", VisitID: "visit-123", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
			if err := s.ReserveInitiative(t.Context(), item, token); err != nil {
				t.Fatal(err)
			}
			if action == "forget" {
				err = s.Forget(t.Context(), 1, 20, "")
			} else {
				err = s.SetProactivity(t.Context(), 1, action)
			}
			if err != nil {
				t.Fatal(err)
			}
			item.Text, item.AcceptedAt = "旧记忆生成的文案", now.Add(time.Second)
			if err := s.CompleteInitiative(t.Context(), item); err != nil {
				t.Fatal(err)
			}
			if err := s.AcceptInitiative(t.Context(), item, token, *pace); !errors.Is(err, ErrBusy) {
				t.Fatal("old candidate survived preference/forget revision")
			}
			items, _ := s.Initiatives(t.Context(), 1, 0)
			if len(items) != 0 {
				t.Fatal("stale text published")
			}
		})
	}
}

func TestCompanionPreferencesDoNotCreateStateWithoutAProfile(t *testing.T) {
	db := testDB(t)
	s, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetProactivity(t.Context(), 42, "less"); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing profile accepted preferences")
	}
	var count int64
	if err := db.Model(&initiativeStateRecord{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed preferences created orphan state")
	}
}
