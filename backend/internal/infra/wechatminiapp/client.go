package wechatminiapp

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

	domainwechatminiapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechatminiapp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

const DefaultAPIBase = "https://api.weixin.qq.com"

type Client struct {
	httpClient *http.Client
	apiBase    string
}

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

func (c *Client) Exchange(ctx context.Context, appID, appSecret, code string) (domainwechatminiapp.Identity, error) {
	appID, appSecret, code = strings.TrimSpace(appID), strings.TrimSpace(appSecret), strings.TrimSpace(code)
	if appID == "" || appSecret == "" || code == "" {
		return domainwechatminiapp.Identity{}, errors.New("wechat mini program exchange is not configured")
	}
	query := url.Values{
		"appid":      {appID},
		"secret":     {appSecret},
		"js_code":    {code},
		"grant_type": {"authorization_code"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"/sns/jscode2session?"+query.Encode(), nil)
	if err != nil {
		return domainwechatminiapp.Identity{}, errors.New("wechat mini program exchange request could not be created")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// The URL contains AppSecret and the one-time code. Never return the raw transport error.
		return domainwechatminiapp.Identity{}, errors.New("wechat mini program exchange request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if err != nil {
		return domainwechatminiapp.Identity{}, errors.New("wechat mini program exchange response could not be read")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domainwechatminiapp.Identity{}, fmt.Errorf("wechat mini program exchange failed: status=%d", resp.StatusCode)
	}
	var result struct {
		OpenID  string `json:"openid"`
		UnionID string `json:"unionid"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err = json.Unmarshal(payload, &result); err != nil {
		return domainwechatminiapp.Identity{}, errors.New("wechat mini program exchange returned invalid response")
	}
	if result.ErrCode != 0 {
		return domainwechatminiapp.Identity{}, fmt.Errorf("wechat mini program exchange rejected: code=%d", result.ErrCode)
	}
	if strings.TrimSpace(result.OpenID) == "" {
		return domainwechatminiapp.Identity{}, errors.New("wechat mini program exchange returned no openid")
	}
	return domainwechatminiapp.Identity{OpenID: strings.TrimSpace(result.OpenID), UnionID: strings.TrimSpace(result.UnionID)}, nil
}
