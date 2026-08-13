package rabbitmq

import (
	"context"
	"errors"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (

	// 交换机  + binding routing key
	commentExchange   = "comment.events"
	commentQueue      = "comment.events"
	commentBindingKey = "comment.*"

	// routing key 反映了在当前业务下的动作 种类/个数
	commentPublishRk = "comment.publish"
	commentDeleteRk  = "comment.delete"
)

// 一个mq 配备一个 ch
type CommentMQ struct {
	ch *amqp.Channel
}

type CommentEvent struct {
	EventID    string    `json:"event_id"`
	Action     string    `json:"action"`
	CommentID  uint      `json:"comment_id,omitempty"`
	Username   string    `json:"username,omitempty"`
	VideoID    uint      `json:"video_id,omitempty"`
	AuthorID   uint      `json:"author_id,omitempty"`
	Content    string    `json:"content,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

// 特殊的MQ 都由  base MQ 转化
func NewCommentMQ(base *RabbitMQ) (*CommentMQ, error) {
	// MQ 提供 connection 然后 通过函数封装成 channel
	if base == nil {
		return nil, errors.New("base MQ is nil")
	}
	ch, err := base.NewChannel()
	if err != nil {
		return nil, err
	}
	// 链接exchange 和 queue
	if err := DeclareTopicTopology(ch, commentExchange, commentQueue, commentBindingKey); err != nil {
		ch.Close()
		return nil, err
	}
	return &CommentMQ{ch: ch}, nil
}

func (cm *CommentMQ) Publish(ctx context.Context, username string, videoID, authorID uint, content string) error {
	return cm.publish(ctx, commentPublishRk, commentPublishRk, CommentEvent{
		Username: username,
		VideoID:  videoID,
		AuthorID: authorID,
		Content:  content,
	})
}
func (cm *CommentMQ) Delete(ctx context.Context, commentid uint) error {
	return cm.publish(ctx, commentDeleteRk, commentDeleteRk, CommentEvent{
		CommentID: commentid,
	})
}

func (cm *CommentMQ) publish(ctx context.Context, action, routingKey string, evt CommentEvent) error {
	if cm == nil || cm.ch == nil {
		return errors.New("comment mq is not initialized")
	}
	id, err := newEventID(16)
	if err != nil {
		return err
	}
	evt.EventID = id
	evt.Action = action
	evt.OccurredAt = time.Now().UTC()
	return PublishJSON(ctx, cm.ch, commentExchange, routingKey, evt)
}
