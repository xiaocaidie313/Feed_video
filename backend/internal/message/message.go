package message

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Message struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SenderID   uint      `gorm:"index;not null" json:"sender_id"`
	ReceiverID uint      `gorm:"index;not null" json:"receiver_id"`
	Content    string    `gorm:"type:text" json:"content"`
	IsRead     bool      `gorm:"default:false" json:"is_read"`
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"created_at"`
}

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Send(c *gin.Context) {
	c.JSON(501, gin.H{"error": "message send not implemented"})
}

func (h *Handler) List(c *gin.Context) {
	c.JSON(501, gin.H{"error": "message list not implemented"})
}
