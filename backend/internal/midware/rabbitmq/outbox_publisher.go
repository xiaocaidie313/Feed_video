package rabbitmq

import (
	"context"
	"errors"
	"feedvid/internal/eventstore"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
)

type OutboxPublisher struct{ base *RabbitMQ }

func NewOutboxPublisher(base *RabbitMQ) (*OutboxPublisher, error) {
	if base == nil {
		return nil, errors.New("RabbitMQ is unavailable")
	}
	ch, err := base.NewChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()
	if err := declareOutboxTopology(ch); err != nil {
		return nil, err
	}
	return &OutboxPublisher{base: base}, nil
}
func declareOutboxTopology(ch *amqp.Channel) error {
	for _, route := range []struct{ exchange, queue, binding string }{
		{"like.events", "like.events", "like.*"},
		{"like.events", "notification.like", "like.like"},
		{"video.timeline.events", "video.timeline.update.queue", "video.timeline.*"},
	} {
		if err := DeclareTopicTopology(ch, route.exchange, route.queue, route.binding); err != nil {
			return err
		}
	}
	return nil
}
func (p *OutboxPublisher) Publish(ctx context.Context, message eventstore.Message) error {
	ch, err := p.base.NewChannel()
	if err != nil {
		return err
	}
	defer ch.Close()
	// Also recreate durable topology after a broker connection is replaced.
	if err := declareOutboxTopology(ch); err != nil {
		return err
	}
	if err := ch.Confirm(false); err != nil {
		return err
	}
	returned := ch.NotifyReturn(make(chan amqp.Return, 1))
	confirmation, err := ch.PublishWithDeferredConfirmWithContext(ctx, message.Exchange, message.RoutingKey, true, false, amqp.Publishing{
		MessageId: message.EventID, ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: message.Payload,
	})
	if err != nil {
		return err
	}
	if confirmation == nil {
		return errors.New("publisher confirmation unavailable")
	}
	ack, err := confirmation.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !ack {
		return errors.New("broker negatively acknowledged event")
	}
	select {
	case result := <-returned:
		return fmt.Errorf("event was unroutable: %d %s", result.ReplyCode, result.ReplyText)
	default:
		return nil
	}
}
