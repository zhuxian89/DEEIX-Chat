package companion

// NotificationRuntime is immutable for one consent or claimed delivery.
type NotificationRuntime struct {
	Sender     NotificationSender
	TemplateID string
	AppID      string
}

func (s *Service) notificationRuntime() NotificationRuntime {
	if s.NotificationProvider != nil {
		return s.NotificationProvider()
	}
	return NotificationRuntime{Sender: s.Notifier, TemplateID: s.NotificationTemplateID}
}
