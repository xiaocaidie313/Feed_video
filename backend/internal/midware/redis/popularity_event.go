package redis

import (
	"context"
	"errors"
	goredis "github.com/redis/go-redis/v9"
	"strconv"
	"time"
)

var popularityEventScript = goredis.NewScript(`
if redis.call('EXISTS',KEYS[1]) == 1 then return 0 end
redis.call('ZINCRBY',KEYS[2],ARGV[1],ARGV[2])
redis.call('EXPIRE',KEYS[2],7200)
redis.call('DEL',KEYS[3],KEYS[4])
redis.call('SET',KEYS[1],'1','EX',604800)
return 1
`)

func (c *Client) ApplyPopularityEvent(ctx context.Context, eventID string, videoID uint, delta int64, occurredAt time.Time) error {
	if c == nil || c.rdb == nil {
		return errors.New("redis is unavailable")
	}
	if eventID == "" || videoID == 0 {
		return errors.New("invalid popularity event")
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	detail, entity := c.Key("video:detail:id=%d", videoID), c.Key("video:entity:%d", videoID)
	if time.Since(occurredAt) > 2*time.Hour {
		return c.rdb.Del(ctx, detail, entity).Err()
	}
	bucket := c.Key("hot:video:1m:%s", occurredAt.UTC().Format("200601021504"))
	return popularityEventScript.Run(ctx, c.rdb, []string{c.Key("event:popularity:%s", eventID), bucket, detail, entity}, delta, strconv.FormatUint(uint64(videoID), 10)).Err()
}
