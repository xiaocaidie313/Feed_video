package redis

import (
	"context"
	"time"
)

func (c *Client) SetBatch(ctx context.Context, values map[string][]byte, ttl time.Duration) error {
	if c == nil || c.rdb == nil || len(values) == 0 {
		return nil
	}
	pipe := c.rdb.Pipeline()
	for key, value := range values {
		pipe.Set(ctx, key, value, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}
