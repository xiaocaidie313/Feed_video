package worker

import (
	"context"
	"encoding/json"
	"errors"
	"feedvid/internal/apierror"
	"feedvid/internal/midware/rabbitmq"
	rediscache "feedvid/internal/midware/redis"
	"feedvid/internal/video"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type LikeWorker struct {
	cache  *rediscache.Client
	ch     *amqp.Channel
	likes  *video.LikeRepo
	videos *video.VideoRepo
	queue  string
}

func NewLikeWorker(ch *amqp.Channel, likes *video.LikeRepo, videos *video.VideoRepo, queue string, caches ...*rediscache.Client) *LikeWorker {
	w := &LikeWorker{ch: ch, likes: likes, videos: videos, queue: queue}
	if len(caches) > 0 {
		w.cache = caches[0]
	}
	return w
}

func (w *LikeWorker) Run(ctx context.Context) error {
	if w == nil || w.ch == nil || w.likes == nil || w.videos == nil {
		return errors.New("like worker is not initialized")
	}
	if w.queue == "" {
		return errors.New("queue is required")
	}

	deliveries, err := w.ch.Consume(
		w.queue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("deliveries channel closed")
			}
			w.handleDelivery(ctx, d)
		}
	}
}

func (w *LikeWorker) handleDelivery(ctx context.Context, d amqp.Delivery) {
	const maxRetries = 3
	for i := 0; i <= maxRetries; i++ {
		select {
		case <-ctx.Done():
			_ = d.Nack(false, true)
			return
		default:
		}
		if err := w.Process(ctx, d.Body); err != nil {
			if i >= maxRetries {
				log.Printf("like worker: 重试 %d 次后仍失败, 进入死信: %v", maxRetries, err)
				_ = d.Nack(false, false)
				return
			}
			wait := time.Duration(1<<uint(i)) * time.Second
			log.Printf("like worker: 处理失败, %v 后重试 (%d/%d): %v", wait, i+1, maxRetries, err)
			time.Sleep(wait)
			continue
		}
		_ = d.Ack(false)
		return
	}
}

func (w *LikeWorker) Process(ctx context.Context, body []byte) error {
	var evt rabbitmq.LikeEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return err
	}
	if evt.EventID == "" || evt.UserID == 0 || evt.VideoID == 0 {
		return errors.New("invalid like event")
	}
	if evt.Action != "like" && evt.Action != "unlike" {
		return errors.New("invalid like action")
	}
	if !evt.Committed {
		_, err := w.likes.ChangeLike(ctx, evt.VideoID, evt.UserID, evt.Action == "like", evt.EventID)
		if errors.Is(err, apierror.ErrNotFound) {
			return nil
		}
		return err
	}
	if w.cache == nil {
		return nil
	}
	delta := int64(1)
	if evt.Action == "unlike" {
		delta = -1
	}
	return w.cache.ApplyPopularityEvent(ctx, evt.EventID, evt.VideoID, delta, evt.OccurredAt)
}
