package companion

import (
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
)

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
	Topic                *Topic    `json:"topic,omitempty"`
}

type Memory struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Evidence  string    `json:"evidence"`
	ExpiresAt time.Time `json:"expiresAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Topic struct {
	Category    string    `json:"category"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Opener      string    `json:"opener"`
	PublishedAt time.Time `json:"publishedAt"`
	FetchedAt   time.Time `json:"fetchedAt"`
}

func stateDTO(state *app.State) *State {
	if state == nil {
		return nil
	}
	memories := make([]Memory, len(state.Memories))
	for i, memory := range state.Memories {
		memories[i] = Memory{
			ID:        memory.ID,
			Key:       memory.Key,
			Value:     memory.Value,
			Evidence:  memory.Evidence,
			ExpiresAt: memory.ExpiresAt,
			UpdatedAt: memory.UpdatedAt,
		}
	}
	result := &State{
		Name:                 state.Name,
		Model:                state.Model,
		ConversationPublicID: state.ConversationPublicID,
		Quiet:                state.Quiet,
		Greeting:             state.Greeting,
		GreetingID:           state.GreetingID,
		GreetingAt:           state.GreetingAt,
		GreetingOffered:      state.GreetingOffered,
		Memories:             memories,
	}
	if state.Topic != nil {
		topic := Topic(*state.Topic)
		result.Topic = &topic
	}
	return result
}
