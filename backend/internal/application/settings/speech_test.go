package settings

import (
	"context"
	"strings"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
)

func TestSpeechSettingsUseExistingEncryptedConfiguration(t *testing.T) {
	ctx := context.Background()
	repo := newSettingsSeedRepo()
	s := NewService(repo, "speech-test-encryption-key")
	if err := s.Seed(ctx, config.Config{}); err != nil {
		t.Fatal(err)
	}
	if repo.items["speech:enabled"].Value != "false" {
		t.Fatal("speech must default to disabled")
	}
	_, err := s.BatchUpdate(ctx, []PatchItem{{Namespace: "speech", Key: "enabled", Value: "true"}})
	if err == nil {
		t.Fatal("enabled speech without credentials")
	}
	if repo.items["speech:enabled"].Value != "false" {
		t.Fatal("invalid update was persisted")
	}
	result, err := s.BatchUpdate(ctx, []PatchItem{
		{Namespace: "speech", Key: "tencent_secret_id", Value: "test-secret-id"},
		{Namespace: "speech", Key: "tencent_secret_key", Value: "test-secret-key"},
		{Namespace: "speech", Key: "enabled", Value: "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tencent_secret_id", "tencent_secret_key"} {
		stored := repo.items["speech:"+key].Value
		if stored == "" || strings.Contains(stored, "test-secret") {
			t.Fatal("credential was not encrypted")
		}
		for _, item := range result["speech"] {
			if item.Key == key && (!item.Sensitive || !item.Configured || item.Value != "") {
				t.Fatal("credential exposed in admin response")
			}
		}
	}
	_, err = s.BatchUpdate(ctx, []PatchItem{{Namespace: "speech", Key: "tencent_secret_key", Value: ""}})
	if err != nil {
		t.Fatal(err)
	}
	values, err := s.RuntimeValuesByNamespace(ctx, "speech")
	if err != nil || values["tencent_secret_key"] != "test-secret-key" {
		t.Fatal("blank edit cleared saved credential")
	}
	_, err = s.BatchUpdate(ctx, []PatchItem{{Namespace: "speech", Key: "engine", Value: "16k_zh-PY"}, {Namespace: "speech", Key: "tencent_secret_key", Value: "rotated-key"}})
	if err != nil {
		t.Fatal(err)
	}
	values, err = s.RuntimeValuesByNamespace(ctx, "speech")
	if err != nil || values["engine"] != "16k_zh-PY" || values["tencent_secret_key"] != "rotated-key" {
		t.Fatal("configuration did not update immediately")
	}
	_, err = s.BatchUpdate(ctx, []PatchItem{{Namespace: "speech", Key: "tencent_secret_key", Clear: true}})
	if err == nil {
		t.Fatal("cleared required credential while enabled")
	}
	_, err = s.BatchUpdate(ctx, []PatchItem{{Namespace: "speech", Key: "enabled", Value: "false"}, {Namespace: "speech", Key: "tencent_secret_key", Clear: true}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSpeechSeedImportsCredentialsEncryptedAndPreservesAdminChanges(t *testing.T) {
	ctx := context.Background()
	repo := newSettingsSeedRepo()
	s := NewService(repo, "speech-test-encryption-key")
	cfg := config.Config{SpeechTencentSecretID: " bootstrap-id ", SpeechTencentSecretKey: " bootstrap-key "}
	if err := s.Seed(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	values, err := s.RuntimeValuesByNamespace(ctx, "speech")
	if err != nil || values["enabled"] != "true" || values["tencent_secret_id"] != "bootstrap-id" || values["tencent_secret_key"] != "bootstrap-key" {
		t.Fatal("startup did not import and enable speech credentials")
	}
	for _, key := range []string{"tencent_secret_id", "tencent_secret_key"} {
		stored := repo.items["speech:"+key].Value
		if stored == "" || strings.Contains(stored, "bootstrap-") {
			t.Fatal("startup credential was not encrypted")
		}
	}
	items, err := s.ListByNamespace(ctx, "speech")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if strings.HasPrefix(item.Key, "tencent_secret_") && (!item.Sensitive || !item.Configured || item.Value != "") {
			t.Fatal("startup credential exposed in admin response")
		}
	}
	_, err = s.BatchUpdate(ctx, []PatchItem{
		{Namespace: "speech", Key: "tencent_secret_key", Value: "rotated-key"},
		{Namespace: "speech", Key: "enabled", Value: "false"},
		{Namespace: "speech", Key: "engine", Value: "16k_zh-PY"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, restart := range []config.Config{cfg, {}} {
		if err := s.Seed(ctx, restart); err != nil {
			t.Fatal(err)
		}
		values, err = s.RuntimeValuesByNamespace(ctx, "speech")
		if err != nil || values["enabled"] != "false" || values["tencent_secret_id"] != "bootstrap-id" || values["tencent_secret_key"] != "rotated-key" || values["engine"] != "16k_zh-PY" {
			t.Fatal("restart overwrote settings maintained by the administrator")
		}
	}
}

func TestSpeechSeedDoesNotEnableIncompleteOrPreviouslyDisabledSettings(t *testing.T) {
	ctx := context.Background()
	for _, cfg := range []config.Config{
		{},
		{SpeechTencentSecretID: "bootstrap-id", SpeechTencentSecretKey: " "},
		{SpeechTencentSecretKey: "bootstrap-key"},
	} {
		repo := newSettingsSeedRepo()
		s := NewService(repo, "speech-test-encryption-key")
		if err := s.Seed(ctx, cfg); err != nil {
			t.Fatal(err)
		}
		if repo.items["speech:enabled"].Value != "false" {
			t.Fatal("startup enabled speech without both credentials")
		}
		cfg = config.Config{SpeechTencentSecretID: "other-id", SpeechTencentSecretKey: "other-key"}
		if err := s.Seed(ctx, cfg); err != nil {
			t.Fatal(err)
		}
		if repo.items["speech:enabled"].Value != "false" {
			t.Fatal("later deployment re-enabled an existing disabled setting")
		}
	}
}
