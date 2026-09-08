package speech

import (
	"errors"
	"strconv"
	"strings"
)

const Namespace = "speech"

type Config struct {
	Enabled   bool
	SecretID  string
	SecretKey string
	Engine    string
}

func ValidEngine(engine string) bool {
	switch engine {
	case "16k_zh", "16k_zh-PY", "16k_zh_dialect":
		return true
	}
	return false
}

// Parse also accepts encrypted credential values for configuration presence checks.
// Only the settings service decrypts credentials for recognition requests.
func Parse(values map[string]string) (Config, error) {
	cfg := Config{
		SecretID:  strings.TrimSpace(values["tencent_secret_id"]),
		SecretKey: strings.TrimSpace(values["tencent_secret_key"]),
		Engine:    strings.TrimSpace(values["engine"]),
	}
	if cfg.Engine == "" {
		cfg.Engine = "16k_zh"
	}
	if raw := strings.TrimSpace(values["enabled"]); raw != "" {
		var err error
		cfg.Enabled, err = strconv.ParseBool(raw)
		if err != nil {
			return Config{}, errors.New("speech:enabled must be a boolean")
		}
	}
	if !ValidEngine(cfg.Engine) {
		return Config{}, errors.New("speech:engine is not supported")
	}
	if cfg.Enabled && (cfg.SecretID == "" || cfg.SecretKey == "") {
		return Config{}, errors.New("speech credentials are required before enabling voice input")
	}
	return cfg, nil
}
