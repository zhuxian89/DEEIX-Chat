package invitation

import (
	"crypto/rand"
	"fmt"
	"math"
	"time"
)

// CodePrefix 是邀请码的固定前缀，用于与注册码（REG-）区分。
const CodePrefix = "INV-"

// DefaultCodeLength 是邀请码随机部分的默认长度（不含前缀）。
const DefaultCodeLength = 7

// codeAlphabet 是去歧义字符集，与注册码一致。
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// GenerateCode 生成带前缀的邀请码。长度为随机部分长度，默认 DefaultCodeLength。
func GenerateCode(randomLength int) (string, error) {
	if randomLength <= 0 {
		randomLength = DefaultCodeLength
	}
	buf := make([]byte, randomLength)
	raw := make([]byte, randomLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = codeAlphabet[int(raw[i])%len(codeAlphabet)]
	}
	return fmt.Sprintf("%s%s", CodePrefix, string(buf)), nil
}

// UsdToNanousd 将美元金额转为纳美元（1 USD = 1e9 nanousd），与 billing 模块一致。
func UsdToNanousd(value float64) int64 {
	return int64(math.Round(value * 1000000000))
}

// ApplyInput 描述注册事务内要应用的邀请奖励，由 application 层读取配置后组装。
// Enabled=false 或 Code 为空时，注册流程宽松放行（正常建用户，不发奖、不报错）。
type ApplyInput struct {
	Code                 string // 用户填写的邀请码（INV-xxx）
	Enabled              bool   // invitation.enabled 配置
	InviteeRewardNanousd int64  // 被邀请人奖励（纳美元），<=0 不发
	InviterRewardNanousd int64  // 邀请人奖励（纳美元），<=0 不发
	CodeLength           int    // 新用户邀请码随机部分长度
}

// InvitationCode 表示用户固定的邀请码，1:1 映射用户。
type InvitationCode struct {
	ID        uint
	UserID    uint
	Code      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Relationship 表示一次邀请事件及其双方奖励发放状态。
type Relationship struct {
	ID                   uint
	InviterUserID        uint
	InvitedUserID        uint
	InvitationCode       string
	InviteeRewardNanousd int64
	InviterRewardNanousd int64
	InviteeRewardedAt    *time.Time
	InviterRewardedAt    *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// InvitationPanel 表示用户中心的邀请面板视图。
type InvitationPanel struct {
	InvitationCode string
	InviteLink     string
	InviteCount    int64
}

// InvitedUser 表示邀请人视角下被邀请人的脱敏信息。
type InvitedUser struct {
	RelationshipID       uint
	InvitedUserID        uint
	InvitedDisplayName   string
	InvitedUsername      string
	InvitedAt            time.Time
	InviterRewardNanousd int64
}
