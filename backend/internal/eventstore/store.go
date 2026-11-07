package eventstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type Message struct {
	ID            uint      `gorm:"primaryKey"`
	EventID       string    `gorm:"size:64;uniqueIndex;default:null"`
	VideoID       uint      `gorm:"index"`
	EventType     string    `gorm:"size:50"`
	Exchange      string    `gorm:"size:128"`
	RoutingKey    string    `gorm:"size:128"`
	Payload       []byte    `gorm:"type:json"`
	CreateTime    time.Time `gorm:"autoCreateTime"`
	Status        string    `gorm:"size:50;index;default:pending"`
	Attempts      int
	NextAttemptAt *time.Time `gorm:"index"`
	LastError     string     `gorm:"size:512"`
}

func (Message) TableName() string { return "outbox_msgs" }

type Receipt struct {
	Consumer  string `gorm:"primaryKey;size:64"`
	EventID   string `gorm:"primaryKey;size:64"`
	CreatedAt time.Time
}

// Once must run inside the same transaction as the business effect.
func Once(tx *gorm.DB, consumer, id string, apply func(*gorm.DB) error) (bool, error) {
	if id == "" {
		return false, errors.New("event_id is required")
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Receipt{Consumer: consumer, EventID: id})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	return true, apply(tx)
}

type Publisher interface {
	Publish(context.Context, Message) error
}
type Dispatcher struct {
	db        *gorm.DB
	publisher Publisher
}

func NewDispatcher(db *gorm.DB, publisher Publisher) *Dispatcher {
	return &Dispatcher{db: db, publisher: publisher}
}
func prepare(m *Message) error {
	if m.EventID == "" {
		m.EventID = fmt.Sprintf("outbox-%d", m.ID)
	}
	// Old pending timeline rows are upgraded deterministically on every retry.
	if len(m.Payload) == 0 && m.EventType == "video_published" {
		m.Exchange, m.RoutingKey = "video.timeline.events", "video.timeline.publish"
		payload := struct {
			EventID    string    `json:"event_id"`
			VideoID    uint      `json:"video_id"`
			CreateTime int64     `json:"create_time"`
			OccurredAt time.Time `json:"occurred_at"`
		}{m.EventID, m.VideoID, m.CreateTime.UnixMilli(), m.CreateTime}
		var err error
		m.Payload, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}
	if m.Exchange == "" || m.RoutingKey == "" || !json.Valid(m.Payload) {
		return errors.New("outbox event has no valid destination or payload")
	}
	return nil
}
func (d *Dispatcher) DispatchOne(ctx context.Context) (bool, error) {
	if d == nil || d.db == nil || d.publisher == nil {
		return false, errors.New("outbox dispatcher is not initialized")
	}
	found := false
	var publishErr error
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pending []Message
		result := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)", "pending", time.Now().UTC()).Order("id ASC").Limit(1).Find(&pending)
		if result.Error != nil {
			return result.Error
		}
		if len(pending) == 0 {
			return nil
		}
		m := pending[0]

		found = true
		publishErr = prepare(&m)
		if publishErr == nil {
			publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			publishErr = d.publisher.Publish(publishCtx, m)
			cancel()
		}
		if publishErr != nil {
			delay := time.Second * time.Duration(1<<min(m.Attempts, 6))
			next := time.Now().UTC().Add(delay)
			message := publishErr.Error()
			if len(message) > 512 {
				message = message[:512]
			}
			return tx.Model(&m).Updates(map[string]interface{}{"event_id": m.EventID, "attempts": m.Attempts + 1, "next_attempt_at": next, "last_error": message}).Error
		}
		return tx.Delete(&m).Error
	})
	if err != nil {
		return found, err
	}
	return found, publishErr
}
