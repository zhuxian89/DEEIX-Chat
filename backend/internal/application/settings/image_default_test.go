package settings

import (
	"context"
	"strings"
	"testing"

	domainsettings "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/settings"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
)

func TestSeedConversationDefaultImageModelPreservesExistingValues(t *testing.T) {
	repo := newSettingsSeedRepo(domainsettings.SystemSetting{Namespace: "chat", Key: "conversation_default_model", Value: "existing-chat", ValueType: "string"})
	service := NewService(repo, "")
	if err := service.Seed(context.Background(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	key := "chat:conversation_default_image_model"
	item, ok := repo.items[key]
	if !ok || item.Value != "" || item.ValueType != "string" {
		t.Fatalf("missing empty image default setting: %#v", item)
	}
	item.Value = "configured-image"
	repo.items[key] = item
	if err := service.Seed(context.Background(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	if repo.items[key].Value != "configured-image" || repo.items["chat:conversation_default_model"].Value != "existing-chat" {
		t.Fatal("re-seeding replaced an administrator's default model")
	}
}

func TestRuntimeSettingsAppliesConversationDefaultImageModel(t *testing.T) {
	cfg := config.Config{ConversationDefaultModel: "chat"}
	runtimeSettings := NewRuntimeSettings(nil, nil, "")
	runtimeSettings.applyItem(&cfg, domainsettings.SystemSetting{Namespace: "chat", Key: "conversation_default_image_model", Value: " image-model "})
	if cfg.ConversationDefaultImageModel != "image-model" || cfg.ConversationDefaultModel != "chat" {
		t.Fatal("image default was not applied independently of chat default")
	}
	runtimeSettings.applyItem(&cfg, domainsettings.SystemSetting{Namespace: "chat", Key: "conversation_default_image_model", Value: ""})
	if cfg.ConversationDefaultImageModel != "" {
		t.Fatal("image default was not cleared")
	}
}

func TestValidateConversationDefaultImageModel(t *testing.T) {
	for _, value := range []string{"", "gpt-image", strings.Repeat("x", 255)} {
		if err := validatePatchItem(PatchItem{Namespace: "chat", Key: "conversation_default_image_model", Value: value}); err != nil {
			t.Fatalf("valid image default rejected: %v", err)
		}
	}
	if err := validatePatchItem(PatchItem{Namespace: "chat", Key: "conversation_default_image_model", Value: strings.Repeat("x", 256)}); err == nil {
		t.Fatal("overlong image default accepted")
	}
}
