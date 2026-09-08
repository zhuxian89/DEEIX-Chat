package companion

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	chat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	domainchat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"go.uber.org/zap"
)

func initiativeTurns(now time.Time) []domainchat.Message {
	return []domainchat.Message{
		{ID: 11, Role: "user", Content: "最近在学摄影", Status: "success", CreatedAt: now.Add(-2 * time.Minute)},
		{ID: 12, Role: "assistant", Content: "可以先试着观察窗边的自然光。", Status: "success", CreatedAt: now.Add(-time.Minute)},
	}
}

func TestCompanionIdleInitiativeRespectsConversationAndPacing(t *testing.T) {
	now := time.Date(2026, 9, 8, 2, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		change func(*Profile, *domain.InitiativeState, []domainchat.Message)
	}{
		{"quiet", func(p *Profile, _ *domain.InitiativeState, _ []domainchat.Message) { p.Quiet = true }},
		{"lease", func(p *Profile, _ *domain.InitiativeState, _ []domainchat.Message) {
			p.LeaseUntil = now.Add(time.Minute).Unix()
		}},
		{"unread", func(p *Profile, _ *domain.InitiativeState, _ []domainchat.Message) { p.ReadThroughID = 11 }},
		{"forgotten", func(p *Profile, _ *domain.InitiativeState, _ []domainchat.Message) { p.ForgetThroughID = 12 }},
		{"same visit", func(_ *Profile, s *domain.InitiativeState, _ []domainchat.Message) { s.IdleVisitID = "visit-123" }},
		{"same turn new device", func(_ *Profile, s *domain.InitiativeState, _ []domainchat.Message) { s.IdleAfterMessageID = 12 }},
		{"too soon", func(_ *Profile, _ *domain.InitiativeState, m []domainchat.Message) {
			m[1].CreatedAt = now.Add(-44 * time.Second)
		}},
		{"old chat", func(_ *Profile, _ *domain.InitiativeState, m []domainchat.Message) {
			m[1].CreatedAt = now.Add(-31 * time.Minute)
		}},
		{"waiting for answer", func(_ *Profile, _ *domain.InitiativeState, m []domainchat.Message) {
			m[1].Content = "你想先拍什么？"
		}},
		{"question without punctuation", func(_ *Profile, _ *domain.InitiativeState, m []domainchat.Message) {
			m[1].Content = "要不要试着拍一张"
		}},
		{"closing", func(_ *Profile, _ *domain.InitiativeState, m []domainchat.Message) {
			m[0].Content = "我先去忙，晚点聊"
		}},
		{"pending", func(_ *Profile, _ *domain.InitiativeState, m []domainchat.Message) { m[1].Status = "pending" }},
		{"blocked", func(_ *Profile, _ *domain.InitiativeState, m []domainchat.Message) { m[1].Status = "blocked" }},
		{"off", func(_ *Profile, s *domain.InitiativeState, _ []domainchat.Message) { s.Mode = "off" }},
		{"less frequent", func(_ *Profile, s *domain.InitiativeState, _ []domainchat.Message) {
			s.Mode = "less"
			s.LastIdleAt = now.Add(-30 * time.Minute)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, pace, items := &Profile{ReadThroughID: 12}, &domain.InitiativeState{}, initiativeTurns(now)
			if !initiativeAllowed(p, pace, "idle", "visit-123", 12, items, now, false) {
				t.Fatal("eligible pause rejected")
			}
			tc.change(p, pace, items)
			if initiativeAllowed(p, pace, "idle", "visit-123", 12, items, now, false) {
				t.Fatal("inappropriate interruption allowed")
			}
		})
	}
}

func TestCompanionHomeInitiativeUsesBeijingLimitsAndUnansweredBackoff(t *testing.T) {
	now := time.Date(2026, 9, 8, 16, 1, 0, 0, time.UTC) // Sep 9 in Beijing.
	p := &Profile{ReadThroughID: 12}
	items := initiativeTurns(now.Add(-48 * time.Hour))
	pace := &domain.InitiativeState{HomeDay: "2026-09-09", HomeCount: 2, LastHomeAt: now.Add(-13 * time.Hour), HomeAfterUserID: 10}
	if initiativeAllowed(p, pace, "home", "visit-123", 0, items, now, false) {
		t.Fatal("Beijing daily limit bypassed")
	}
	pace.HomeDay = "2026-09-08"
	if !initiativeAllowed(p, pace, "home", "visit-123", 0, items, now, false) {
		t.Fatal("new Beijing day did not reset cap")
	}
	pace.HomeAfterUserID, pace.UnansweredHomes, pace.LastHomeAt = 11, 3, now.Add(-25*time.Hour)
	if initiativeAllowed(p, pace, "home", "visit-123", 0, items, now, false) {
		t.Fatal("ignored greetings did not back off")
	}
	pace.LastHomeAt = now.Add(-49 * time.Hour)
	if !initiativeAllowed(p, pace, "home", "visit-123", 0, items, now, false) {
		t.Fatal("home card remained permanently silent after ignoring")
	}
	pace.LastHomeAt, pace.HomeAfterUserID = now.Add(-13*time.Hour), 10
	if !initiativeAllowed(p, pace, "home", "visit-123", 0, items, now, false) {
		t.Fatal("a reply did not restore ordinary cadence")
	}
	pace.Mode = "less"
	if initiativeAllowed(p, pace, "home", "visit-123", 0, items, now, false) {
		t.Fatal("less proactive mode ignored")
	}
}

