package config

import "testing"

func TestValidateDefersMiniAppSettingsUntilDatabaseOverridesAreLoaded(t *testing.T) {
	cfg := validConfigForEnv("dev")
	cfg.WeChatMiniAppEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("database-backed mini app settings must be validated after loading: %v", err)
	}
}
