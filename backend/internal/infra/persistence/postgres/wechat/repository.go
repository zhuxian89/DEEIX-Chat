package wechat

import (
	"context"
	"errors"
	"strings"
	"time"

	domainregistrationcode "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/registrationcode"
	domainwechat "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/wechat"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/dberror"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"gorm.io/gorm"
)

const (
	settingsNamespace       = "wechat"
	adminContactSettingKey  = "admin_contact"
	adminContactDescription = "微信公众号管理员联系方式"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func translateError(err error) error {
	if err == nil {
		return nil
	}
	if dberror.IsRecordNotFound(err) {
		return repository.ErrNotFound
	}
	if dberror.IsUniqueConstraint(err) {
		return repository.ErrDuplicate
	}
	return err
}

// IssueRegistrationCode is idempotent for an OpenID and atomic for first issuance.
func (r *Repo) IssueRegistrationCode(ctx context.Context, openID, code string) (string, error) {
	result, err := r.IssueRegistrationCodeDetailed(ctx, openID, code)
	return result.Code, err
}

func (r *Repo) IssueRegistrationCodeDetailed(ctx context.Context, openID, code string) (domainwechat.IssueResult, error) {
	var result domainwechat.IssueResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = issueRegistrationCodeTx(tx, openID, code)
		return err
	})
	return result, err
}

func (r *Repo) IssueRegistrationCodeWithInvocationLog(ctx context.Context, openID, code string, log *domainwechat.InvocationLog) (domainwechat.IssueResult, error) {
	var result domainwechat.IssueResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = issueRegistrationCodeTx(tx, openID, code)
		if err != nil {
			return err
		}
		log.RegistrationCodeID = result.RegistrationCodeID
		if result.Created {
			log.Result = domainwechat.ResultIssued
		} else {
			log.Result = domainwechat.ResultReplayed
		}
		entry := &model.WeChatKeywordInvocationLog{OpenID: log.OpenID, Keyword: log.Keyword, Action: log.Action, TemplateID: log.TemplateID, RegistrationCodeID: log.RegistrationCodeID, Result: log.Result, ErrorCode: log.ErrorCode, ErrorMessage: log.ErrorMessage}
		if err := tx.Create(entry).Error; err != nil {
			return err
		}
		log.ID, log.CreatedAt = entry.ID, entry.CreatedAt
		return nil
	})
	return result, translateError(err)
}

func issueRegistrationCodeTx(tx *gorm.DB, openID, code string) (domainwechat.IssueResult, error) {
	var existing model.WeChatRegistrationIssuance
	if err := tx.Where("open_id = ?", openID).First(&existing).Error; err == nil {
		var registration model.RegistrationCode
		if err := tx.First(&registration, existing.RegistrationCodeID).Error; err != nil {
			return domainwechat.IssueResult{}, err
		}
		result := domainwechat.IssueResult{Code: registration.Code, RegistrationCodeID: registration.ID, Created: false}
		if registration.Status == domainregistrationcode.StatusUsed && registration.UsedByUserID != 0 {
			result.Used = true
			var usedBy model.User
			if err := tx.Unscoped().Where("id = ?", registration.UsedByUserID).First(&usedBy).Error; err != nil {
				if dberror.IsRecordNotFound(err) {
					result.DeletedUser = true
				} else {
					return domainwechat.IssueResult{}, err
				}
			}
		}
		return result, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return domainwechat.IssueResult{}, err
	}

	registration := model.RegistrationCode{
		Code:     code,
		CodeHint: code[len(code)-4:],
		Status:   domainregistrationcode.StatusActive,
	}
	if err := tx.Create(&registration).Error; err != nil {
		if dberror.IsUniqueConstraint(err) {
			return domainwechat.IssueResult{}, repository.ErrDuplicate
		}
		return domainwechat.IssueResult{}, err
	}
	issuance := model.WeChatRegistrationIssuance{OpenID: openID, RegistrationCodeID: registration.ID}
	if err := tx.Create(&issuance).Error; err != nil {
		if dberror.IsUniqueConstraint(err) {
			return domainwechat.IssueResult{}, repository.ErrDuplicate
		}
		return domainwechat.IssueResult{}, err
	}
	return domainwechat.IssueResult{Code: registration.Code, RegistrationCodeID: registration.ID, Created: true}, nil
}

func (r *Repo) GetKeywordRule(ctx context.Context, keyword string) (*domainwechat.KeywordRule, error) {
	var item model.WeChatKeywordRule
	err := r.db.WithContext(ctx).Preload("Template").Where("keyword = ?", strings.TrimSpace(keyword)).First(&item).Error
	if err != nil {
		return nil, translateError(err)
	}
	return toKeywordRule(item), nil
}

func (r *Repo) ListKeywordRules(ctx context.Context, offset, limit int) ([]domainwechat.KeywordRule, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.WeChatKeywordRule{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, translateError(err)
	}
	items := make([]model.WeChatKeywordRule, 0)
	if err := query.Preload("Template").Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, translateError(err)
	}
	result := make([]domainwechat.KeywordRule, 0, len(items))
	for _, item := range items {
		result = append(result, *toKeywordRule(item))
	}
	return result, total, nil
}

