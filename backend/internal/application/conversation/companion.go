package conversation

// This adapter is intentionally separate from the upstream conversation runtime.
// Companion messages use its ownership checks, moderation, billing and run recovery.
import (
	"context"
	"errors"

	appcompact "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/compact"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

const CompanionRecentMessageLimit = 24

type companionMemoryReader interface {
	ListCompanionMemoryMessages(context.Context, uint, uint, uint, int) ([]model.Message, error)
}

func (s *Service) CompanionMemoryBatch(ctx context.Context, userID, conversationID, afterID uint) ([]model.Message, error) {
	if _, err := s.repo.GetConversationByUser(ctx, conversationID, userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}
	reader, ok := s.repo.(companionMemoryReader)
	if !ok {
		return nil, errors.New("companion memory cursor is unavailable")
	}
	return reader.ListCompanionMemoryMessages(ctx, userID, conversationID, afterID, 32)
}

// ForCompanion creates a request-scoped facade; it never copies Service's locks.
// A fresh configuration snapshot retains current administrator settings while
// substituting the companion's bounded memory for ordinary memory/compaction.
func (s *Service) ForCompanion(conversationID, userID, forgetThrough uint, systemPrompt string) *Service {
	cfg := s.cfg.Snapshot()
	cfg.ContextCompactEnabled = false
	cfg.CompactAsyncEnabled = false
	cfg.SemanticContextEnabled = false
	cfg.ContextTokenBudgetEnabled = true
	runtime := config.NewRuntime(cfg)
	repo := &companionConversationRepository{
		ConversationRepository: s.repo, conversationID: conversationID,
		userID: userID, forgetThrough: forgetThrough, systemPrompt: systemPrompt, contextPolicy: buildContextPolicyJSON(cfg),
	}
	return &Service{
		cfg: runtime, repo: repo, cache: s.cache, routeResolver: s.routeResolver,
		llmClient: companionLLMGateway{s.llmClient}, mediaDownloader: s.mediaDownloader,
		mcpClient: s.mcpClient, mcpRepo: s.mcpRepo, uploadSvc: s.uploadSvc,
		compactSvc:   appcompact.NewServiceWithRuntime(runtime, repo, s.logger),
		embeddingSvc: s.embeddingSvc, processingSvc: s.processingSvc,
		extractSvc: s.extractSvc, ragSvc: s.ragSvc, billingSvc: s.billingSvc,
		auditWriter: s.auditWriter, storeProvider: s.storeProvider, logger: s.logger,
		moderationSvc: s.moderationSvc, generationStreams: s.generationStreams,
		imageContextCache: s.imageContextCache,
	}
}

// CompanionMessages returns approved conversation messages for memory extraction.
func (s *Service) CompanionMessages(ctx context.Context, userID, conversationID uint, limit int) ([]model.Message, error) {
	if _, err := s.repo.GetConversationByUser(ctx, conversationID, userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}
	if limit < 1 || limit > 64 {
		limit = 64
	}
	items, _, err := s.repo.ListRecentMessages(ctx, conversationID, limit)
	return items, err
}

type companionConversationRepository struct {
	repository.ConversationRepository
	conversationID, userID, forgetThrough uint
	systemPrompt, contextPolicy           string
}

func (r *companionConversationRepository) GetConversationByUser(ctx context.Context, id, userID uint) (*model.Conversation, error) {
	item, err := r.ConversationRepository.GetConversationByUser(ctx, id, userID)
	if err != nil || item == nil {
		return item, err
	}
	if id != r.conversationID || userID != r.userID {
		return nil, repository.ErrNotFound
	}
	copy := *item
	copy.ProjectSystemPrompt = r.systemPrompt
	copy.ContextPolicy = r.contextPolicy
	copy.LastResponseID = ""
	copy.LastPromptFingerprint = ""
	return &copy, nil
}

func (r *companionConversationRepository) ListMessageAncestors(ctx context.Context, id, leaf uint, depth int) ([]model.Message, error) {
	if id != r.conversationID {
		return nil, repository.ErrNotFound
	}
	if depth <= 0 || depth > CompanionRecentMessageLimit {
		depth = CompanionRecentMessageLimit
	}
	items, err := r.ConversationRepository.ListMessageAncestors(ctx, id, leaf, depth)
	if err != nil {
		return nil, err
	}
	kept := make([]model.Message, 0, len(items))
	for _, item := range items {
		if item.ID > r.forgetThrough && item.Status != "blocked" {
			item.ReasoningContent = ""
			kept = append(kept, item)
		}
	}
	// Stop the runtime's paged ancestor scan at our deliberate context boundary.
	if len(kept) > 0 {
		kept[0].ParentMessageID = nil
	}
	return kept, nil
}

// No old ordinary-chat snapshot or recalled artifact can reintroduce forgotten facts.
func (r *companionConversationRepository) GetLatestContextSnapshot(context.Context, uint) (*model.ContextSnapshot, error) {
	return nil, repository.ErrNotFound
}

func (r *companionConversationRepository) ListRecentContextArtifacts(context.Context, repository.ContextArtifactListFilter) ([]model.ContextArtifact, error) {
	return nil, nil
}

type companionLLMGateway struct{ llmGateway }

func (g companionLLMGateway) Generate(ctx context.Context, route llm.RouteConfig, input llm.GenerateInput) (*llm.GenerateOutput, error) {
	return g.llmGateway.Generate(ctx, route, enforceTemporaryGenerateInput(input))
}

func (g companionLLMGateway) GenerateStream(ctx context.Context, route llm.RouteConfig, input llm.GenerateInput, emit func(llm.GenerateStreamEvent) error) (*llm.GenerateOutput, error) {
	return g.llmGateway.GenerateStream(ctx, route, enforceTemporaryGenerateInput(input), emit)
}
