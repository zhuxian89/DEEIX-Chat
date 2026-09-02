package config

import (
	"strings"
	"testing"
)

func TestValidateRequiresMiniAppSecretsAndPresetsOnlyWhenEnabled(t *testing.T) {
	cfg := validConfigForEnv("dev")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled mini app should not affect validation: %v", err)
	}

	cfg.WeChatMiniAppEnabled = true
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "WECHAT_MINIAPP_APP_ID") {
		t.Fatalf("missing app id error = %v", err)
	}
	cfg.WeChatMiniAppAppID = "wx-app"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "WECHAT_MINIAPP_APP_SECRET") {
		t.Fatalf("missing app secret error = %v", err)
	}
	cfg.WeChatMiniAppAppSecret = "secret"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "WECHAT_MINIAPP_DEFAULT_CHAT_MODEL") {
		t.Fatalf("missing chat preset error = %v", err)
	}
	cfg.WeChatMiniAppDefaultChatModel = "chat-model"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "WECHAT_MINIAPP_DEFAULT_IMAGE_MODEL") {
		t.Fatalf("missing image preset error = %v", err)
	}
	cfg.WeChatMiniAppDefaultImageModel = "image-model"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("complete mini app config error = %v", err)
	}
}
