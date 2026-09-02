package wechatminiapp

import "time"

type LoginRequest struct {
	Code string `json:"code" binding:"required,max=256"`
}

type MiniAppPresetsResponse struct {
	ChatModel  string `json:"chatModel"`
	ImageModel string `json:"imageModel"`
}

type MiniAppUserResponse struct {
	ID                   uint   `json:"id"`
	PublicID             string `json:"publicID"`
	Username             string `json:"username"`
	DisplayName          string `json:"displayName"`
	AvatarURL            string `json:"avatarURL"`
	SubscriptionTier     string `json:"subscriptionTier"`
	SubscriptionPlanName string `json:"subscriptionPlanName"`
	SubscriptionStatus   string `json:"subscriptionStatus"`
}

type MiniAppAuthResponse struct {
	AccessToken      string              `json:"accessToken"`
	SessionID        string              `json:"sessionID"`
	ExpiresAt        time.Time           `json:"expiresAt"`
	RefreshExpiresAt time.Time           `json:"refreshExpiresAt"`
	User             MiniAppUserResponse `json:"user"`
}

type LoginResponse struct {
	Auth    MiniAppAuthResponse    `json:"auth"`
	Created bool                   `json:"created"`
	Presets MiniAppPresetsResponse `json:"presets"`
}

type LoginResponseDoc struct {
	Data     LoginResponse `json:"data"`
	ErrorMsg string        `json:"errorMsg"`
}

type ErrorDoc struct {
	Data      interface{} `json:"data" extensions:"x-nullable"`
	ErrorCode string      `json:"errorCode"`
	ErrorMsg  string      `json:"errorMsg"`
	RequestID string      `json:"requestId,omitempty"`
}
