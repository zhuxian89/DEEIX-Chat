package settings

import (
	"strings"
	"testing"

	domainsettings "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/settings"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
)

func TestMiniappTodoFeedbackCodeIsSensitive(t *testing.T) {
	s := NewService(nil, "")
	item := s.settingResponse(domainsettings.SystemSetting{Namespace: "miniapp_todo", Key: "feedback_code", Value: "configured-value"})
	if !item.Sensitive || !item.Configured || item.Value != "" {
		t.Fatalf("feedback code exposed: %+v", item)
	}
}

func TestMiniappTodoCodeSupportsEncryptedDynamicUpdatesAndClearing(t *testing.T) {
	repo := newSettingsSeedRepo()
	s := NewService(repo, "todo-test-encryption-key")
	if err := s.Seed(t.Context(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	if repo.items["miniapp_todo:feedback_code"].Value != "" {
		t.Fatal("unexpected default code")
	}
	for _, value := range []string{"888", "changed-code"} {
		result, err := s.BatchUpdate(t.Context(), []PatchItem{{Namespace: "miniapp_todo", Key: "feedback_code", Value: value}})
		if err != nil {
			t.Fatal(err)
		}
		stored := repo.items["miniapp_todo:feedback_code"].Value
		if stored == "" || stored == value {
			t.Fatal("code was not encrypted")
		}
		if len(result["miniapp_todo"]) != 1 || result["miniapp_todo"][0].Value != "" {
			t.Fatal("code exposed in response")
		}
		values, err := s.RuntimeValuesByNamespace(t.Context(), "miniapp_todo")
		if err != nil || values["feedback_code"] != value {
			t.Fatal("configuration did not update", err)
		}
	}
	if _, err := s.BatchUpdate(t.Context(), []PatchItem{{Namespace: "miniapp_todo", Key: "feedback_code", Value: ""}}); err != nil {
		t.Fatal(err)
	}
	values, err := s.RuntimeValuesByNamespace(t.Context(), "miniapp_todo")
	if err != nil || values["feedback_code"] != "changed-code" {
		t.Fatal("blank edit must preserve the secret", err)
	}
	if err := s.Seed(t.Context(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	values, err = s.RuntimeValuesByNamespace(t.Context(), "miniapp_todo")
	if err != nil || values["feedback_code"] != "changed-code" {
		t.Fatal("restart overwrote configuration", err)
	}
	if _, err := s.BatchUpdate(t.Context(), []PatchItem{{Namespace: "miniapp_todo", Key: "feedback_code", Clear: true}}); err != nil {
		t.Fatal(err)
	}
	values, err = s.RuntimeValuesByNamespace(t.Context(), "miniapp_todo")
	if err != nil || values["feedback_code"] != "" {
		t.Fatal("explicit clear failed", err)
	}
	if err := validatePatchItem(PatchItem{Namespace: "miniapp_todo", Key: "feedback_code", Value: strings.Repeat("x", 129)}); err == nil {
		t.Fatal("oversized code accepted")
	}
}
