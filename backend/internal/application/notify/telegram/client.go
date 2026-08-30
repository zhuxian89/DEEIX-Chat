// Package telegram 提供独立的管理员 Telegram 通知能力。
// 仅负责把文本消息推送给管理员 Bot，不感知具体业务事件。
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

// DefaultAPIBase 是 Telegram Bot API 默认端点。
const DefaultAPIBase = "https://api.telegram.org"

// Config 是一次发送所需的全部配置。
type Config struct {
	BotToken string
	ChatID   string
}

// Client 通过 Bot API 发送消息。
type Client struct {
	httpClient *http.Client
	apiBase    string
}

// NewClient 创建 Telegram 客户端；httpClient 为 nil 时使用默认实例。
func NewClient(httpClient *http.Client, apiBase string) *Client {
	if httpClient == nil {
		httpClient = security.NewOutboundHTTPClient(security.NewStrictOutboundPolicy(true), 10*time.Second)
	}
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		apiBase = DefaultAPIBase
	}
	return &Client{httpClient: httpClient, apiBase: apiBase}
}

// Ready 判断配置是否满足发送条件。
func (c Config) Ready() bool {
	return strings.TrimSpace(c.BotToken) != "" && strings.TrimSpace(c.ChatID) != ""
}

// SendMessage 发送一条文本消息；返回错误仅供调用方记录日志。
func (c *Client) SendMessage(ctx context.Context, cfg Config, text string) error {
	if !cfg.Ready() {
		return fmt.Errorf("telegram notification is not configured")
	}
	body := strings.NewReader(url.Values{
		"chat_id": {strings.TrimSpace(cfg.ChatID)},
		"text":    {truncateMessage(text)},
	}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.apiBase+"/bot"+strings.TrimSpace(cfg.BotToken)+"/sendMessage", body)
	if err != nil {
		return fmt.Errorf("telegram sendMessage request could not be created")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// The request URL contains the Bot Token; never expose the raw transport
		// error to logs or callers.
		return errors.New("telegram sendMessage request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram sendMessage failed: status=%d", resp.StatusCode)
	}
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return errors.New("telegram sendMessage returned invalid response")
	}
	if !result.OK {
		return fmt.Errorf("telegram sendMessage rejected: %s", strings.TrimSpace(result.Description))
	}
	return nil
}

// telegramMessageMaxRunes 是 Telegram 单条消息上限（4096 字符），预留格式余量。
const telegramMessageMaxRunes = 4000

func truncateMessage(text string) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= telegramMessageMaxRunes {
		return string(runes)
	}
	return string(runes[:telegramMessageMaxRunes]) + "\n…"
}
