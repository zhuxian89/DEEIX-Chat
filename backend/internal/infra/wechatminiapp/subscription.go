package wechatminiapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	domain "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/companion"
)

// SubscriptionClient sends only a locally reserved, explicitly authorized delivery.
// A transport failure is ambiguous and must not trigger another send.
type SubscriptionClient struct {
	client                       *Client
	credentials                  func() (string, string)
	titleField, timeField, state string
	mu                           sync.Mutex
	token, appID, secret         string
	expires                      time.Time
}

func NewSubscriptionClient(httpClient *http.Client, base string, credentials func() (string, string), titleField, timeField, state string) (*SubscriptionClient, error) {
	if credentials == nil || !regexp.MustCompile(`^thing[1-9][0-9]*$`).MatchString(titleField) || !regexp.MustCompile(`^time[1-9][0-9]*$`).MatchString(timeField) {
		return nil, errors.New("subscription template fields are not configured")
	}
	if state == "" {
		state = "formal"
	}
	if state != "formal" && state != "trial" && state != "developer" {
		return nil, errors.New("invalid subscription environment")
	}
	return &SubscriptionClient{client: NewClient(httpClient, base), credentials: credentials, titleField: titleField, timeField: timeField, state: state}, nil
}

func (c *SubscriptionClient) post(ctx context.Context, path string, body interface{}, out interface{}) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return errors.New("subscription request is invalid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.client.apiBase+path, bytes.NewReader(encoded))
	if err != nil {
		return errors.New("subscription request could not be created")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.httpClient.Do(req)
	if err != nil {
		return errors.New("subscription request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("subscription response status=%d", resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	if err != nil || json.Unmarshal(payload, out) != nil {
		return errors.New("subscription response is invalid")
	}
	return nil
}

func (c *SubscriptionClient) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	appID, secret := c.credentials()
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(secret) == "" {
		return "", errors.New("subscription credentials are unavailable")
	}
	if appID == c.appID && secret == c.secret && time.Now().Before(c.expires) {
		return c.token, nil
	}
	var result struct {
		Token   string `json:"access_token"`
		Expires int    `json:"expires_in"`
		Code    int    `json:"errcode"`
	}
	err := c.post(ctx, "/cgi-bin/stable_token", map[string]interface{}{"grant_type": "client_credential", "appid": appID, "secret": secret, "force_refresh": false}, &result)
	if err != nil {
		return "", err
	}
	if result.Code != 0 || result.Token == "" || result.Expires < 120 {
		return "", errors.New("subscription token was rejected")
	}
	c.appID, c.secret, c.token = appID, secret, result.Token
	c.expires = time.Now().Add(time.Duration(result.Expires-60) * time.Second)
	return c.token, nil
}

func (c *SubscriptionClient) Send(ctx context.Context, d domain.NotificationDelivery) error {
	if d.OpenID == "" || d.TemplateID == "" || d.Title == "" || d.DueAt.IsZero() {
		return errors.New("subscription delivery is invalid")
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	title := []rune(strings.TrimSpace(d.Title))
	if len(title) > 20 {
		title = title[:20]
	}
	body := map[string]interface{}{"touser": d.OpenID, "template_id": d.TemplateID, "page": "pages/index/index", "miniprogram_state": c.state, "lang": "zh_CN", "data": map[string]interface{}{
		c.titleField: map[string]string{"value": string(title)}, c.timeField: map[string]string{"value": d.DueAt.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02 15:04")}}}
	var result struct {
		Code *int `json:"errcode"`
	}
	if err := c.post(ctx, "/cgi-bin/message/subscribe/send?access_token="+url.QueryEscape(token), body, &result); err != nil {
		return err
	}
	if result.Code == nil {
		return errors.New("subscription response has no result")
	}
	if *result.Code != 0 {
		if *result.Code == 40001 || *result.Code == 40014 || *result.Code == 42001 {
			c.mu.Lock()
			if c.token == token {
				c.expires = time.Time{}
			}
			c.mu.Unlock()
		}
		return fmt.Errorf("subscription delivery rejected: code=%d", *result.Code)
	}
	return nil
}
