package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"go.uber.org/zap"
)

type companionRepoFixture struct {
	branchContextRepositoryStub
	conversation model.Conversation
}

func (r *companionRepoFixture) GetConversationByUser(_ context.Context, id, userID uint) (*model.Conversation, error) {
	if id != r.conversation.ID || userID != r.conversation.UserID {
		return nil, repository.ErrNotFound
	}
	return &r.conversation, nil
}

func TestCompanionFacadeBoundsLongHistoryWithoutChangingStoredConversation(t *testing.T) {
	repo := &companionRepoFixture{branchContextRepositoryStub: branchContextRepositoryStub{messages: buildBranchContextMessages(600)}, conversation: model.Conversation{ID: 7, UserID: 9, ProjectSystemPrompt: "ordinary", LastResponseID: "provider-old"}}
	base := &Service{cfg: config.NewRuntime(config.Config{ContextCompactEnabled: true, SemanticContextEnabled: true}), repo: repo, logger: zap.NewNop()}
	companion := base.ForCompanion(7, 9, 590, "fixed persona + private memory")
	leaf := uint(600)
	branch := &messageBranchState{ParentMessageID: &leaf}
	if err := companion.loadMessageBranchContext(t.Context(), 7, branch, nil, "default"); err != nil {
		t.Fatal(err)
	}
	if repo.calls != 1 || len(branch.ExistingMessages) != 10 || branch.ExistingMessages[0].ID != 591 {
		t.Fatalf("unbounded or forgotten history: calls=%d messages=%v", repo.calls, branch.ExistingMessages)
	}
	item, err := companion.repo.GetConversationByUser(t.Context(), 7, 9)
	if err != nil || item.ProjectSystemPrompt != "fixed persona + private memory" || item.LastResponseID != "" {
		t.Fatalf("persona not injected: %+v %v", item, err)
	}
	if repo.conversation.ProjectSystemPrompt != "ordinary" || repo.conversation.LastResponseID != "provider-old" {
		t.Fatal("mutated original conversation")
	}
	if companion.cfg.Snapshot().ContextCompactEnabled || companion.cfg.Snapshot().SemanticContextEnabled || !base.cfg.Snapshot().ContextCompactEnabled {
		t.Fatal("mutated normal-chat configuration")
	}
	if _, err := companion.repo.GetConversationByUser(t.Context(), 7, 10); !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("cross-user context exposed")
	}
	if snapshot, err := companion.repo.GetLatestContextSnapshot(t.Context(), 7); snapshot != nil || !errors.Is(err, repository.ErrNotFound) {
		t.Fatal("old snapshot was recalled")
	}
}

func TestCompanionHistoryStopsAt24Messages(t *testing.T) {
	repo := &companionRepoFixture{branchContextRepositoryStub: branchContextRepositoryStub{messages: buildBranchContextMessages(600)}}
	wrapped := &companionConversationRepository{ConversationRepository: repo, conversationID: 7}
	items, err := wrapped.ListMessageAncestors(t.Context(), 7, 600, 256)
	if err != nil || len(items) != CompanionRecentMessageLimit || items[0].ParentMessageID != nil {
		t.Fatalf("history not bounded: %d %v", len(items), err)
	}
	if repo.messages[576].ParentMessageID == nil {
		t.Fatal("stored ancestry was modified")
	}
}

func TestCompanionHistoryKeepsDatesOutOfDialogueText(t *testing.T) {
	repo := &companionRepoFixture{branchContextRepositoryStub: branchContextRepositoryStub{messages: buildBranchContextMessages(3)}}
	at := time.Date(2026, 9, 7, 16, 5, 0, 0, time.UTC)
	for i := range repo.messages {
		repo.messages[i].CreatedAt = at
		repo.messages[i].Content = "我明天要面试"
	}
	wrapped := &companionConversationRepository{ConversationRepository: repo, conversationID: 7}
	for attempt := 0; attempt < 2; attempt++ {
		items, err := wrapped.ListMessageAncestors(t.Context(), 7, 3, 24)
		if err != nil || len(items) != 3 {
			t.Fatalf("history error: %v", err)
		}
		for _, item := range items {
			if item.Content != "我明天要面试" || !item.CreatedAt.Equal(at) {
				t.Fatalf("history body or source timestamp changed: %+v", item)
			}
		}
	}
	if repo.messages[0].Content != "我明天要面试" {
		t.Fatal("stored text changed")
	}
}

func TestCompanionHistoryDoesNotRepeatLeakedAssistantTimeMarkers(t *testing.T) {
	const leaked = "[历史消息时间：北京时间 2026-09-08 07:17:21] “不认真”这个标签背后"
	repo := &companionRepoFixture{branchContextRepositoryStub: branchContextRepositoryStub{messages: buildBranchContextMessages(2)}}
	repo.messages[0].Role, repo.messages[0].Content = "user", leaked
	repo.messages[1].Role, repo.messages[1].Content = "assistant", leaked
	wrapped := &companionConversationRepository{ConversationRepository: repo, conversationID: 7}
	items, err := wrapped.ListMessageAncestors(t.Context(), 7, 2, 24)
	if err != nil || len(items) != 2 {
		t.Fatalf("history error: %v", err)
	}
	if items[0].Content != leaked || items[1].Content != "“不认真”这个标签背后" {
		t.Fatalf("assistant marker was retained or user quotation was changed: %+v", items)
	}
	if repo.messages[1].Content != leaked {
		t.Fatal("stored assistant message was rewritten")
	}
}

func TestCompanionHistoryTimePromptPreservesDatesAndMemoryBoundary(t *testing.T) {
	at := time.Date(2026, 9, 7, 16, 5, 0, 0, time.UTC)
	messages := []model.Message{
		{ID: 1, Role: "user", Content: "forgotten", CreatedAt: at},
		{ID: 2, Role: "user", Content: "blocked", Status: "blocked", CreatedAt: at},
		{ID: 3, Role: "user", Content: "undated"},
		{ID: 4, Role: "tool", Content: "tool output", CreatedAt: at},
		{ID: 5, Role: "user", Content: "我明天要面试", CreatedAt: at},
		{ID: 6, Role: "assistant", Content: "[历史消息时间：北京时间 2026-09-08 00:05:00]\n想聊聊面试吗？", CreatedAt: at.Add(time.Minute)},
		{ID: 7, Role: "user", Content: strings.Repeat("聊", 140), CreatedAt: at},
	}
	prompt := CompanionHistoryTimePrompt(messages, 1)
	raw := strings.TrimSuffix(strings.TrimPrefix(prompt, "\n<history_message_times>\n"), "\n</history_message_times>")
	var entries []companionHistoryTime
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Role != "user" || entries[0].TextExcerpt != "我明天要面试" || entries[0].AtBeijing != "2026-09-08T00:05:00+08:00" || entries[1].TextExcerpt != "想聊聊面试吗？" || entries[1].AtBeijing != "2026-09-08T00:06:00+08:00" || len([]rune(entries[2].TextExcerpt)) != 120 {
		t.Fatalf("incorrect dated context: %+v", entries)
	}
	if CompanionHistoryTimePrompt(messages, 7) != "" {
		t.Fatal("forgotten messages leaked through the timestamp index")
	}
	longHistory := make([]model.Message, CompanionRecentMessageLimit+10)
	for i := range longHistory {
		longHistory[i] = model.Message{ID: uint(i + 1), Role: "user", CreatedAt: at}
	}
	if count := strings.Count(CompanionHistoryTimePrompt(longHistory, 0), `"at_beijing"`); count != CompanionRecentMessageLimit {
		t.Fatalf("unbounded timestamp index: %d", count)
	}
}
