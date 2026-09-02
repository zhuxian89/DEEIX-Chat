package user

import (
	"context"
	"strings"
	"time"

	domainuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/user"
	domainwechatminiapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechatminiapp"
	models "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repo) GetMiniAppBinding(ctx context.Context, appID, openID string) (*domainwechatminiapp.Binding, error) {
	var item models.WeChatMiniAppBinding
	if err := r.db.WithContext(ctx).
		Where("app_id = ? AND open_id = ?", strings.TrimSpace(appID), strings.TrimSpace(openID)).
		First(&item).Error; err != nil {
		return nil, translateError(err)
	}
	return toDomainMiniAppBinding(item), nil
}

func (r *Repo) TouchMiniAppBinding(ctx context.Context, appID, openID, unionID string, now time.Time) (*domainwechatminiapp.Binding, error) {
	var result *domainwechatminiapp.Binding
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item models.WeChatMiniAppBinding
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("app_id = ? AND open_id = ?", strings.TrimSpace(appID), strings.TrimSpace(openID)).
			First(&item).Error; err != nil {
			return translateError(err)
		}
		if item.RevokedAt != nil {
			return repository.ErrConflict
		}
		unionID = strings.TrimSpace(unionID)
		if item.UnionID != "" && unionID != "" && item.UnionID != unionID {
			return repository.ErrConflict
		}
		updates := map[string]interface{}{"last_login_at": now}
		if item.UnionID == "" && unionID != "" {
			updates["union_id"] = unionID
			updates["union_id_observed_at"] = now
		}
		if err := tx.Model(&item).Updates(updates).Error; err != nil {
			return translateError(err)
		}
		if err := tx.Where("id = ?", item.ID).First(&item).Error; err != nil {
			return translateError(err)
		}
		result = toDomainMiniAppBinding(item)
		return nil
	})
	return result, translateError(err)
}

// CreateMiniAppUserAndBinding atomically creates the canonical DEEIX account,
// its disabled password credential, invitation code, and Mini Program binding.
func (r *Repo) CreateMiniAppUserAndBinding(
	ctx context.Context,
	user *domainuser.User,
	credential domainuser.Credential,
	binding *domainwechatminiapp.Binding,
	invitationCodeLength int,
) error {
	if user == nil || binding == nil || strings.TrimSpace(binding.AppID) == "" || strings.TrimSpace(binding.OpenID) == "" {
		return repository.ErrInvalidInput
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.createWithCredentialTx(tx, user, credential, 0, 0, nil, false); err != nil {
			return err
		}
		// GORM applies the model's default:true tag to a zero-value bool during
		// Create. Mini Program accounts are intentionally passwordless, so make
		// the trust boundary explicit inside the same transaction.
		if !credential.PasswordEnabled {
			if err := tx.Model(&models.UserCredential{}).
				Where("user_id = ?", user.ID).
				Update("password_enabled", false).Error; err != nil {
				return translateError(err)
			}
		}
		if err := createInvitationCodeForUserTx(tx, user.ID, invitationCodeLength); err != nil {
			return err
		}
		item := &models.WeChatMiniAppBinding{
			UserID:            user.ID,
			AppID:             strings.TrimSpace(binding.AppID),
			OpenID:            strings.TrimSpace(binding.OpenID),
			UnionID:           strings.TrimSpace(binding.UnionID),
			UnionIDObservedAt: binding.UnionIDObservedAt,
			LastLoginAt:       binding.LastLoginAt,
			RevokedAt:         binding.RevokedAt,
		}
		if err := tx.Create(item).Error; err != nil {
			return translateError(err)
		}
		binding.ID = item.ID
		binding.UserID = item.UserID
		binding.CreatedAt = item.CreatedAt
		binding.UpdatedAt = item.UpdatedAt
		return nil
	})
	return translateError(err)
}

// RevokeActiveMiniAppSessions keeps Mini Program cold-start login bounded
// without touching browser sessions.
func (r *Repo) RevokeActiveMiniAppSessions(ctx context.Context, userID uint, userAgentPrefix string, now time.Time) error {
	prefix := strings.TrimSpace(userAgentPrefix)
	if userID == 0 || prefix == "" {
		return repository.ErrInvalidInput
	}
	return translateError(r.db.WithContext(ctx).Model(&models.UserSession{}).
		Where("user_id = ? AND revoked_at IS NULL AND user_agent LIKE ?", userID, prefix+"%").
		Updates(map[string]interface{}{"revoked_at": now, "revoke_reason": "wechat_miniapp_relogin"}).Error)
}

func toDomainMiniAppBinding(item models.WeChatMiniAppBinding) *domainwechatminiapp.Binding {
	return &domainwechatminiapp.Binding{
		ID:                item.ID,
		UserID:            item.UserID,
		AppID:             item.AppID,
		OpenID:            item.OpenID,
		UnionID:           item.UnionID,
		UnionIDObservedAt: item.UnionIDObservedAt,
		LastLoginAt:       item.LastLoginAt,
		RevokedAt:         item.RevokedAt,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
	}
}
