package video

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type VideoRepo struct {
	db *gorm.DB
}

func NewVideoRepo(db *gorm.DB) *VideoRepo {
	return &VideoRepo{db: db}
}

func (vr *VideoRepo) CreateVideo(ctx context.Context, video *Video) error {
	if err := vr.db.WithContext(ctx).Create(video).Error; err != nil {
		return err
	}
	return nil
}

func (vr *VideoRepo) CreateMsg(ctx context.Context, Msg *OutboxMsg) error {
	if err := vr.db.WithContext(ctx).Create(Msg).Error; err != nil {
		return err
	}
	return nil
}

func (vr *VideoRepo) DeleteVideo(ctx context.Context, videoID uint) error {
	if err := vr.db.WithContext(ctx).Delete(&Video{}, videoID).Error; err != nil {
		return err
	}
	return nil
}

func (vr *VideoRepo) ListByAuthorID(ctx context.Context, authorID int64) ([]Video, error) {
	var videos []Video
	if err := vr.db.WithContext(ctx).
		Where("author_id = ?", authorID).
		Order("create_time desc").
		Limit(200).
		Find(&videos).Error; err != nil {
		return nil, err
	}
	return videos, nil
}

func (vr *VideoRepo) GetDetail(ctx context.Context, videoID uint) (*Video, error) {
	var video Video
	if err := vr.db.WithContext(ctx).First(&video, videoID).Error; err != nil {
		return nil, err
	}
	return &video, nil
}

func (vr *VideoRepo) GetByID(ctx context.Context, id uint) (*Video, error) {
	var video Video
	if err := vr.db.WithContext(ctx).First(&video, id).Error; err != nil {
		return nil, err
	}
	return &video, nil
}

func (vr *VideoRepo) IsExist(ctx context.Context, id uint) (bool, error) {
	var video Video
	if err := vr.db.WithContext(ctx).First(&video, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (vr *VideoRepo) UpdatePopularity(ctx context.Context, id uint, change int64) error {
	if err := vr.db.WithContext(ctx).Model(&Video{}).
		Where("id = ?", id).
		Update("popularity", gorm.Expr("popularity + ?", change)).Error; err != nil {
		return err
	}
	return nil
}
func (vr *VideoRepo) ChangeLikesCount(ctx context.Context, id uint, change int64) error {
	if err := vr.db.WithContext(ctx).Model(&Video{}).
		Where("id = ?", id).
		UpdateColumn("likes_count", gorm.Expr("GREATEST(likes_count + ?, 0)", change)).Error; err != nil {
		return err
	}
	return nil
}

func (vr *VideoRepo) ChangePopularity(ctx context.Context, id uint, change int64) error {
	if err := vr.db.WithContext(ctx).Model(&Video{}).
		Where("id = ?", id).
		UpdateColumn("popularity", gorm.Expr("GREATEST(popularity + ?, 0)", change)).Error; err != nil {
		return err
	}
	return nil
}

func (vr *VideoRepo) CountByAuthor(ctx context.Context, authorID uint) (int64, error) {
	var count int64
	if err := vr.db.WithContext(ctx).Model(&Video{}).Where("author_id = ?", authorID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (vr *VideoRepo) TotalLikesByAuthor(ctx context.Context, authorID uint) (int64, error) {
	var total int64
	if err := vr.db.WithContext(ctx).Model(&Video{}).Where("author_id = ?", authorID).Select("COALESCE(SUM(likes_count), 0)").Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}
func (vr *VideoRepo) UpdateLikesCount(ctx context.Context, id uint, likesCount int64) error {
	if err := vr.db.WithContext(ctx).Model(&Video{}).
		Where("id = ?", id).
		UpdateColumn("likes_count", likesCount).Error; err != nil {
		return err
	}
	return nil
}
