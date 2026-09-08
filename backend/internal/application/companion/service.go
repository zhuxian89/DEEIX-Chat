package companion

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	chat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	domainchat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/google/uuid"
)

type Service struct {
	Store         Store
	Chat          *chat.Service
	TopicProvider TopicProvider
}

type State struct {
	Name                 string
	Model                string
	ConversationPublicID string
	Quiet                bool
	Greeting             string
	GreetingID           string
	GreetingAt           time.Time
	GreetingOffered      bool
	Memories             []Memory
	Topic                *Topic
	Proactivity          string
	InitiativeVersion    int
	Initiatives          []Initiative
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
	pace, err := s.Store.InitiativeState(ctx, userID)
	if err != nil {
		return nil, err
	}
	initiatives, err := s.Store.Initiatives(ctx, userID, p.ConversationID)
	if err != nil {
		return nil, err
	}
	return &State{Name: Name, Model: p.Model, ConversationPublicID: p.ConversationPublicID, Quiet: p.Quiet,
		Greeting: currentGreeting(p.LastGreeting), GreetingID: p.GreetingID, GreetingAt: p.GreetingAt, Memories: memories, Topic: storedTopic(p.LastTopicJSON),
		Proactivity: proactivityMode(p, pace), InitiativeVersion: 1, Initiatives: initiatives}, nil
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
		conversation, createErr := s.Chat.CreateConversation(ctx, userID, "和"+Name+"聊聊", model, "")
		if createErr != nil {
			return nil, createErr
		}
		p.ConversationID, p.ConversationPublicID, p.Model = conversation.ID, conversation.PublicID, model
		if err := s.Store.SaveConversation(ctx, p, token); err != nil {
			return nil, err
		}
	}
	if p.Model != model {
		if err := s.Store.UpdateModel(ctx, userID, token, model); err != nil {
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
		updates := domain.GreetingUpdate{Text: greeting, ID: uuid.NewString(), At: now, AfterUserID: latestUser}
		if topic := s.pickGreetingTopic(ctx, p, messages, now); topic != nil {
			updates.Text = timeGreeting(now) + topic.Opener
			encoded, _ := json.Marshal(topicData(*topic))
			updates.TopicJSON = string(encoded)
			updates.TopicAt = now
			updates.SeenTopicsJSON = rememberTopic(p.SeenTopicsJSON, topic.URL)
		}
		if err := s.Store.SaveGreeting(ctx, userID, token, updates); err != nil {
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
	now := time.Now()
	items, err := s.Chat.CompanionMessages(ctx, userID, p.ConversationID, 24)
	if err != nil {
		return nil, nil, err
	}
	lastUser := lastUserMessage(items, p.ForgetThroughID)
	var topics []Topic
	if s.TopicProvider != nil {
		topics, _ = s.topics(ctx, now)
	}
	// The topic the user is replying to remains sourceable even after it was seen.
	if topic := storedTopic(p.LastTopicJSON); topic != nil && freshTopic(*topic, now) {
		topics = append([]Topic{*topic}, topics...)
	}
	topics = rankTopics(topics, memories, p, now, query)
	if len(topics) > 2 {
		topics = topics[:2]
	}
	prompt := BuildPrompt(p, memories, now, query) + continuityPrompt(now, lastUser.CreatedAt, topics) + chat.CompanionHistoryTimePrompt(items, p.ForgetThroughID)
	initiatives, err := s.Store.Initiatives(ctx, userID, p.ConversationID)
	if err != nil {
		return nil, nil, err
	}
	prompt += initiativeContext(initiatives, now)
	return s.Chat.ForCompanion(p.ConversationID, userID, p.ForgetThroughID, prompt), p, nil
}

func (s *Service) SetQuiet(ctx context.Context, userID uint, quiet bool) error {
	mode := "normal"
	if quiet {
		mode = "off"
	}
	return s.SetProactivity(ctx, userID, mode)
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
	return s.Store.MarkRead(ctx, userID, messageID)
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
	ID        uint   `json:"id"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	AtBeijing string `json:"at_beijing"`
}

type extractionSource struct {
	Text string
	At   time.Time
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
	sources := map[uint]extractionSource{}
	var through uint
	userTurns := 0
	for _, m := range items {
		if m.ID <= p.ThroughID || m.ID <= p.ForgetThroughID || m.Status != "success" {
			continue
		}
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		messages = append(messages, extractionMessage{ID: m.ID, Role: m.Role, Text: clip(m.Content, 800), AtBeijing: beijingTimestamp(m.CreatedAt)})
		if m.Role == "user" {
			sources[m.ID] = extractionSource{Text: m.Content, At: m.CreatedAt}
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
	claimed, err := s.Store.ClaimRefresh(ctx, userID, p.Revision, token, time.Now())
	if err != nil || !claimed {
		return err
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

func validatedFacts(userID uint, extracted []extractedFact, sources map[uint]extractionSource, now time.Time) []Memory {
	items := []Memory{}
	seen := map[string]bool{}
	for _, fact := range extracted {
		key, value, evidence := clip(fact.Key, 60), strings.TrimSpace(fact.Value), strings.TrimSpace(fact.Evidence)
		if key == "" || strings.HasPrefix(key, "topic:") || seen[key] || value == "" || len([]rune(value)) > 120 || len([]rune(evidence)) < 4 || len([]rune(evidence)) > 300 {
			continue
		}
		source, ok := sources[fact.SourceMessageID]
		if !ok || !strings.Contains(source.Text, evidence) {
			continue
		}
		days := fact.Days
		if days < 1 || days > 90 {
			days = 7
		}
		items = append(items, Memory{ID: uuid.NewString(), UserID: userID, Key: key, Value: value, Evidence: evidence,
			SourceMessageID: fact.SourceMessageID, SourceAt: source.At, ExpiresAt: now.Add(time.Duration(days) * 24 * time.Hour), UpdatedAt: now})
		seen[key] = true
		if len(items) == 8 {
			break
		}
	}
	return items
}

func (s *Service) saveExtraction(ctx context.Context, p *Profile, token string, through uint, extracted extractedMemory, sources map[uint]extractionSource) error {
	now := time.Now()
	return s.Store.SaveExtraction(ctx, domain.Extraction{
		UserID: p.UserID, Revision: p.Revision, Token: token, ThroughID: through,
		Summary: clip(extracted.Summary, 1800), At: now,
		Memories: validatedFacts(p.UserID, extracted.Memories, sources, now),
	})
}
