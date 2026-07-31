package social

type SocialHander struct {
	service *SocialService
}

func NewSocialHandler(service *SocialService) *SocialHander {
	return &SocialHander{
		service: service,
	}
}