func TestCompanionInitiativeRequiresEvidenceAndAllowsSilence(t *testing.T) {
	sources := []initiativeSource{{ID: "memory:one", Text: "用户喜欢摄影，想学自然光人像。"}}
	text, _, err := validatedInitiative(`{"speak":true,"text":"拍自然光人像时，可以先试试侧着面对窗户。","source_id":"memory:one","evidence":"想学自然光人像"}`, sources, nil)
	if err != nil || text == "" {
		t.Fatalf("grounded candidate rejected: %v", err)
	}
	if next, _, err := validatedInitiative(`{"speak":false}`, sources, nil); err != nil || next != "" {
		t.Fatal("silence was not respected")
	}
	for _, raw := range []string{
		`{"speak":true,"text":"你终于来了","source_id":"memory:one","evidence":"想学自然光人像"}`,
		`{"speak":true,"text":"你面试成功了","source_id":"memory:one","evidence":"面试成功"}`,
		`{"speak":true,"text":"聊摄影吧","source_id":"other-user","evidence":"想学自然光人像"}`,
		`{"speak":true,"text":"拍了什么？效果如何？","source_id":"memory:one","evidence":"想学自然光人像"}`,
	} {
		if text, _, err := validatedInitiative(raw, sources, nil); err == nil || text != "" {
			t.Fatalf("invalid candidate accepted: %s", raw)
		}
	}
}

type initiativeChatRepo struct {
	repository.ConversationRepository
	items []domainchat.Message
}

func (r *initiativeChatRepo) GetConversationByUser(_ context.Context, conversationID, userID uint) (*domainchat.Conversation, error) {
	if conversationID != 7 || userID != 1 {
		return nil, repository.ErrNotFound
	}
	return &domainchat.Conversation{ID: 7, UserID: 1, PublicID: "companion-chat"}, nil
}

func (r *initiativeChatRepo) ListRecentMessages(_ context.Context, _ uint, _ int) ([]domainchat.Message, int64, error) {
	return r.items, int64(len(r.items)), nil
}

func initiativeService(t *testing.T) (*Service, *testRepository, *initiativeChatRepo) {
	t.Helper()
	store := testStore(t)
	_, _ = store.Ensure(t.Context(), 1)
	store.db.Table("companion_profiles").Where("user_id = ?", 1).Updates(map[string]interface{}{"conversation_id": 7, "conversation_public_id": "companion-chat", "read_through_id": 12})
	repo := &initiativeChatRepo{}
	service := chat.NewService(config.Config{}, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop())
	return &Service{Store: store, Chat: service}, store, repo
}

func TestCompanionCandidateIsInvisibleUntilAcceptedAndAcceptIsIdempotent(t *testing.T) {
	s, store, _ := initiativeService(t)
	candidate, err := s.PrepareInitiative(t.Context(), 1, "home", "visit-123", 0)
	if err != nil || candidate == nil {
		t.Fatalf("first greeting: %v", err)
	}
	state, _ := s.State(t.Context(), 1)
	if len(state.Initiatives) != 0 {
		t.Fatal("unaccepted candidate leaked into history")
	}
	for i := 0; i < 2; i++ {
		accepted, err := s.AcceptInitiative(t.Context(), 1, candidate.ID, "visit-123")
		if err != nil || accepted == nil || accepted.AcceptedAt.IsZero() {
			t.Fatalf("accept failed: %v", err)
		}
	}
	pace, _ := store.InitiativeState(t.Context(), 1)
	state, _ = s.State(t.Context(), 1)
	if pace.HomeCount != 1 || len(state.Initiatives) != 1 || state.GreetingID != candidate.ID {
		t.Fatal("acceptance duplicated pacing/history")
	}
	if _, err := store.Initiative(t.Context(), 2, candidate.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-user candidate visible")
	}
	if next, err := s.PrepareInitiative(t.Context(), 1, "home", "visit-456", 0); err != nil || next != nil {
		t.Fatal("reentry repeated greeting")
	}
}

func TestCompanionParallelDevicesReserveOnlyOneCandidate(t *testing.T) {
	s, store, _ := initiativeService(t)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			if _, err := s.PrepareInitiative(t.Context(), 1, "home", fmt.Sprintf("visit-%03d", i), 0); err != nil {
				t.Error(err)
			}
		}(i)
	}
	workers.Wait()
	var count int64
	if err := store.db.Table("companion_initiatives").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("reserved %d candidates across devices", count)
	}
	token, err := store.Acquire(t.Context(), 1)
	if err != nil {
		t.Fatalf("candidate generation held the chat lease: %v", err)
	}
	store.Release(1, token)
}

func TestCompanionRejectsCandidateAfterNewTurnPreferencesOrForget(t *testing.T) {
	for _, action := range []string{"new message", "off", "forget", "expired", "another visit"} {
		t.Run(action, func(t *testing.T) {
			s, store, repo := initiativeService(t)
			candidate, err := s.PrepareInitiative(t.Context(), 1, "home", "visit-123", 0)
			if err != nil || candidate == nil {
				t.Fatal(err)
			}
			visit := "visit-123"
			switch action {
			case "new message":
				repo.items = initiativeTurns(time.Now())
			case "off":
				if err := s.SetProactivity(t.Context(), 1, "off"); err != nil {
					t.Fatal(err)
				}
			case "forget":
				if err := store.Forget(t.Context(), 1, 0, ""); err != nil {
					t.Fatal(err)
				}
			case "expired":
				store.db.Table("companion_initiatives").Where("id = ?", candidate.ID).Update("expires_at", time.Now().Add(-time.Second))
			case "another visit":
				visit = "different-visit"
			}
			accepted, _ := s.AcceptInitiative(t.Context(), 1, candidate.ID, visit)
			if accepted != nil {
				t.Fatal("stale candidate published")
			}
			state, _ := s.State(t.Context(), 1)
			if len(state.Initiatives) != 0 {
				t.Fatal("stale candidate leaked into history")
			}
		})
	}
}
