package worker

import (
	"context"
	"feedvid/internal/eventstore"
	"feedvid/internal/midware/rabbitmq"
	"gorm.io/gorm"
	"log"
	"time"
)

func StartOutboxPoller(ctx context.Context, db *gorm.DB, mq *rabbitmq.RabbitMQ) {
	if db == nil || mq == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		var dispatcher *eventstore.Dispatcher
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if dispatcher == nil {
				publisher, err := rabbitmq.NewOutboxPublisher(mq)
				if err != nil {
					log.Printf("outbox publisher unavailable: %v", err)
					continue
				}
				dispatcher = eventstore.NewDispatcher(db, publisher)
			}
			for range 20 {
				found, err := dispatcher.DispatchOne(ctx)
				if err != nil {
					log.Printf("outbox dispatch retained for retry: %v", err)
				}
				if !found || ctx.Err() != nil {
					break
				}
			}
		}
	}()
}
