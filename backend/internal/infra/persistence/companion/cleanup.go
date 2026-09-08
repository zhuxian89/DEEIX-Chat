package companion

import (
	"context"

	"gorm.io/gorm"
)

// PurgeDeletedUsers removes only this feature's data. Account deletion remains
// upstream-owned; no delete/auth handler or identity schema needs a feature hook.
func (s *Store) PurgeDeletedUsers(ctx context.Context) error {
	if !s.db.Migrator().HasTable("identity_users") {
		return nil
	}
	var ids []uint
	if err := s.db.WithContext(ctx).Model(&profileRecord{}).
		Where("NOT EXISTS (SELECT 1 FROM identity_users WHERE identity_users.id = companion_profiles.user_id)").
		Order("user_id ASC").Limit(100).Pluck("user_id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id IN ?", ids).Delete(&initiativeRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&initiativeStateRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&memoryRecord{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id IN ?", ids).Delete(&profileRecord{}).Error
	})
}
