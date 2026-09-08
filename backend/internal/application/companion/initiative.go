package companion

import (
	"context"
	"errors"
	"strings"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"github.com/google/uuid"
)

type Initiative = domain.Initiative

func (s *Service) SetProactivity(ctx context.Context, userID uint, mode string) error {
	if mode != "normal" && mode != "less" && mode != "off" {
		return ErrInvalid
	}
	return s.Store.SetProactivity(ctx, userID, mode)
}

// PrepareInitiative does not publish a message. Model work runs without the chat
// lease so that a user can start talking while a candidate is being considered.
func (s *Service) PrepareInitiative(ctx context.Context, userID uint, kind, visit string, after uint) (*Initiative, error) {
	if (kind != "home" && kind != "idle") || len(visit) < 8 || len(visit) > 64 {
		return nil, ErrInvalid
	}
	token, err := s.Store.Acquire(ctx, userID)
	if errors.Is(err, ErrBusy) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item, p, err := s.reserveInitiative(ctx, userID, token, kind, visit, after)
	s.Store.Release(userID, token)
	if err != nil || item == nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	text, topic, err := s.composeInitiative(ctx, p, item)
	if err != nil || text == "" {
		return nil, err
	}
	item.Text, item.TopicJSON = text, topic
	if err := s.Store.CompleteInitiative(ctx, *item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) reserveInitiative(ctx context.Context, userID uint, token, kind, visit string, after uint) (*Initiative, *Profile, error) {
	p, err := s.Store.Profile(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	pace, err := s.Store.InitiativeState(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	items, err := s.Chat.CompanionMessages(ctx, userID, p.ConversationID, 24)
	if err != nil {
		return nil, nil, err
	}
	p.LeaseUntil = 0
	now := time.Now()
	if !initiativeAllowed(p, pace, kind, visit, after, items, now, false) {
		return nil, nil, nil
	}
	latestUser, latest := messageBoundary(items)
	item := &Initiative{ID: uuid.NewString(), UserID: userID, ConversationID: p.ConversationID, Kind: kind, VisitID: visit,
		AfterMessageID: latest, AfterUserID: latestUser, Revision: p.Revision, CreatedAt: now, ExpiresAt: now.Add(90 * time.Second)}
	if err := s.Store.ReserveInitiative(ctx, *item, token); err != nil {
		return nil, nil, err
	}
	return item, p, nil
}

func (s *Service) AcceptInitiative(ctx context.Context, userID uint, id, visit string) (*Initiative, error) {
	if strings.TrimSpace(id) == "" || len(id) > 64 || len(visit) < 8 || len(visit) > 64 {
		return nil, ErrInvalid
	}
	token, err := s.Store.Acquire(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer s.Store.Release(userID, token)
	item, err := s.Store.Initiative(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if item.VisitID != visit {
		return nil, ErrInvalid
	}
	if !item.AcceptedAt.IsZero() {
		return item, nil
	}
	p, err := s.Store.Profile(ctx, userID)
	if err != nil {
		return nil, err
	}
	if item.Revision != p.Revision || item.ConversationID != p.ConversationID || item.Text == "" || !item.ExpiresAt.After(time.Now()) {
		return nil, nil
	}
	pace, err := s.Store.InitiativeState(ctx, userID)
	if err != nil {
		return nil, err
	}
	items, err := s.Chat.CompanionMessages(ctx, userID, p.ConversationID, 24)
	if err != nil {
		return nil, err
	}
	_, latest := messageBoundary(items)
	p.LeaseUntil = 0
	now := time.Now()
	if latest != item.AfterMessageID || !initiativeAllowed(p, pace, item.Kind, visit, latest, items, now, true) {
		return nil, nil
	}
	if item.Kind == "idle" && (pace.IdleVisitID != visit || pace.IdleAfterMessageID != latest) {
		return nil, nil
	}
	item.AcceptedAt = now
	if err := s.Store.AcceptInitiative(ctx, *item, token, acceptedPacing(*pace, *item)); err != nil {
		return nil, err
	}
	return item, nil
}
