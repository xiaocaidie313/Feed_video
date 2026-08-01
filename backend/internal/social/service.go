package social

import (
	"context"
	"errors"
	"feedvid/internal/account"
)

type SocialService struct {
	repo        *SocialRepository
	accountRepo *account.AccountRepo
}

func NewSocialService(repo *SocialRepository) *SocialService {
	return &SocialService{repo: repo}
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
	return ss.repo.Follow(ctx, social)
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
	return ss.repo.Unfollow(ctx, social)
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
