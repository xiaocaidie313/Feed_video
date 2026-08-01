package video

import "time"

// @todo 是否需要的 回复评论的结构体
type Comment struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"index" json:"username"`
	VideoID   uint      `gorm:"index" json:"video_id"`
	AuthorID  uint      `gorm:"index" json:"author_id"`
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

type PublishCommentRequest struct {
	Comment string
	VedioID uint
}

type DeleteCommentRequest struct {
	CommentID uint
}

type GetAllCommentsRequest struct {
	VideoID uint
}
