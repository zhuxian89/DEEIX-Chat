package conversation

import "strings"

// GetConversationDefaultImageModel 返回后台配置的生图入口默认模型。
func (s *Service) GetConversationDefaultImageModel() string {
	if s == nil || s.cfg == nil {
		return ""
	}
	return strings.TrimSpace(s.cfg.Snapshot().ConversationDefaultImageModel)
}
