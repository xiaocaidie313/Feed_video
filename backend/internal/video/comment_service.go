package video

import "context"

type CommentService struct {
	repo *CommentRepo
}

func NewCommentService(repo *CommentRepo) *CommentService {
	return &CommentService{repo: repo}
}

func (s *CommentService) CreateComment(ctx context.Context, req *PublishCommentRequest) error {
	return s.repo.CreateComment(ctx, req)
}