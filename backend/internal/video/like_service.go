package video

import (
	"context"
	"feedvid/internal/apierror"
	rediscache "feedvid/internal/midware/redis"
	"time"
)

type LikeService struct {
	repo      *LikeRepo
	VideoRepo *VideoRepo
	cache     *rediscache.Client
}

func NewLikeService(repo *LikeRepo, videoRepo *VideoRepo, cache *rediscache.Client) *LikeService {
	return &LikeService{repo: repo, VideoRepo: videoRepo, cache: cache}
}
func (s *LikeService) change(ctx context.Context, like *Like, wanted bool) error {
	if like == nil {
		return apierror.ErrValidation
	}
	changed, err := s.repo.ChangeLike(ctx, like.VideoID, like.AccountID, wanted, "")
	if err != nil {
		return err
	}
	if changed && s.cache != nil {
		// Best effort immediate invalidation; the durable event repeats it later.
		cacheCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = s.cache.Del(cacheCtx, s.cache.Key("video:detail:id=%d", like.VideoID))
		_ = s.cache.Del(cacheCtx, s.cache.Key("video:entity:%d", like.VideoID))
	}
	return nil
}
func (s *LikeService) Like(ctx context.Context, like *Like) error { return s.change(ctx, like, true) }
func (s *LikeService) Unlike(ctx context.Context, like *Like) error {
	return s.change(ctx, like, false)
}
func (s *LikeService) IsLiked(ctx context.Context, videoID, accountID uint) (bool, error) {
	return s.repo.IsLiked(ctx, videoID, accountID)
}
func (s *LikeService) ListLikedVideos(ctx context.Context, accountID uint) ([]Video, error) {
	return s.repo.ListLikedVideos(ctx, accountID)
}
