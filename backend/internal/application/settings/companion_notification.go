package settings

import (
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

const NotificationConfigNamespace = "companion"
const NotificationConfigKey = "notification"

// One setting keeps the template and its field mapping atomic on updates.
type NotificationConfig struct {
	Enabled    bool
	TemplateID string
	TitleField string
	TimeField  string
	State      string
}

// The settings document is private to the configuration boundary. Public
// application values stay independent of their stored JSON representation.
type notificationConfigDocument struct {
	Enabled    bool   `json:"enabled"`
	TemplateID string `json:"templateId"`
	TitleField string `json:"titleField"`
	TimeField  string `json:"timeField"`
	State      string `json:"state"`
}

func DefaultNotificationConfigJSON() string {
	return `{"enabled":true,"templateId":"GkYenXrXMZjt6OY9NCmm_ylfOUGwdq8TYcW45lzuXEE","titleField":"thing5","timeField":"time30","state":"formal"}`
}

var notificationTemplatePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var notificationTitlePattern = regexp.MustCompile(`^thing[1-9][0-9]{0,5}$`)
var notificationTimePattern = regexp.MustCompile(`^time[1-9][0-9]{0,5}$`)

func ParseNotificationConfig(raw string) (NotificationConfig, error) {
	var c NotificationConfig
	if len(raw) > 4096 {
		return c, errors.New("notification configuration is too large")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document notificationConfigDocument
	if err := decoder.Decode(&document); err != nil {
		return c, errors.New("notification configuration must be a JSON object")
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		return c, errors.New("notification configuration must contain one JSON object")
	}
	c = NotificationConfig{
		Enabled: document.Enabled, TemplateID: document.TemplateID,
		TitleField: document.TitleField, TimeField: document.TimeField, State: document.State,
	}
	c.TemplateID = strings.TrimSpace(c.TemplateID)
	c.TitleField = strings.TrimSpace(c.TitleField)
	c.TimeField = strings.TrimSpace(c.TimeField)
	if !notificationTemplatePattern.MatchString(c.TemplateID) || !notificationTitlePattern.MatchString(c.TitleField) || !notificationTimePattern.MatchString(c.TimeField) {
		return c, errors.New("notification template ID and thing/time field names are required")
	}
	if c.State != "formal" && c.State != "trial" && c.State != "developer" {
		return c, errors.New("notification state must be formal, trial or developer")
	}
	return c, nil
}
