package model

// WeChatReplyTemplate stores administrator-managed text replies.
type WeChatReplyTemplate struct {
	ControlPlaneModel
	Name         string `gorm:"size:128;not null;uniqueIndex:idx_wechat_reply_templates_name;comment:妯℃澘鍚嶇О"`
	ResponseType string `gorm:"size:32;not null;default:'text';comment:鍥炲绫诲瀷"`
	Content      string `gorm:"type:text;not null;comment:妯℃澘鍐呭"`
	Enabled      bool   `gorm:"not null;default:true;index:idx_wechat_reply_templates_enabled;comment:鏄惁鍚敤"`
}

func (WeChatReplyTemplate) TableName() string { return "wechat_reply_templates" }

// WeChatKeywordRule maps one exact keyword to one action and one reply template.
type WeChatKeywordRule struct {
	ControlPlaneModel
	Keyword    string              `gorm:"size:128;not null;uniqueIndex:idx_wechat_keyword_rules_keyword;comment:鍏众鍙峰叧閿瘝"`
	Action     string              `gorm:"size:64;not null;index:idx_wechat_keyword_rules_action;comment:鍔熻兘action"`
	TemplateID uint                `gorm:"not null;index:idx_wechat_keyword_rules_template;comment:鍥炲妯℃澘ID"`
	Enabled    bool                `gorm:"not null;default:true;index:idx_wechat_keyword_rules_enabled;comment:鏄惁鍚敤"`
	Template   WeChatReplyTemplate `gorm:"foreignKey:TemplateID"`
}

func (WeChatKeywordRule) TableName() string { return "wechat_keyword_rules" }

// WeChatKeywordInvocationLog records a handled keyword request without storing raw XML.
type WeChatKeywordInvocationLog struct {
	ControlPlaneModel
	OpenID             string `gorm:"size:128;not null;index:idx_wechat_invocation_logs_open_id;comment:WeChat OpenID"`
	Keyword            string `gorm:"size:128;not null;index:idx_wechat_invocation_logs_keyword;comment:鍏众鍙峰叧閿瘝"`
	Action             string `gorm:"size:64;not null;index:idx_wechat_invocation_logs_action;comment:鍔熻兘action"`
	TemplateID         uint   `gorm:"not null;default:0;index:idx_wechat_invocation_logs_template;comment:妯℃澘ID"`
	RegistrationCodeID uint   `gorm:"not null;default:0;index:idx_wechat_invocation_logs_code;comment:娉ㄥ唽鐮佹 ID"`
	Result             string `gorm:"size:32;not null;index:idx_wechat_invocation_logs_result;comment:澶勭悊缁撴灉"`
	ErrorCode          string `gorm:"size:64;not null;default:'';comment:error code"`
	ErrorMessage       string `gorm:"type:text;not null;comment:閿欒鎽樿"`
}

func (WeChatKeywordInvocationLog) TableName() string { return "wechat_keyword_invocation_logs" }
