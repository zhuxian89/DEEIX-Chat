package app

import (
	"strings"
	"sync"

	companionapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/companion"
	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/wechatminiapp"
)

func companionNotificationProvider(runtime *config.Runtime) func() companionapp.NotificationRuntime {
	var mu sync.Mutex
	var lastConfig domain.NotificationConfig
	var lastAppID, lastSecret string
	var current companionapp.NotificationRuntime
	return func() companionapp.NotificationRuntime {
		mu.Lock()
		defer mu.Unlock()
		cfg := runtime.Snapshot()
		c, err := domain.ParseNotificationConfig(cfg.CompanionNotificationJSON)
		appID, secret := strings.TrimSpace(cfg.WeChatMiniAppAppID), strings.TrimSpace(cfg.WeChatMiniAppAppSecret)
		if err != nil || !c.Enabled || appID == "" || secret == "" {
			current = companionapp.NotificationRuntime{}
			lastSecret = ""
			return current
		}
		if current.Sender != nil && c == lastConfig && appID == lastAppID && secret == lastSecret {
			return current
		}
		// Credentials and field mapping belong to the same immutable snapshot.
		// Cache the sender (and its access token) until relevant settings change.
		sender, err := wechatminiapp.NewSubscriptionClient(nil, "", func() (string, string) { return appID, secret }, c.TitleField, c.TimeField, c.State)
		if err != nil {
			return companionapp.NotificationRuntime{}
		}
		lastConfig, lastAppID, lastSecret = c, appID, secret
		current = companionapp.NotificationRuntime{Sender: sender, TemplateID: c.TemplateID, AppID: appID}
		return current
	}
}
