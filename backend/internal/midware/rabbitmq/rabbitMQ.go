package rabbitmq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"feedvid/internal/config"
	"log"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQ 只管理 Connection，Channel 由各组件按需创建
type RabbitMQ struct {
	Conn *amqp.Connection
}

// 构建 基础能力 基础连接
func NewRabbitMQ(cfg *config.RabbitMQConfig) (*RabbitMQ, error) {
	if cfg == nil {
		return nil, errors.New("rabbitmq config is nil")
	}
	url := CreateRabbitMQurl(cfg)
	// 建立连接
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	return &RabbitMQ{Conn: conn}, nil
}

func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	if r.Conn != nil {
		return r.Conn.Close()
	}
	return nil
}

func (r *RabbitMQ) NewChannel() (*amqp.Channel, error) {
	if r == nil || r.Conn == nil {
		return nil, errors.New("rabbitmq connection is not initialized")
	}
	return r.Conn.Channel()
}

// 声明 Topic 拓扑结构
// exchange: 交换机名称
// queue: 队列名称
// bindingKey: 绑定键
//
//	用于确定 交换机和队列之间的绑定关系
func DeclareTopicTopology(ch *amqp.Channel, exchange string, queue string, bindingKey string) error {
	if ch == nil {
		return errors.New("channel is not initialized")
	}
	if exchange == "" || queue == "" || bindingKey == "" {
		return errors.New("exchange/queue/bindingKey is required")
	}

	if err := ch.ExchangeDeclare(
		exchange, // 1. name
		"topic",  // 2. kind
		true,     // 3. durable   持久化   重启后是否存在 是否需要重新创建
		false,    // 4. autoDelete  当不在需要的时候  是否自动删除
		false,    // 5. internal  表示生产者可以直接向这个 Exchange 发布消息。 是否只能在内部使用
		false,    // 6. noWait
		nil,      // 7. arguments
	); err != nil {
		return err
	}

	q, err := ch.QueueDeclare(
		queue, // 1. name
		true,  // 2. durable
		false, // 3. autoDelete
		false, // 4. exclusive   表示该队列不属于当前 Connection 独占，可以被其他 Connection 使用。
		false, // 5. noWait
		amqp.Table{
			"x-dead-letter-exchange": DLXExchange,
		}, // 6. arguments
	)
	if err != nil {
		return err
	}

	if err := ch.QueueBind(
		q.Name,
		bindingKey,
		exchange,
		false,
		nil,
	); err != nil {
		return err
	}
	if err := DeclareDLX(ch, queue); err != nil {
		log.Printf("DLX declare failed for %s: %v", queue, err)
	}
	return nil
}

func PublishJSON(ctx context.Context, ch *amqp.Channel, exchange string, routingKey string, payload interface{}) error {
	if ch == nil {
		return errors.New("channel is not initialized")
	}
	if exchange == "" || routingKey == "" {
		return errors.New("exchange and routingKey are required")
	}
	// 序列化 payload 为 JSON 字节数组 []byte
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         b,
	})
}

func newEventID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func CreateRabbitMQurl(cfg *config.RabbitMQConfig) string {
	url := "amqp://" + cfg.Username + ":" + cfg.Password + "@" + cfg.Host + ":" + strconv.Itoa(cfg.Port) + "/"
	return url
}
