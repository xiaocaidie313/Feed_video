package redis

import (
	"feedvid/internal/config"
	"strconv"

	redis "github.com/redis/go-redis/v9"
)

// 定义 client 结构体
type Client struct {
	rdb       *redis.Client // redis client
	keyPrefix string        // 版本隔离
}

const defaultKeyPrefix = "v1:"

// 内部创建
func NewClient(rdb *redis.Client, keyPrefix string) *Client {
	if keyPrefix == "" {
		keyPrefix = defaultKeyPrefix
	}
	return &Client{
		rdb:       rdb,
		keyPrefix: keyPrefix,
	}
}

// 环境配置创建
func NewFromEnv(cfg *config.RedisConfig) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Host + ":" + strconv.Itoa(cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return &Client{rdb: rdb, keyPrefix: defaultKeyPrefix}, nil
}
