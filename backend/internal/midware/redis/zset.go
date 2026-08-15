package redis

import (
	"context"
	"errors"
	"time"

	redis "github.com/redis/go-redis/v9"
)

func (c *Client) Zincrby(ctx context.Context, key string, increment float64, member string) error {
	if c == nil || c.rdb == nil {
		return errors.New("redis client is not initialized")
	}
	return c.rdb.ZIncrBy(ctx, key, increment, member).Err()
}

func (c *Client) ZAdd(ctx context.Context, key string, members ...redis.Z) error {

	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.ZAdd(ctx, key, members...).Err()
}

func (c *Client) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error {

	if c == nil || c.rdb == nil {
		return errors.New("redis client is not initialized")
	}
	return c.rdb.ZRemRangeByRank(ctx, key, start, stop).Err()
}

func (c *Client) ZRemRangeByScore(ctx context.Context, key string, min, max string) error {
	if c == nil || c.rdb == nil {
		return errors.New("redis client is not initialized")
	}
	return c.rdb.ZRemRangeByScore(ctx, key, min, max).Err()
}

// 过期时间
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) error {
	if c == nil || c.rdb == nil {
		return errors.New("redis client is not initialized")
	}
	return c.rdb.Expire(ctx, key, expiration).Err()
}

func (c *Client) ZUnionStore(ctx context.Context, dst string, keys []string, aggregate string) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.ZUnionStore(ctx, dst, &redis.ZStore{
		Keys:      keys,
		Aggregate: aggregate,
	}).Err()
}

func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, nil
	}
	n, err := c.rdb.Exists(ctx, key).Result()
	return n > 0, err
}

func (c *Client) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if c == nil || c.rdb == nil {
		return nil, nil
	}
	return c.rdb.ZRevRange(ctx, key, start, stop).Result()
}

func (c *Client) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	if c == nil || c.rdb == nil {
		return nil, errors.New("redis client is not initialized")
	}
	return c.rdb.ZRangeWithScores(ctx, key, start, stop).Result()
}

func (c *Client) ZRevRangeByScore(ctx context.Context, key string, max, min string, offset, count int64) ([]string, error) {
	if c == nil || c.rdb == nil {
		return nil, nil
	}
	return c.rdb.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
		Max:    max,
		Min:    min,
		Offset: offset,
		Count:  count,
	}).Result()
}
