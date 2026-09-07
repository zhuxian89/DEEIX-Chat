package companion

import (
	"context"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
)

type Profile = domain.Profile
type Memory = domain.Memory
type Topic = domain.Topic

var (
	ErrBusy     = domain.ErrBusy
	ErrNotFound = domain.ErrNotFound
	ErrInvalid  = domain.ErrInvalid
)

// Store keeps persistence details outside the companion use cases.
type Store interface {
	Profile(ctx context.Context, userID uint) (*Profile, error)
	Ensure(ctx context.Context, userID uint) (*Profile, error)
	Acquire(ctx context.Context, userID uint) (string, error)
	Renew(ctx context.Context, userID uint, token string) error
	Release(userID uint, token string)
	Memories(ctx context.Context, userID uint) ([]Memory, error)
	Forget(ctx context.Context, userID, throughID uint, memoryID string) error
	Edit(ctx context.Context, userID, throughID uint, memoryID, value string) error
	SaveConversation(ctx context.Context, profile *Profile, token string) error
	UpdateModel(ctx context.Context, userID uint, token, model string) error
	SaveGreeting(ctx context.Context, userID uint, token string, greeting domain.GreetingUpdate) error
	SetQuiet(ctx context.Context, userID uint, quiet bool) error
	MarkRead(ctx context.Context, userID, messageID uint) error
	ClaimRefresh(ctx context.Context, userID, revision uint, token string, now time.Time) (bool, error)
	SaveExtraction(ctx context.Context, extraction domain.Extraction) error
	TopicCaches(ctx context.Context, since time.Time, limit int) ([]domain.TopicCache, error)
	ClaimTopics(ctx context.Context, category, token string, now time.Time) (bool, error)
	SaveTopics(ctx context.Context, category, token, topicsJSON string, now time.Time) error
	SaveTopicFeedback(ctx context.Context, userID uint, token string, memory Memory) error
}
