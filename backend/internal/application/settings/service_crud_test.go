package settings

import (
	"context"
	"errors"
	"testing"

	domainsettings "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/settings"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
)

func TestBatchUpdateRestoresMetadataForMissingSetting(t *testing.T) {
	repo := newSettingsSeedRepo()
	service := NewService(repo, "test-encryption-key")

	_, err := service.BatchUpdate(context.Background(), []PatchItem{
		{Namespace: "notify", Key: "enabled", Value: "false"},
	})
	if err != nil {
		t.Fatalf("batch update: %v", err)
	}

	item := repo.items["notify:enabled"]
	if item.ValueType != "bool" {
		t.Fatalf("expected bool metadata, got %q", item.ValueType)
	}
	if item.Description == "" {
		t.Fatal("expected default description to be restored")
	}
}

func TestDeleteSettingRejectsUnknownKey(t *testing.T) {
	service := NewService(newSettingsSeedRepo(), "")

	err := service.Delete(context.Background(), "notify", "unknown")
	if !errors.Is(err, ErrInvalidSetting) {
		t.Fatalf("expected ErrInvalidSetting, got %v", err)
	}
}

func TestDeleteSettingRemovesKnownSetting(t *testing.T) {
	repo := newSettingsSeedRepo()
	service := NewService(repo, "")
	if _, err := service.BatchUpdate(context.Background(), []PatchItem{
		{Namespace: "notify", Key: "chat_id", Value: "123456"},
	}); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	if err := service.Delete(context.Background(), "notify", "chat_id"); err != nil {
		t.Fatalf("delete setting: %v", err)
	}
	if _, exists := repo.items["notify:chat_id"]; exists {
		t.Fatal("expected setting to be removed")
	}
}

func TestListAvailableReturnsMissingKnownSettingsOnly(t *testing.T) {
	repo := newSettingsSeedRepo(domainsettings.SystemSetting{
		Namespace: "notify",
		Key:       "enabled",
		Value:     "true",
	})
	service := NewService(repo, "")

	available, err := service.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("list available settings: %v", err)
	}
	for _, item := range available["notify"] {
		if item.Key == "enabled" {
			t.Fatal("expected existing setting to be excluded")
		}
	}
	if _, ok := available["notify"]; !ok {
		t.Fatal("expected missing notify settings to be available")
	}
}

func TestRuntimeSettingsReturnsDeletedValueToBaseline(t *testing.T) {
	repo := newSettingsSeedRepo(domainsettings.SystemSetting{
		Namespace: "auth",
		Key:       "token_ttl_hours",
		Value:     "24",
	})
	baseline := config.Config{TokenTTLHours: 12}
	runtime := config.NewRuntime(baseline)
	runtimeSettings := NewRuntimeSettings(repo, nil, "")
	runtimeSettings.SetBaseline(baseline)

	if err := runtimeSettings.ApplyTo(context.Background(), runtime); err != nil {
		t.Fatalf("apply override: %v", err)
	}
	if got := runtime.Snapshot().TokenTTLHours; got != 24 {
		t.Fatalf("expected override 24, got %d", got)
	}

	delete(repo.items, "auth:token_ttl_hours")
	if err := runtimeSettings.ApplyTo(context.Background(), runtime); err != nil {
		t.Fatalf("apply baseline: %v", err)
	}
	if got := runtime.Snapshot().TokenTTLHours; got != 12 {
		t.Fatalf("expected baseline 12 after deletion, got %d", got)
	}
}
