package companion

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	chat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	domainchat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	Store *Store
	Chat  *chat.Service
}

type State struct {
	Name                 string    `json:"name"`
	Model                string    `json:"model"`
	ConversationPublicID string    `json:"conversationPublicID"`
	Quiet                bool      `json:"quiet"`
	Greeting             string    `json:"greeting"`
	GreetingID           string    `json:"greetingID"`
	GreetingAt           time.Time `json:"greetingAt"`
	GreetingOffered      bool      `json:"greetingOffered"`
	Memories             []Memory  `json:"memories"`
}

func (s *Service) State(ctx context.Context, userID uint) (*State, error) {
	p, err := s.Store.Profile(ctx, userID)
	if err != nil {
		return nil, err
	}
	memories, err := s.Store.Memories(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &State{Name: "小伴", Model: p.Model, ConversationPublicID: p.ConversationPublicID, Quiet: p.Quiet,
		Greeting: p.LastGreeting, GreetingID: p.GreetingID, GreetingAt: p.GreetingAt, Memories: memories}, nil
}

func (s *Service) Open(ctx context.Context, userID uint, model string, allowGreeting bool) (*State, error) {
	if strings.TrimSpace(model) == "" || len(model) > 200 {
		return nil, ErrInvalid
	}
	if _, err := s.Store.Ensure(ctx, userID); err != nil {
		return nil, err
	}
	token, err := s.Store.Acquire(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer s.Store.Release(userID, token)
	p, err := s.Store.Profile(ctx, userID)
	if err != nil {
		return nil, err
	}
	if p.ConversationID != 0 {
		_, err = s.Chat.GetConversationByPublicID(ctx, userID, p.ConversationPublicID)
		if errors.Is(err, chat.ErrConversationNotFound) {
			p.ConversationID = 0
		} else if err != nil {
			return nil, err
		}
	}
	if p.ConversationID == 0 {
		conversation, createErr := s.Chat.CreateConversation(ctx, userID, "和小伴聊聊", model, "")
		if createErr != nil {
			return nil, createErr
		}
		p.ConversationID, p.ConversationPublicID, p.Model = conversation.ID, conversation.PublicID, model
		if err := s.Store.db.WithContext(ctx).Model(&Profile{}).Where("user_id = ? AND lease_token = ?", userID, token).Updates(map[string]interface{}{
			"conversation_id": p.ConversationID, "conversation_public_id": p.ConversationPublicID, "model": model,
		}).Error; err != nil {
			return nil, err
		}
	}
	if p.Model != model {
		if err := s.Store.db.WithContext(ctx).Model(&Profile{}).Where("user_id = ? AND lease_token = ?", userID, token).Update("model", model).Error; err != nil {
			return nil, err
		}
		p.Model = model
	}
	messages, err := s.Chat.CompanionMessages(ctx, userID, p.ConversationID, 64)
	if err != nil {
		return nil, err
	}
	latestUser, latestMessage := messageBoundary(messages)
	p.LeaseUntil = 0 // this request owns the lease
	now := time.Now()
	offered := allowGreeting && latestMessage <= p.ReadThroughID && CanGreet(p, latestUser, now)
	if offered {
		greeting := Greeting(p, now)
		if err := s.Store.db.WithContext(ctx).Model(&Profile{}).Where("user_id = ? AND lease_token = ?", userID, token).Updates(map[string]interface{}{
			"last_greeting": greeting, "greeting_id": uuid.NewString(), "greeting_at": now, "greeting_after_user_id": latestUser,
		}).Error; err != nil {
			return nil, err
		}
	}
	state, err := s.State(ctx, userID)
	if state != nil {
		state.GreetingOffered = offered
	}
	return state, err
}

func messageBoundary(messages []domainchat.Message) (latestUser, latest uint) {
	for _, m := range messages {
		if m.ID > latest {
			latest = m.ID
		}
		if m.Role == "user" && m.Status != "blocked" && m.ID > latestUser {
			latestUser = m.ID
		}
	}
	return
}

func (s *Service) Conversation(ctx context.Context, userID uint, publicID, query string) (*chat.Service, *Profile, error) {
	p, err := s.Store.Profile(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	if publicID == "" || p.ConversationPublicID != publicID {
		return nil, nil, ErrNotFound
	}
	memories, err := s.Store.Memories(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	return s.Chat.ForCompanion(p.ConversationID, userID, p.ForgetThroughID, BuildPrompt(p, memories, time.Now(), query)), p, nil
}

func (s *Service) SetQuiet(ctx context.Context, userID uint, quiet bool) error {
	result := s.Store.db.WithContext(ctx).Model(&Profile{}).Where("user_id = ?", userID).Update("quiet", quiet)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) MarkRead(ctx context.Context, userID, messageID uint) error {
	p, err := s.Store.Profile(ctx, userID)
	if err != nil {
		return err
	}
	items, err := s.Chat.CompanionMessages(ctx, userID, p.ConversationID, 1)
	if err != nil {
		return err
	}
	_, latest := messageBoundary(items)
	if messageID == 0 || messageID > latest {
		return ErrInvalid
	}
	return s.Store.db.WithContext(ctx).Model(&Profile{}).Where("user_id = ? AND read_through_id < ?", userID, messageID).Update("read_through_id", messageID).Error
}

func (s *Service) Forget(ctx context.Context, userID uint, memoryID string) error {
	return s.mutateMemory(ctx, userID, memoryID, nil)
}

func (s *Service) EditMemory(ctx context.Context, userID uint, memoryID, value string) error {
	value = strings.TrimSpace(value)
	if memoryID == "" || value == "" || len([]rune(value)) > 120 {
		return ErrInvalid
	}
	return s.mutateMemory(ctx, userID, memoryID, &value)
}

func (s *Service) mutateMemory(ctx context.Context, userID uint, memoryID string, value *string) error {
	token, err := s.Store.Acquire(ctx, userID)
	if err != nil {
		return err
	}
	defer s.Store.Release(userID, token)
	p, err := s.Store.Profile(ctx, userID)
	if err != nil {
		return err
	}
	items, err := s.Chat.CompanionMessages(ctx, userID, p.ConversationID, 1)
	if err != nil && !errors.Is(err, chat.ErrConversationNotFound) {
		return err
	}
	_, through := messageBoundary(items)
	if through < p.ForgetThroughID {
		through = p.ForgetThroughID
	}
	if value != nil {
		return s.Store.Edit(ctx, userID, through, memoryID, *value)
	}
	return s.Store.Forget(ctx, userID, through, memoryID)
}

type extractedFact struct {
	Key             string `json:"key"`
	Value           string `json:"value"`
	Evidence        string `json:"evidence"`
	SourceMessageID uint   `json:"sourceMessageID"`
	Days            int    `json:"days"`
}
type extractedMemory struct {
	Summary  string          `json:"summary"`
	Memories []extractedFact `json:"memories"`
}
type extractionMessage struct {
	ID   uint   `json:"id"`
	Role string `json:"role"`
	Text string `json:"text"`
}

// Refresh is called after a successful chat, never by a notification scheduler.
// A DB claim bounds platform-paid extraction to one attempt / 5 min / user.
// Its failure never makes an already-completed user reply fail.
func (s *Service) Refresh(ctx context.Context, userID uint) error {
	p, err := s.Store.Profile(ctx, userID)
	if err != nil {
		return err
	}
	afterID := p.ThroughID
	if p.ForgetThroughID > afterID {
		afterID = p.ForgetThroughID
	}
	items, err := s.Chat.CompanionMemoryBatch(ctx, userID, p.ConversationID, afterID)
	if err != nil {
		return err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	// Keep an incomplete final user turn for the next batch.
	for len(items) > 0 && items[len(items)-1].Role != "assistant" {
		items = items[:len(items)-1]
	}
	messages := []extractionMessage{}
	sources := map[uint]string{}
	var through uint
	userTurns := 0
	for _, m := range items {
		if m.ID <= p.ThroughID || m.ID <= p.ForgetThroughID || m.Status != "success" {
			continue
		}
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		messages = append(messages, extractionMessage{ID: m.ID, Role: m.Role, Text: clip(m.Content, 800)})
		if m.Role == "user" {
			sources[m.ID] = m.Content
			userTurns++
		}
		if m.ID > through {
			through = m.ID
		}
	}
	if userTurns < 4 || len(messages) == 0 || messages[len(messages)-1].Role != "assistant" {
		return nil
	}
	token := uuid.NewString()
	claim := s.Store.db.WithContext(ctx).Model(&Profile{}).Where("user_id = ? AND revision = ? AND refresh_after < ?", userID, p.Revision, time.Now().Unix()).Updates(map[string]interface{}{
		"refresh_token": token, "refresh_after": time.Now().Add(5 * time.Minute).Unix(),
	})
	if claim.Error != nil {
		return claim.Error
	}
	if claim.RowsAffected == 0 {
		return nil
	}
	if time.Since(p.SummaryAt) > 30*24*time.Hour {
		p.Summary = ""
	}
	result, err := s.Chat.StreamTemporaryChat(ctx, chat.TemporaryChatInput{
		UserID: userID, SessionID: "companion-memory", ClientRunID: "companion-memory-" + token,
		RequestID: token, Model: p.Model, Options: map[string]interface{}{"max_tokens": 3000},
		Messages: []chat.TemporaryChatMessage{{Role: "user", Content: extractionPrompt(p, messages)}},
	}, nil)
	if err != nil {
		return err
	}
	if result == nil || result.IsModerationBlocked() {
		return nil
	}
	var extracted extractedMemory
	text := strings.TrimSpace(result.AssistantMessage.Content)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	}
	if len(text) > 32_000 || json.Unmarshal([]byte(text), &extracted) != nil {
		return ErrInvalid
	}
	return s.saveExtraction(ctx, p, token, through, extracted, sources)
}

func validatedFacts(userID uint, extracted []extractedFact, sources map[uint]string, now time.Time) []Memory {
	items := []Memory{}
	seen := map[string]bool{}
	for _, fact := range extracted {
		key, value, evidence := clip(fact.Key, 60), strings.TrimSpace(fact.Value), strings.TrimSpace(fact.Evidence)
		if key == "" || seen[key] || value == "" || len([]rune(value)) > 120 || len([]rune(evidence)) < 4 || len([]rune(evidence)) > 300 {
			continue
		}
		source, ok := sources[fact.SourceMessageID]
		if !ok || !strings.Contains(source, evidence) {
			continue
		}
		days := fact.Days
		if days < 1 || days > 90 {
			days = 7
		}
		items = append(items, Memory{ID: uuid.NewString(), UserID: userID, Key: key, Value: value, Evidence: evidence,
			SourceMessageID: fact.SourceMessageID, ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour), UpdatedAt: now})
		seen[key] = true
		if len(items) == 8 {
			break
		}
	}
	return items
}

func (s *Service) saveExtraction(ctx context.Context, p *Profile, token string, through uint, extracted extractedMemory, sources map[uint]string) error {
	now := time.Now()
	return s.Store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&Profile{}).Where("user_id = ? AND revision = ? AND refresh_token = ?", p.UserID, p.Revision, token).Updates(map[string]interface{}{
			"summary": clip(extracted.Summary, 1800), "summary_at": now, "through_id": through, "refresh_token": "",
		})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return nil
		}
		if err := tx.Where("user_id = ? AND expires_at <= ?", p.UserID, now).Delete(&Memory{}).Error; err != nil {
			return err
		}
		for _, memory := range validatedFacts(p.UserID, extracted.Memories, sources, now) {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "evidence", "source_message_id", "expires_at", "updated_at"})}).Create(&memory).Error; err != nil {
				return err
			}
		}
		var obsolete []string
		if err := tx.Model(&Memory{}).Where("user_id = ?", p.UserID).Order("updated_at DESC, id ASC").Offset(60).Limit(16).Pluck("id", &obsolete).Error; err != nil {
			return err
		}
		if len(obsolete) > 0 {
			return tx.Where("user_id = ? AND id IN ?", p.UserID, obsolete).Delete(&Memory{}).Error
		}
		return nil
	})
}
