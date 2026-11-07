package video

import (
	"context"
	"encoding/json"
	"errors"
	"feedvid/internal/apierror"
	"feedvid/internal/midware/rabbitmq"
	rediscache "feedvid/internal/midware/redis"
	"fmt"
	"github.com/google/uuid"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

type VideoService struct {
	repo      *VideoRepo
	cache     *rediscache.Client
	cachedTTL time.Duration
}

func NewVideoService(repo *VideoRepo, cache *rediscache.Client) *VideoService {
	return &VideoService{repo: repo, cache: cache, cachedTTL: 10 * time.Minute}
}

func (vs *VideoService) Publish(ctx context.Context, video *Video) error {
	if video == nil {
		return fmt.Errorf("video is nil: %w", apierror.ErrValidation)
	}
	video.Title = strings.TrimSpace(video.Title)
	video.PlayURL = strings.TrimSpace(video.PlayURL)
	video.CoverURL = strings.TrimSpace(video.CoverURL)

	if video.Title == "" {
		return fmt.Errorf("title is required: %w", apierror.ErrValidation)
	}
	if video.PlayURL == "" {
		return fmt.Errorf("play url is required: %w", apierror.ErrValidation)
	}
	if video.CoverURL == "" {
		return fmt.Errorf("cover url is required: %w", apierror.ErrValidation)
	}

	//事务保证视频写入库和消息写入本地消息表的一致性
	err := vs.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(video).Error; err != nil {
			return err
		}

		event := rabbitmq.TimelineEvent{EventID: uuid.NewString(), VideoID: video.ID, CreateTime: video.CreateTime.UnixMilli(), OccurredAt: time.Now().UTC()}
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		msg := OutboxMsg{EventID: event.EventID, VideoID: video.ID, EventType: "video_published", Status: "pending", CreateTime: video.CreateTime,
			Exchange: "video.timeline.events", RoutingKey: "video.timeline.publish", Payload: payload}

		if err := tx.Create(&msg).Error; err != nil {
			return err
		}
		// 获得所有tag标签
		tags := ExtractTags(video.Title + " " + video.Description)
		for _, tagName := range tags {
			var tag Tag
			if err := tx.Where("name = ?", tagName).FirstOrCreate(&tag, Tag{Name: tagName}).Error; err != nil {
				return err
			}
			if err := tx.Create(&VideoTag{VideoID: video.ID, TagID: tag.ID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return err

}

func (vs *VideoService) Delete(ctx context.Context, id uint, authorID uint) error {
	video, err := vs.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if video == nil {
		return errors.New("video not found")
	}
	if video.AuthorID != authorID {
		return apierror.ErrUnauthorized
	}
	if err := vs.repo.DeleteVideo(ctx, id); err != nil {
		return err
	}

	if vs.cache != nil {
		cacheKey := vs.cache.Key("video:detail:id=%d", id)
		// 删除缓存  就算是 请求结束了 也要删除缓存  用新的 context
		_ = vs.cache.Del(context.Background(), cacheKey)
	}
	return nil
}

func (vs *VideoService) ListByAuthorID(ctx context.Context, authorID uint) ([]Video, error) {
	videos, err := vs.repo.ListByAuthorID(ctx, int64(authorID))
	if err != nil {
		return nil, err
	}
	return videos, nil
}

// 真正访问的视频 才会加载 打缓存
func (vs *VideoService) GetDetail(ctx context.Context, id uint) (*Video, error) {
	if vs.cache == nil {
		return vs.repo.GetByID(ctx, id)
	}

	cacheKey := vs.cache.Key("video:detail:id=%d", id)

	// step 1: 读缓存
	v, ok, err := vs.getCachedVideo(ctx, cacheKey)
	if ok {
		return v, nil
	}
	// step 2 获取redis缓存失败 ----- redis内部没有缓存 <------ 从DB拿 然后写入缓存
	if err != nil && vs.cache.IsMiss(err) {
		lockKey := "lock:" + cacheKey
		//向集群发送请求  需要上锁
		opCtx, lockcancel := context.WithTimeout(ctx, 50*time.Millisecond)
		token, locked, lockerr := vs.cache.Lock(opCtx, lockKey, 2*time.Second)
		lockcancel()
		if lockerr == nil && locked {
			defer func() {
				unlockCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				if unlockerr := vs.cache.Unlock(unlockCtx, lockKey, token); unlockerr != nil {
					log.Println("unlock error:", unlockerr)
				}
			}()
			// 如果在上锁期间 别人已经写入缓存 则直接返回
			if v, ok, _ := vs.getCachedVideo(context.Background(), cacheKey); ok {
				return v, nil
			}

			// 上好锁了
			video, err := vs.repo.GetByID(ctx, id)
			if err != nil {
				return nil, err
			}
			setCacheErr := vs.setCachedVideo(ctx, cacheKey, video)
			if setCacheErr != nil {
				log.Printf("cache write failed: %v", setCacheErr)
			}
			return video, nil
		}

		// 如果没抢到锁 则等待20ms 后重试  一共是 1s
		// 轮询过程
		for i := 0; i < 5; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(20 * time.Millisecond):
				if v, ok, _ := vs.getCachedVideo(context.Background(), cacheKey); ok {
					return v, nil
				}
			}
		}
	}

	// Redis 异常 / 等待超时：回源 DB
	return vs.loadDetailAndCache(ctx, id, cacheKey)
}

func (vs *VideoService) loadDetailAndCache(ctx context.Context, id uint, cacheKey string) (*Video, error) {
	video, err := vs.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := vs.setCachedVideo(ctx, cacheKey, video); err != nil {
		log.Println("set video cache error:", err)
	}
	return video, nil
}

func (vs *VideoService) getCachedVideo(ctx context.Context, cacheKey string) (*Video, bool, error) {
	var cacheV Video

	opCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if vs.cache == nil {
		return nil, false, errors.New("redis client not initialized")
	}

	MarshaledVideo, err := vs.cache.GetBytes(opCtx, cacheKey)
	if err != nil {
		return &Video{}, false, err
	}
	if err := json.Unmarshal(MarshaledVideo, &cacheV); err != nil {
		return &Video{}, false, err
	}
	return &cacheV, true, nil
}

func (vs *VideoService) setCachedVideo(ctx context.Context, cacheKey string, video *Video) error {
	if vs.cache == nil || cacheKey == "" {
		return errors.New("redis client not initialized or cache key is empty")
	}
	if video == nil {
		return fmt.Errorf("video is nil: %w", apierror.ErrValidation)
	}

	marshaledVideo, err := json.Marshal(video)
	if err != nil {
		return err
	}
	opCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	return vs.cache.SetBytes(opCtx, cacheKey, marshaledVideo, vs.cachedTTL)
}

func (vs *VideoService) UpdateLikesCount(ctx context.Context, id uint, likesCount int64) error {
	if err := vs.repo.UpdateLikesCount(ctx, id, likesCount); err != nil {
		return err
	}
	return nil
}

func (vs *VideoService) UpdatePopularity(ctx context.Context, id uint, change int64) error {
	if err := vs.repo.UpdatePopularity(ctx, id, change); err != nil {
		return err
	}
	UpdatePopularityCache(ctx, vs.cache, id, change)
	return nil
}
