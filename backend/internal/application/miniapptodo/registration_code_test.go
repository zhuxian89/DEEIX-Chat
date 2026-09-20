package miniapptodo

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/miniapptodo"
)

func TestRegistrationCodeFeedbackIndependentOfLegacyConfiguration(t *testing.T) {
	store := &fakeStore{grants: map[string]time.Time{}}
	called := 0
	config := Config{AppID: "app", RedeemRegistrationCode: func(ctx context.Context, owner domain.Owner, code string, now time.Time) (time.Time, error) {
		called++
		want, _ := store.ResolveOwner(ctx, 7, "app")
		if owner != want || code != "reg-test" {
			t.Fatal(owner, code)
		}
		return store.Unlock(ctx, owner, now)
	}}
	service := NewService(store, config)
	first, err := service.Feedback(t.Context(), 7, " reg-test ")
	if err != nil || !first.Unlocked || first.UnlockedAt == nil || called != 1 {
		t.Fatal(first, called, err)
	}
	if _, err := service.Feedback(t.Context(), 7, "REG-ANOTHER"); err != nil || called != 1 {
		t.Fatal("already activated identity consumed another code", called, err)
	}
	config.FeedbackCode = func(context.Context) (string, error) { return "", errors.New("settings unavailable") }
	store.grants = map[string]time.Time{}
	if status, err := NewService(store, config).Feedback(t.Context(), 7, "reg-test"); err != nil || !status.Unlocked {
		t.Fatal("registration code depends on fixed password settings", status, err)
	}
}

func TestRegistrationCodeNeverFallsBackToSharedPassword(t *testing.T) {
	for _, configured := range []bool{false, true} {
		store := &fakeStore{grants: map[string]time.Time{}}
		config := Config{AppID: "app", FeedbackCode: func(context.Context) (string, error) { return "REG-TEST", nil }}
		if configured {
			config.RedeemRegistrationCode = func(context.Context, domain.Owner, string, time.Time) (time.Time, error) {
				return time.Time{}, domain.ErrInvalidCode
			}
		}
		if _, err := NewService(store, config).Feedback(t.Context(), 7, "REG-TEST"); !errors.Is(err, domain.ErrInvalidCode) || len(store.grants) != 0 {
			t.Fatal("registration code bypassed binding", configured, err)
		}
	}
}
