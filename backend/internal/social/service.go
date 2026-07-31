package social

type SocialService struct {
	repo *SocialRepository
}

func NewSocialService(repo *SocialRepository) *SocialService {
	return &SocialService{repo: repo}
}
