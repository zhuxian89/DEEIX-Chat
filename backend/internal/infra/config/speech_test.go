package config

import "testing"

func TestLoadSpeechCredentialsForSettingsBootstrap(t *testing.T) {
	t.Setenv("SPEECH_TENCENT_SECRET_ID", " bootstrap-id ")
	t.Setenv("SPEECH_TENCENT_SECRET_KEY", " bootstrap-key ")
	cfg := Load()
	if cfg.SpeechTencentSecretID != "bootstrap-id" || cfg.SpeechTencentSecretKey != "bootstrap-key" {
		t.Fatal("speech bootstrap credentials were not loaded and trimmed")
	}
	t.Setenv("SPEECH_TENCENT_SECRET_ID", "")
	t.Setenv("SPEECH_TENCENT_SECRET_KEY", "")
	cfg = Load()
	if cfg.SpeechTencentSecretID != "" || cfg.SpeechTencentSecretKey != "" {
		t.Fatal("speech bootstrap credentials have a hardcoded fallback")
	}
}