func (r *Repo) CreateKeywordRule(ctx context.Context, item *domainwechat.KeywordRule) error {
	dbItem := &model.WeChatKeywordRule{Keyword: strings.TrimSpace(item.Keyword), Action: strings.TrimSpace(item.Action), TemplateID: item.TemplateID, Enabled: item.Enabled}
	if err := r.db.WithContext(ctx).Create(dbItem).Error; err != nil {
		return translateError(err)
	}
	item.ID, item.CreatedAt = dbItem.ID, dbItem.CreatedAt
	return nil
}

func (r *Repo) UpdateKeywordRule(ctx context.Context, id uint, keyword, action string, templateID uint) error {
	result := r.db.WithContext(ctx).Model(&model.WeChatKeywordRule{}).Where("id = ?", id).Updates(map[string]interface{}{"keyword": strings.TrimSpace(keyword), "action": strings.TrimSpace(action), "template_id": templateID})
	if result.Error != nil {
		return translateError(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *Repo) SetKeywordRuleEnabled(ctx context.Context, id uint, enabled bool) error {
	result := r.db.WithContext(ctx).Model(&model.WeChatKeywordRule{}).Where("id = ?", id).Update("enabled", enabled)
	if result.Error != nil {
		return translateError(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *Repo) ListReplyTemplates(ctx context.Context, offset, limit int) ([]domainwechat.ReplyTemplate, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.WeChatReplyTemplate{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, translateError(err)
	}
	items := make([]model.WeChatReplyTemplate, 0)
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, translateError(err)
	}
	result := make([]domainwechat.ReplyTemplate, 0, len(items))
	for _, item := range items {
		result = append(result, toReplyTemplate(item))
	}
	return result, total, nil
}

func (r *Repo) GetReplyTemplate(ctx context.Context, id uint) (*domainwechat.ReplyTemplate, error) {
	var item model.WeChatReplyTemplate
	if err := r.db.WithContext(ctx).First(&item, id).Error; err != nil {
		return nil, translateError(err)
	}
	result := toReplyTemplate(item)
	return &result, nil
}

func (r *Repo) CreateReplyTemplate(ctx context.Context, item *domainwechat.ReplyTemplate) error {
	dbItem := &model.WeChatReplyTemplate{Name: strings.TrimSpace(item.Name), ResponseType: strings.TrimSpace(item.ResponseType), Content: item.Content, Enabled: item.Enabled}
	if err := r.db.WithContext(ctx).Create(dbItem).Error; err != nil {
		return translateError(err)
	}
	item.ID, item.CreatedAt = dbItem.ID, dbItem.CreatedAt
	return nil
}

func (r *Repo) UpdateReplyTemplate(ctx context.Context, id uint, name, responseType, content string) error {
	result := r.db.WithContext(ctx).Model(&model.WeChatReplyTemplate{}).Where("id = ?", id).Updates(map[string]interface{}{"name": strings.TrimSpace(name), "response_type": strings.TrimSpace(responseType), "content": content})
	if result.Error != nil {
		return translateError(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *Repo) SetReplyTemplateEnabled(ctx context.Context, id uint, enabled bool) error {
	result := r.db.WithContext(ctx).Model(&model.WeChatReplyTemplate{}).Where("id = ?", id).Update("enabled", enabled)
	if result.Error != nil {
		return translateError(result.Error)
	}
	if result.RowsAffected == 0 {
		return repository.ErrNotFound
	}
	return nil
}

type issuanceRow struct {
	ID                 uint
	OpenID             string
	RegistrationCodeID uint
	Code               string
	Status             string
	UsedByUserID       uint
	UsedAt             *time.Time
	CreatedAt          time.Time
}

func (r *Repo) ListIssuanceRecords(ctx context.Context, offset, limit int, queryText string) ([]domainwechat.IssuanceRecord, int64, error) {
	base := r.db.WithContext(ctx).Table("wechat_registration_issuances AS i").Joins("JOIN registration_codes AS c ON c.id = i.registration_code_id")
	if queryText = strings.TrimSpace(queryText); queryText != "" {
		like := "%" + queryText + "%"
		base = base.Where("i.open_id LIKE ? OR c.code LIKE ?", like, like)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, translateError(err)
	}
	rows := make([]issuanceRow, 0)
	if err := base.Select("i.id, i.open_id, i.registration_code_id, c.code, c.status, c.used_by_user_id, c.used_at, i.created_at").Order("i.id DESC").Offset(offset).Limit(limit).Scan(&rows).Error; err != nil {
		return nil, 0, translateError(err)
	}
	items := make([]domainwechat.IssuanceRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, domainwechat.IssuanceRecord{ID: row.ID, OpenID: row.OpenID, RegistrationCodeID: row.RegistrationCodeID, Code: row.Code, Status: row.Status, UsedByUserID: row.UsedByUserID, UsedAt: row.UsedAt, CreatedAt: row.CreatedAt})
	}
	return items, total, nil
}

func (r *Repo) ListInvocationLogs(ctx context.Context, offset, limit int, result, action, queryText string) ([]domainwechat.InvocationLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.WeChatKeywordInvocationLog{})
	if result = strings.TrimSpace(result); result != "" {
		query = query.Where("result = ?", result)
	}
	if action = strings.TrimSpace(action); action != "" {
		query = query.Where("action = ?", action)
	}
	if queryText = strings.TrimSpace(queryText); queryText != "" {
		like := "%" + queryText + "%"
		query = query.Where("open_id LIKE ? OR keyword LIKE ?", like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, translateError(err)
	}
	items := make([]model.WeChatKeywordInvocationLog, 0)
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, translateError(err)
	}
	resultItems := make([]domainwechat.InvocationLog, 0, len(items))
	for _, item := range items {
		resultItems = append(resultItems, domainwechat.InvocationLog{ID: item.ID, OpenID: item.OpenID, Keyword: item.Keyword, Action: item.Action, TemplateID: item.TemplateID, RegistrationCodeID: item.RegistrationCodeID, Result: item.Result, ErrorCode: item.ErrorCode, ErrorMessage: item.ErrorMessage, CreatedAt: item.CreatedAt})
	}
	return resultItems, total, nil
}

func (r *Repo) CreateInvocationLog(ctx context.Context, item *domainwechat.InvocationLog) error {
	dbItem := &model.WeChatKeywordInvocationLog{OpenID: item.OpenID, Keyword: item.Keyword, Action: item.Action, TemplateID: item.TemplateID, RegistrationCodeID: item.RegistrationCodeID, Result: item.Result, ErrorCode: item.ErrorCode, ErrorMessage: item.ErrorMessage}
	if err := r.db.WithContext(ctx).Create(dbItem).Error; err != nil {
		return translateError(err)
	}
	item.ID, item.CreatedAt = dbItem.ID, dbItem.CreatedAt
	return nil
}

func (r *Repo) Stats(ctx context.Context) (domainwechat.Stats, error) {
	var stats domainwechat.Stats
	if err := r.db.WithContext(ctx).Model(&model.WeChatRegistrationIssuance{}).Count(&stats.IssuanceCount).Error; err != nil {
		return stats, translateError(err)
	}
	if err := r.db.WithContext(ctx).Model(&model.WeChatKeywordInvocationLog{}).Where("result IN ?", []string{domainwechat.ResultIssued, domainwechat.ResultReplayed, domainwechat.ResultHandled}).Count(&stats.SuccessCount).Error; err != nil {
		return stats, translateError(err)
	}
	if err := r.db.WithContext(ctx).Model(&model.WeChatKeywordInvocationLog{}).Where("result = ?", domainwechat.ResultFailed).Count(&stats.FailureCount).Error; err != nil {
		return stats, translateError(err)
	}
	return stats, nil
}

func (r *Repo) GetAdminContact(ctx context.Context) (string, error) {
	var item model.SystemSetting
	err := r.db.WithContext(ctx).
		Where("namespace = ? AND key = ?", settingsNamespace, adminContactSettingKey).
		First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domainwechat.DefaultAdminContact, nil
	}
	if err != nil {
		return "", translateError(err)
	}
	contact := strings.TrimSpace(item.Value)
	if contact == "" {
		return domainwechat.DefaultAdminContact, nil
	}
	return contact, nil
}

func (r *Repo) SetAdminContact(ctx context.Context, contact string) error {
	item := model.SystemSetting{Namespace: settingsNamespace, Key: adminContactSettingKey}
	err := r.db.WithContext(ctx).
		Where("namespace = ? AND key = ?", settingsNamespace, adminContactSettingKey).
		Assign(model.SystemSetting{
			Value:       strings.TrimSpace(contact),
			ValueType:   "string",
			Description: adminContactDescription,
		}).
		FirstOrCreate(&item).Error
	return translateError(err)
}

func toReplyTemplate(item model.WeChatReplyTemplate) domainwechat.ReplyTemplate {
	return domainwechat.ReplyTemplate{ID: item.ID, Name: item.Name, ResponseType: item.ResponseType, Content: item.Content, Enabled: item.Enabled, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func toKeywordRule(item model.WeChatKeywordRule) *domainwechat.KeywordRule {
	return &domainwechat.KeywordRule{ID: item.ID, Keyword: item.Keyword, Action: item.Action, TemplateID: item.TemplateID, TemplateName: item.Template.Name, TemplateType: item.Template.ResponseType, TemplateContent: item.Template.Content, TemplateEnabled: item.Template.Enabled, Enabled: item.Enabled, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

var _ repository.WeChatRegistrationRepository = (*Repo)(nil)
var _ repository.WeChatDetailedRegistrationRepository = (*Repo)(nil)
var _ repository.WeChatAdminRepository = (*Repo)(nil)
