package settings

import (
	"strings"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
)

func TestCompanionNotificationSeedUpdateAndDelete(t *testing.T) {
	repo := newSettingsSeedRepo()
	service := NewService(repo, "")
	if err := service.Seed(t.Context(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	item, ok := repo.items["companion:notification"]
	if !ok || item.ValueType != "json" || item.Value != DefaultNotificationConfigJSON() {
		t.Fatal("reminder defaults were not seeded")
	}
	custom := strings.ReplaceAll(item.Value, "formal", "developer")
	if _, err := service.BatchUpdate(t.Context(), []PatchItem{{Namespace: "companion", Key: "notification", Value: custom}}); err != nil {
		t.Fatal(err)
	}
	if err := service.Seed(t.Context(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	runtime := config.NewRuntime(config.Config{})
	loader := NewRuntimeSettings(repo, nil, "")
	loader.SetBaseline(config.Config{})
	if err := loader.ApplyTo(t.Context(), runtime); err != nil {
		t.Fatal(err)
	}
	if runtime.Snapshot().CompanionNotificationJSON != custom {
		t.Fatal("restart replaced admin value or update did not reach runtime")
	}
	if err := service.Delete(t.Context(), "companion", "notification"); err != nil {
		t.Fatal(err)
	}
	if err := loader.ApplyTo(t.Context(), runtime); err != nil {
		t.Fatal(err)
	}
	if runtime.Snapshot().CompanionNotificationJSON != "" {
		t.Fatal("deleted notification config remained active")
	}
}

func TestCompanionNotificationRejectsInvalidChangesWithoutOverwriting(t *testing.T) {
	repo := newSettingsSeedRepo()
	service := NewService(repo, "")
	if err := service.Seed(t.Context(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	valid := DefaultNotificationConfigJSON()
	for _, bad := range []string{
		`null`, `{}`, valid + `{}`, strings.ReplaceAll(valid, "thing5", "{{thing5.DATA}}"),
		strings.ReplaceAll(valid, "time30", "thing30"), strings.ReplaceAll(valid, "formal", "production"),
		strings.ReplaceAll(valid, "templateId", "template"), strings.ReplaceAll(valid, `"enabled":true`, `"enabled":"true"`),
	} {
		if _, err := service.BatchUpdate(t.Context(), []PatchItem{{Namespace: "companion", Key: "notification", Value: bad}}); err == nil {
			t.Fatalf("accepted invalid notification configuration: %s", bad)
		}
		if repo.items["companion:notification"].Value != valid {
			t.Fatal("invalid edit overwrote working notification configuration")
		}
	}
}
