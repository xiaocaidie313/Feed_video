package social

import (
	"context"
	"errors"
	"feedvid/internal/account"
	"log"

	rediscache "feedvid/internal/midware/redis"
)

type SocialService struct {
	repo        *SocialRepository
	accountRepo *account.AccountRepo
	cache       *rediscache.Client
}

func NewSocialService(repo *SocialRepository, accountRepo *account.AccountRepo, cache *rediscache.Client) *SocialService {
	return &SocialService{repo: repo, accountRepo: accountRepo, cache: cache}
}

func (ss *SocialService) Follow(ctx context.Context, social *Social) error {
	_, err := ss.accountRepo.FindUserByAccountID(ctx, social.FollowerID)
	if err != nil {
		return errors.New("follower not found")
	}
	_, err = ss.accountRepo.FindUserByAccountID(ctx, social.VloggerID)
	if err != nil {
		return errors.New("vlogger not found")
	}

	isFollowed, err := ss.repo.IsFollowed(ctx, social)
	if err != nil {
		return err
	}
	if isFollowed {
		return errors.New("already followed")
	}
	// 先写 DB，确保数据持久化
	if err := ss.repo.Follow(ctx, social); err != nil {
		return err
	}
	// DB 成功后，失效该用户的关注列表缓存
	ss.invalidateFollowingFeedCache(context.Background(), social.FollowerID)
	return nil
}
func (ss *SocialService) UnFollow(ctx context.Context, social *Social) error {
	_, err := ss.accountRepo.FindUserByAccountID(ctx, social.FollowerID)
	if err != nil {
		return errors.New("follower not found")
	}
	_, err = ss.accountRepo.FindUserByAccountID(ctx, social.VloggerID)
	if err != nil {
		return errors.New("vlogger not found")
	}

	// 先写 DB
	if err := ss.repo.Unfollow(ctx, social); err != nil {
		return err
	}

	// 失效缓存
	ss.invalidateFollowingFeedCache(context.Background(), social.FollowerID)
	return nil
}
func (ss *SocialService) CountAllFollowers(ctx context.Context, vloggerID uint) (int64, error) {
	_, err := ss.accountRepo.FindUserByAccountID(ctx, vloggerID)
	if err != nil {
		return 0, errors.New("vlogger not found")
	}
	return ss.repo.CountFollowers(ctx, vloggerID)
}
func (ss *SocialService) CountAllVollgers(ctx context.Context, followerID uint) (int64, error) {
	_, err := ss.accountRepo.FindUserByAccountID(ctx, followerID)
	if err != nil {
		return 0, errors.New("follower not found")
	}
	return ss.repo.CountVloggers(ctx, followerID)
}
func (ss *SocialService) GetAllVollgers(ctx context.Context, followerID uint) ([]*account.Account, error) {
	_, err := ss.accountRepo.FindUserByAccountID(ctx, followerID)
	if err != nil {
		return nil, errors.New("follower not found")
	}
	return ss.repo.GetAllVloggers(ctx, followerID)
}
func (ss *SocialService) GetAllFollowers(ctx context.Context, vloggerID uint) ([]*account.Account, error) {
	_, err := ss.accountRepo.FindUserByAccountID(ctx, vloggerID)
	if err != nil {
		return nil, errors.New("vlogger not found")
	}
	return ss.repo.GetAllFollowers(ctx, vloggerID)
}
func (ss *SocialService) IsFollowed(ctx context.Context, social *Social) (bool, error) {
	_, err := ss.accountRepo.FindUserByAccountID(ctx, social.FollowerID)
	if err != nil {
		return false, errors.New("follower not found")
	}
	_, err = ss.accountRepo.FindUserByAccountID(ctx, social.VloggerID)
	if err != nil {
		return false, errors.New("vlogger not found")
	}
	return ss.repo.IsFollowed(ctx, social)
}

// accountID的following的feed缓存删除旧的缓存
func (ss *SocialService) invalidateFollowingFeedCache(ctx context.Context, accountID uint) {
	if ss.cache == nil {
		return
	}

	pattern := ss.cache.Key("feed:listByFollowing:*:accountID=%d:*", accountID)

	if err := ss.cache.DelByPattern(ctx, pattern); err != nil {
		log.Println("failed to delete following feed cache:", err)
	}
}
