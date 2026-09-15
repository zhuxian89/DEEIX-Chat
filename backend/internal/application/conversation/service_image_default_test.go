package conversation

import (
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
)

func TestConversationDefaultImageModelUsesCurrentRuntime(t *testing.T) {
	var missing *Service
	if missing.GetConversationDefaultImageModel() != "" || (&Service{}).GetConversationDefaultImageModel() != "" {
		t.Fatal("missing configuration should return an empty candidate")
	}
	runtime := config.NewRuntime(config.Config{ConversationDefaultModel: "chat", ConversationDefaultImageModel: " image-a "})
	service := &Service{cfg: runtime}
	if service.GetConversationDefaultImageModel() != "image-a" || service.GetConversationSystemDefaultModel() != "chat" {
		t.Fatal("image and chat defaults must remain independent")
	}
	cfg := runtime.Snapshot()
	cfg.ConversationDefaultImageModel = "image-b"
	runtime.Store(cfg)
	if service.GetConversationDefaultImageModel() != "image-b" {
		t.Fatal("image default did not pick up runtime update")
	}
}
