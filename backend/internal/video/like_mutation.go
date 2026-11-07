package video

import (
	"context"
	"encoding/json"
	"errors"
	"feedvid/internal/apierror"
	"feedvid/internal/eventstore"
	"feedvid/internal/midware/rabbitmq"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// ChangeLike serializes changes to one video's counters. The unique like row,
// counters and outbox event commit or roll back together.
func (r *LikeRepo) ChangeLike(ctx context.Context, videoID, accountID uint, wanted bool, sourceEventID string) (bool, error) {
	if videoID == 0 || accountID == 0 {
		return false, apierror.ErrValidation
	}
	changed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		apply := func(tx *gorm.DB) error {
			var v Video
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&v, videoID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apierror.ErrNotFound
				}
				return err
			}
			var delta int64
			action := "unlike"
			if wanted {
				result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Like{VideoID: videoID, AccountID: accountID})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected == 0 {
					return nil
				}
				delta, action = 1, "like"
			} else {
				result := tx.Where("video_id = ? AND account_id = ?", videoID, accountID).Delete(&Like{})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected == 0 {
					return nil
				}
				delta = -1
			}
			if err := tx.Model(&Video{}).Where("id = ?", videoID).Updates(map[string]interface{}{
				"likes_count": gorm.Expr("GREATEST(likes_count + ?, 0)", delta),
				"popularity":  gorm.Expr("GREATEST(popularity + ?, 0)", delta),
			}).Error; err != nil {
				return err
			}
			event := rabbitmq.LikeEvent{EventID: uuid.NewString(), Action: action, UserID: accountID, VideoID: videoID, OccurredAt: time.Now().UTC(), Committed: true}
			payload, err := json.Marshal(event)
			if err != nil {
				return err
			}
			message := OutboxMsg{EventID: event.EventID, VideoID: videoID, EventType: "like_changed", Exchange: "like.events", RoutingKey: "like." + action, Payload: payload, CreateTime: event.OccurredAt, Status: "pending"}
			if err := tx.Create(&message).Error; err != nil {
				return err
			}
			changed = true
			return nil
		}
		if sourceEventID != "" {
			_, err := eventstore.Once(tx, "legacy-like", sourceEventID, apply)
			return err
		}
		return apply(tx)
	})
	return changed, err
}
