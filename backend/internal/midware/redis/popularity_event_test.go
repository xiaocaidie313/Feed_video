package redis

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"testing"
	"time"
)

func TestPopularityEventIsAtomicAndDeduplicated(t *testing.T) {
	server := miniredis.RunT(t)
	raw := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	defer raw.Close()
	client := NewClient(raw, "test:")
	now := time.Now().UTC()
	for range 2 {
		if err := client.ApplyPopularityEvent(context.Background(), "event-one", 42, 1, now); err != nil {
			t.Fatal(err)
		}
	}
	key := client.Key("hot:video:1m:%s", now.Format("200601021504"))
	score, err := raw.ZScore(context.Background(), key, "42").Result()
	if err != nil || score != 1 {
		t.Fatalf("duplicate event changed score to %v (%v)", score, err)
	}
	if err := client.ApplyPopularityEvent(context.Background(), "event-two", 42, -1, now); err != nil {
		t.Fatal(err)
	}
	score, err = raw.ZScore(context.Background(), key, "42").Result()
	if err != nil || score != 0 {
		t.Fatalf("unlike score %v (%v)", score, err)
	}
}
