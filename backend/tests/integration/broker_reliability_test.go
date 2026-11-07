//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"feedvid/internal/config"
	appdb "feedvid/internal/db"
	"feedvid/internal/eventstore"
	"feedvid/internal/midware/rabbitmq"
	"feedvid/internal/video"
	"feedvid/internal/worker"
	amqp "github.com/rabbitmq/amqp091-go"
	"os"
	"testing"
	"time"
)

func TestConfirmedOutboxAndBrokerRedelivery(t *testing.T) {
	db := database(t)
	v := fixtureVideo(t, db)
	cache := redisClient(t)
	raw := os.Getenv("INTEGRATION_AMQP_URL")
	if raw == "" {
		raw = "amqp://feedvid:feedvid_mq_dev_password@127.0.0.1:15672/"
	}
	uri, err := amqp.ParseURI(raw)
	if err != nil {
		t.Fatal(err)
	}
	base, err := rabbitmq.NewRabbitMQ(&config.RabbitMQConfig{Host: uri.Host, Port: uri.Port, Username: uri.Username, Password: uri.Password})
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	publisher, err := rabbitmq.NewOutboxPublisher(base)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := base.NewChannel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	queue, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.QueueBind(queue.Name, "like.like", "like.events", false, nil); err != nil {
		t.Fatal(err)
	}
	deliveries, err := ch.Consume(queue.Name, "", false, true, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := video.NewLikeRepo(db).ChangeLike(context.Background(), v.ID, 2, true, ""); err != nil {
		t.Fatal(err)
	}
	var row eventstore.Message
	if err := db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if found, err := eventstore.NewDispatcher(db, publisher).DispatchOne(ctx); !found || err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	likes := worker.NewLikeWorker(nil, video.NewLikeRepo(db), video.NewVideoRepo(db), "test", cache)
	notifications := worker.NewNotificationWorker(nil, db, "test", nil)
	for pass := 0; pass < 2; pass++ {
		select {
		case delivery := <-deliveries:
			if delivery.MessageId != row.EventID {
				t.Fatal("broker message lost its stable ID")
			}
			if pass == 1 && !delivery.Redelivered {
				t.Fatal("expected broker redelivery")
			}
			if err := likes.Process(ctx, delivery.Body); err != nil {
				t.Fatal(err)
			}
			if err := notifications.Process(ctx, delivery); err != nil {
				t.Fatal(err)
			}
			if pass == 0 {
				if err := delivery.Nack(false, true); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := delivery.Ack(false); err != nil {
					t.Fatal(err)
				}
			}
		case <-ctx.Done():
			t.Fatal("broker delivery timed out")
		}
	}
	assertLikes(t, db, v.ID, 1)
	var count int64
	db.Model(&worker.Notification{}).Count(&count)
	if count != 1 {
		t.Fatalf("broker redelivery created %d notifications", count)
	}
	db.Model(&eventstore.Message{}).Count(&count)
	if count != 0 {
		t.Fatal("confirmed outbox was not cleared")
	}
}
func TestOldOutboxSchemaMigratesPendingRows(t *testing.T) {
	db := database(t)
	if err := db.Exec("DROP TABLE outbox_msgs").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE outbox_msgs (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, video_id BIGINT UNSIGNED, event_type VARCHAR(50),create_time DATETIME(3),status VARCHAR(50))").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO outbox_msgs(video_id,event_type,create_time,status) VALUES (1,'video_published',UTC_TIMESTAMP(3),'pending'),(2,'video_published',UTC_TIMESTAMP(3),'pending')").Error; err != nil {
		t.Fatal(err)
	}
	if err := appdb.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	publisher := &recordingPublisher{}
	dispatcher := eventstore.NewDispatcher(db, publisher)
	for range 2 {
		if found, err := dispatcher.DispatchOne(context.Background()); !found || err != nil {
			t.Fatalf("migrated pending row failed: %v", err)
		}
	}
	if len(publisher.messages) != 2 || publisher.messages[0].EventID == publisher.messages[1].EventID {
		t.Fatal("legacy rows have colliding event IDs")
	}
	for _, message := range publisher.messages {
		var event rabbitmq.TimelineEvent
		if err := json.Unmarshal(message.Payload, &event); err != nil || event.EventID == "" {
			t.Fatal("invalid legacy event")
		}
	}
}
