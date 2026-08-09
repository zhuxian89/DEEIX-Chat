package registrationcode

import (
	appregistrationcode "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/registrationcode"
	"time"
)

type CreateRequest struct {
	Quantity int `json:"quantity" binding:"omitempty,min=1,max=100"`
}
type ErrorDoc struct {
	ErrorMsg  string      `json:"errorMsg"`
	ErrorCode string      `json:"errorCode,omitempty"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"requestId,omitempty"`
	Data      interface{} `json:"data"`
}
type CodeResponse struct {
	ID              uint       `json:"id"`
	Code            string     `json:"code"`
	CodeHint        string     `json:"codeHint"`
	Status          string     `json:"status"`
	UsedByUserID    uint       `json:"usedByUserID"`
	UsedAt          *time.Time `json:"usedAt" extensions:"x-nullable,!x-omitempty"`
	CreatedByUserID uint       `json:"createdByUserID"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}
type ListResponse struct {
	Results []CodeResponse `json:"results"`
	Total   int64          `json:"total"`
}

type ListResponseDoc struct {
	ErrorMsg string       `json:"errorMsg"`
	Data     ListResponse `json:"data"`
}

type CreateResponseDoc struct {
	ErrorMsg string `json:"errorMsg"`
	Data     struct {
		Results []CodeResponse `json:"results"`
	} `json:"data"`
}

type DeleteResponseDoc struct {
	ErrorMsg string `json:"errorMsg"`
	Data     struct {
		Deleted bool `json:"deleted"`
	} `json:"data"`
}

func toResponse(item appregistrationcode.CodeView) CodeResponse {
	v := item.RegistrationCode
	return CodeResponse{ID: v.ID, Code: v.Code, CodeHint: v.CodeHint, Status: v.Status, UsedByUserID: v.UsedByUserID, UsedAt: v.UsedAt, CreatedByUserID: v.CreatedByUserID, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
