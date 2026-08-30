package telegram

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// settingsReader 提供运行时读取 Telegram 配置的能力（对应 settings.Service.RuntimeValuesByNamespace）。
type settingsReader interface {
	RuntimeValuesByNamespace(ctx context.Context, namespace string) (map[string]string, error)
}

// Namespace 是 Telegram 配置在 system_settings 中的命名空间。
const Namespace = "notify"

// 配置键定义。
const (
	KeyEnabled  = "enabled"
	KeyBotToken = "bot_token"
	KeyChatID   = "chat_id"
)

// Notifier 面向业务事件的 Telegram 管理员通知器。
// 每次发送时读取最新配置，便于管理端修改后即时生效；发送失败只记日志，绝不影响主流程。
type Notifier struct {
	settings    settingsReader
	client      *Client
	logger      *zap.Logger
	wg          sync.WaitGroup
	lifecycleMu sync.RWMutex
	closing     bool
}

// NewNotifier 创建通知器；logger 可为 nil（静默）。
func NewNotifier(settings settingsReader, client *Client, logger *zap.Logger) *Notifier {
	return &Notifier{settings: settings, client: client, logger: logger}
}

// loadConfig 读取当前生效配置；读取失败时按未配置处理。
func (n *Notifier) loadConfig(ctx context.Context) (Config, bool) {
	if n.settings == nil {
		return Config{}, false
	}
	values, err := n.settings.RuntimeValuesByNamespace(ctx, Namespace)
	if err != nil {
		n.log().Warn("telegram notify settings load failed", zap.Error(err))
		return Config{}, false
	}
	enabled := true
	if rawEnabled := strings.TrimSpace(values[KeyEnabled]); rawEnabled != "" {
		enabled, _ = strconv.ParseBool(rawEnabled)
	}
	if !enabled {
		return Config{}, false
	}
	return Config{BotToken: values[KeyBotToken], ChatID: values[KeyChatID]}, true
}

// NotifyRegistration 异步通知管理员有新用户注册。
func (n *Notifier) NotifyRegistration(userID uint, username string, email string) {
	header := "🔔 新用户注册"
	if userID > 0 {
		header += " (#" + strconv.FormatUint(uint64(userID), 10) + ")"
	}
	text := header + "\n" +
		"用户名: " + username + "\n" +
		"邮箱: " + email + "\n" +
		"时间: " + time.Now().Format("2006-01-02 15:04:05")
	n.dispatch(text)
}

// NotifyWeChatMessage 异步镜像一条微信公众号收到的消息。
func (n *Notifier) NotifyWeChatMessage(openID string, messageType string, content string) {
	text := "💬 微信公众号收到消息\n" +
		"OpenID: " + openID + "\n" +
		"类型: " + messageType + "\n" +
		"内容: " + content + "\n" +
		"时间: " + time.Now().Format("2006-01-02 15:04:05")
	n.dispatch(text)
}

// NotifyWeChatReply 异步镜像一条微信公众号对外发送的回复。
func (n *Notifier) NotifyWeChatReply(openID string, content string) {
	text := "💬 微信公众号回复\n" +
		"OpenID: " + openID + "\n" +
		"内容: " + content + "\n" +
		"时间: " + time.Now().Format("2006-01-02 15:04:05")
	n.dispatch(text)
}

// dispatch 在独立 goroutine 中发送，超时 15s，失败仅记日志，绝不阻塞主流程。
func (n *Notifier) dispatch(text string) {
	if n == nil || n.client == nil {
		return
	}
	n.lifecycleMu.RLock()
	if n.closing {
		n.lifecycleMu.RUnlock()
		return
	}
	n.wg.Add(1)
	n.lifecycleMu.RUnlock()
	go func() {
		defer n.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cfg, enabled := n.loadConfig(ctx)
		if !enabled {
			return
		}
		if !cfg.Ready() {
			n.log().Debug("telegram notify skipped: bot token or chat id not configured")
			return
		}
		if err := n.client.SendMessage(ctx, cfg, text); err != nil {
			n.log().Warn("telegram notify send failed", zap.Error(err))
		}
	}()
}

// Close 等待所有进行中的发送完成（用于优雅停机）。
func (n *Notifier) Close() {
	if n == nil {
		return
	}
	n.lifecycleMu.Lock()
	n.closing = true
	n.lifecycleMu.Unlock()
	n.wg.Wait()
}

func (n *Notifier) log() *zap.Logger {
	if n.logger != nil {
		return n.logger
	}
	return zap.NewNop()
}
