package conversation

import (
	"context"
	"errors"
	"testing"

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
