package video

import (
	"context"
	"strconv"
	"time"

	rediscache "feedvid/internal/midware/redis"
)

// 更新视频流行度缓存
func UpdatePopularityCache(ctx context.Context, cache *rediscache.Client, id uint, change int64) {
	if cache == nil || id == 0 || change == 0 {
		return
	}

	// 1) 详情缓存：直接失效（最简单靠谱）
	_ = cache.Del(context.Background(), cache.Key("video:detail:id=%d", id))
	_ = cache.Del(context.Background(), cache.Key("video:entity:%d", id))

	// 2) 热榜：写到“时间窗ZSET”，不要用 detail key
	// 这个 now 同时也设定了每次访问请求时候的 key
	now := time.Now().UTC().Truncate(time.Minute)
	// 时间窗 key：每分钟一个 key
	windowKey := cache.Key("hot:video:1m:%s", now.Format("200601021504"))
	member := strconv.FormatUint(uint64(id), 10)

	opCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	_ = cache.Zincrby(opCtx, windowKey, float64(change), member)
	_ = cache.Expire(opCtx, windowKey, 2*time.Hour)
}
