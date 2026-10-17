package app

import (
	"strings"
	"sync"
	"testing"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
)

func TestCompanionNotificationHotConfiguration(t *testing.T) {
	cfg := config.Config{CompanionNotificationJSON: domain.DefaultNotificationConfigJSON()}
	runtime := config.NewRuntime(cfg)
	provider := companionNotificationProvider(runtime)
	if provider().Sender != nil {
		t.Fatal("missing credentials reported ready")
	}
	cfg.WeChatMiniAppAppID, cfg.WeChatMiniAppAppSecret = "app", "secret"
	runtime.Store(cfg)
	first := provider()
	if first.Sender == nil || first.TemplateID == "" || first.AppID != "app" {
		t.Fatal("credentials added after startup did not enable notification")
	}
	if provider().Sender != first.Sender {
		t.Fatal("unchanged config lost sender/token cache")
	}
	cfg.CompanionNotificationJSON = strings.ReplaceAll(cfg.CompanionNotificationJSON, "formal", "developer")
	runtime.Store(cfg)
	second := provider()
	if second.Sender == nil || second.Sender == first.Sender || second.TemplateID != first.TemplateID {
		t.Fatal("jump version did not update independently of template consent")
	}
	cfg.CompanionNotificationJSON = strings.ReplaceAll(cfg.CompanionNotificationJSON, first.TemplateID, "new-template")
	runtime.Store(cfg)
	if provider().TemplateID != "new-template" {
		t.Fatal("template change did not take effect")
	}
	for _, raw := range []string{"", "invalid", strings.ReplaceAll(cfg.CompanionNotificationJSON, `"enabled":true`, `"enabled":false`)} {
		cfg.CompanionNotificationJSON = raw
		runtime.Store(cfg)
		if got := provider(); got.Sender != nil || got.TemplateID != "" {
			t.Fatal("unavailable notification config reported ready")
		}
	}
	cfg.CompanionNotificationJSON = domain.DefaultNotificationConfigJSON()
	runtime.Store(cfg)
	if provider().Sender == nil {
		t.Fatal("re-enabling required a restart")
	}
}

func TestCompanionNotificationConcurrentConfigUpdates(t *testing.T) {
	a := config.Config{CompanionNotificationJSON: domain.DefaultNotificationConfigJSON(), WeChatMiniAppAppID: "app-a", WeChatMiniAppAppSecret: "secret-a"}
	b := a
	b.WeChatMiniAppAppID, b.WeChatMiniAppAppSecret = "app-b", "secret-b"
	c, _ := domain.ParseNotificationConfig(a.CompanionNotificationJSON)
	b.CompanionNotificationJSON = strings.ReplaceAll(a.CompanionNotificationJSON, c.TemplateID, "template-b")
	runtime := config.NewRuntime(a)
	provider := companionNotificationProvider(runtime)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				if n%2 == 0 {
					runtime.Store(a)
				} else {
					runtime.Store(b)
				}
				x := provider()
				if x.Sender == nil || (x.AppID == "app-a" && x.TemplateID != c.TemplateID) || (x.AppID == "app-b" && x.TemplateID != "template-b") {
					t.Error("mixed notification runtime snapshots")
				}
			}
		}()
	}
	wg.Wait()
}
