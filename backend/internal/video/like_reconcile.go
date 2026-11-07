package video

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReconcileCounts repairs historical counters without inventing user actions.
func (r *LikeRepo) ReconcileCounts(ctx context.Context) (int, error) {
	repaired := 0
	var after uint
	for {
		var batch []Video
		if err := r.db.WithContext(ctx).Select("id").Where("id > ?", after).Order("id ASC").Limit(200).Find(&batch).Error; err != nil {
			return repaired, err
		}
		if len(batch) == 0 {
			return repaired, nil
		}
		for _, row := range batch {
			changed := false
			err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var v Video
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "likes_count").First(&v, row.ID).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return nil
					}
					return err
				}
				var actual int64
				if err := tx.Model(&Like{}).Where("video_id = ?", v.ID).Count(&actual).Error; err != nil {
					return err
				}
				if v.LikesCount == actual {
					return nil
				}
				if err := tx.Model(&Video{}).Where("id = ?", v.ID).UpdateColumn("likes_count", actual).Error; err != nil {
					return err
				}
				changed = true
				return nil
			})
			if err != nil {
				return repaired, err
			}
			if changed {
				repaired++
			}
			after = row.ID
		}
	}
}
