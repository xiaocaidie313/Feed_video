package feed

import (
	"context"
	"encoding/json"
	rediscache "feedvid/internal/midware/redis"
	"feedvid/internal/video"

	"feedvid/internal/apierror"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"

	"golang.org/x/sync/singleflight"
)

type FeedService struct {
	repo       *FeedRepository
	likeRepo   *video.LikeRepo
	rediscache *rediscache.Client
	localcache *cache.Cache

	requestGroup singleflight.Group
}

func NewFeedService(repo *FeedRepository, likeRepo *video.LikeRepo, rediscache *rediscache.Client) *FeedService {
	return &FeedService{repo: repo, likeRepo: likeRepo, rediscache: rediscache, localcache: cache.New(3*time.Second, 5*time.Second)}
}

func (f *FeedService) GetVideoByIDs(ctx context.Context, videoIDs []uint) ([]*video.Video, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(videoIDs) == 0 {
		return []*video.Video{}, nil
	}
	byID := map[uint]*video.Video{}
	missing := make([]uint, 0, len(videoIDs))
	seen := map[uint]bool{}
	for _, id := range videoIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		key := f.rediscache.Key("video:entity:%d", id)
		if f.localcache != nil {
			if cached, ok := f.localcache.Get(key); ok {
				if value, ok := cached.(video.Video); ok {
					copy := value
					byID[id] = &copy
					continue
				}
			}
		}
		missing = append(missing, id)
	}
	if f.rediscache != nil && len(missing) > 0 {
		keys := make([]string, len(missing))
		for i, id := range missing {
			keys[i] = f.rediscache.Key("video:entity:%d", id)
		}
		cacheCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		values, err := f.rediscache.MGet(cacheCtx, keys...)
		cancel()
		if err == nil {
			remaining := make([]uint, 0, len(missing))
			for i, id := range missing {
				if encoded, ok := values[i].(string); ok {
					var v video.Video
					if json.Unmarshal([]byte(encoded), &v) == nil && v.ID == id {
						copy := v
						byID[id] = &copy
						if f.localcache != nil {
							f.localcache.Set(keys[i], v, 5*time.Second)
						}
						continue
					}
				}
				remaining = append(remaining, id)
			}
			missing = remaining
		}
	}
	if len(missing) > 0 {
		sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
		parts := make([]string, len(missing))
		for i, id := range missing {
			parts[i] = strconv.FormatUint(uint64(id), 10)
		}
		result := f.requestGroup.DoChan("entities:"+strings.Join(parts, ","), func() (interface{}, error) {
			sharedCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			rows, err := f.repo.GetByIDs(sharedCtx, missing)
			if err != nil {
				return nil, err
			}
			writes := map[string][]byte{}
			for _, v := range rows {
				key := f.rediscache.Key("video:entity:%d", v.ID)
				if f.localcache != nil {
					f.localcache.Set(key, *v, 5*time.Second)
				}
				if data, err := json.Marshal(v); err == nil {
					writes[key] = data
				}
			}
			if f.rediscache != nil {
				cacheCtx, cacheCancel := context.WithTimeout(sharedCtx, 50*time.Millisecond)
				_ = f.rediscache.SetBatch(cacheCtx, writes, time.Hour)
				cacheCancel()
			}
			return rows, nil
		})
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case fetched := <-result:
			if fetched.Err != nil {
				return nil, fetched.Err
			}
			for _, v := range fetched.Val.([]*video.Video) {
				copy := *v
				byID[v.ID] = &copy
			}
		}
	}
	return buildOrderedResult(videoIDs, byID), nil
}

func (f *FeedService) page(ctx context.Context, limit int, before time.Time, viewer uint, id uint, following bool) ([]FeedVideoItem, int64, *uint, bool, error) {
	if limit < 1 || limit > 50 {
		return nil, 0, nil, false, apierror.ErrValidation
	}
	var rows []*video.Video
	var err error
	if following {
		rows, err = f.repo.ListByFollowing(ctx, limit+1, viewer, before, id)
	} else {
		rows, err = f.repo.ListLatest(ctx, limit+1, before, id)
	}
	if err != nil {
		return nil, 0, nil, false, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	ids := make([]uint, len(rows))
	for i, v := range rows {
		ids[i] = v.ID
	}
	videos, err := f.GetVideoByIDs(ctx, ids)
	if err != nil {
		return nil, 0, nil, false, err
	}
	items, err := f.buildFeedVideos(ctx, videos, viewer)
	if err != nil {
		return nil, 0, nil, false, err
	}
	var nextTime int64
	var nextID *uint
	if len(rows) > 0 {
		last := rows[len(rows)-1]
		nextTime = last.CreateTime.UnixMilli()
		id := last.ID
		nextID = &id
	}
	return items, nextTime, nextID, more, nil
}
func (f *FeedService) ListLatest(ctx context.Context, limit int, before time.Time, viewer uint, idBefore ...uint) (ListLatestResponse, error) {
	var id uint
	if len(idBefore) > 0 {
		id = idBefore[0]
	}
	items, next, nextID, more, err := f.page(ctx, limit, before, viewer, id, false)
	return ListLatestResponse{VideoList: items, NextTime: next, NextIDBefore: nextID, HasMore: more}, err
}

// 按照点赞数查询视频
func (f *FeedService) ListLikesCount(ctx context.Context, limit int, cursor *LikesCountCursor, viewerAccountID uint) (ListLikesCountResponse, error) {
	videos, err := f.repo.ListLikesCountWithCursor(ctx, limit, cursor)
	if err != nil {
		return ListLikesCountResponse{}, err
	}
	hasMore := len(videos) == limit
	feedVideos, err := f.buildFeedVideos(ctx, videos, viewerAccountID)
	if err != nil {
		return ListLikesCountResponse{}, err
	}
	resp := ListLikesCountResponse{
		VideoList: feedVideos,
		HasMore:   hasMore,
	}
	if len(videos) > 0 {
		last := videos[len(videos)-1]
		nextLikesCountBefore := last.LikesCount
		nextIDBefore := last.ID
		resp.NextLikesCountBefore = &nextLikesCountBefore
		resp.NextIDBefore = &nextIDBefore
	}
	return resp, nil
}

func (f *FeedService) ListByFollowing(ctx context.Context, limit int, before time.Time, viewer uint, idBefore ...uint) (ListByFollowingResponse, error) {
	var id uint
	if len(idBefore) > 0 {
		id = idBefore[0]
	}
	items, next, nextID, more, err := f.page(ctx, limit, before, viewer, id, true)
	return ListByFollowingResponse{VideoList: items, NextTime: next, NextIDBefore: nextID, HasMore: more}, err
}

func (f *FeedService) ListByPopularity(ctx context.Context, limit int, reqAsOf int64, offset int, viewerAccountID uint, latestPopularity int64, latestBefore time.Time, latestIDBefore uint) (ListByPopularityResponse, error) {
	// Redis 热榜（稳定分页：as_of + offset）
	if f.rediscache != nil {
		// 截断到分钟
		asOf := time.Now().UTC().Truncate(time.Minute)
		if reqAsOf > 0 {
			asOf = time.Unix(reqAsOf, 0).UTC().Truncate(time.Minute)
		}

		const win = 60
		keys := make([]string, 0, win)
		for i := 0; i < win; i++ {
			keys = append(keys, f.rediscache.Key("hot:video:1m:%s", asOf.Add(-time.Duration(i)*time.Minute).Format("200601021504")))
		}

		dest := f.rediscache.Key("hot:video:merge:1m:%s", asOf.Format("200601021504")) // 快照key：同一个as_of页内复用
		opCtx, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
		defer cancel()

		exists, _ := f.rediscache.Exists(opCtx, dest)
		if !exists {
			_ = f.rediscache.ZUnionStore(opCtx, dest, keys, "SUM")
			_ = f.rediscache.Expire(opCtx, dest, 2*time.Minute) // 给翻页留时间
		}

		start := int64(offset)
		stop := start + int64(limit) - 1
		members, err := f.rediscache.ZRevRange(opCtx, dest, start, stop)
		if err == nil && len(members) == 0 {
			if offset > 0 {
				return ListByPopularityResponse{
					VideoList:  []FeedVideoItem{},
					AsOf:       asOf.Unix(),
					NextOffset: offset,
					HasMore:    false,
				}, nil
			}
		}
		if err == nil && len(members) > 0 {
			ids := make([]uint, 0, len(members))
			for _, m := range members {
				u, err := strconv.ParseUint(m, 10, 64)
				if err == nil && u > 0 {
					ids = append(ids, uint(u))
				}
			}

			videos, err := f.repo.GetByIDs(ctx, ids)
			if err == nil {
				byID := make(map[uint]*video.Video, len(videos))
				for _, v := range videos {
					byID[v.ID] = v
				}
				ordered := make([]*video.Video, 0, len(ids))
				for _, id := range ids {
					if v := byID[id]; v != nil {
						ordered = append(ordered, v)
					}
				}
				items, err := f.buildFeedVideos(ctx, ordered, viewerAccountID)
				if err != nil {
					return ListByPopularityResponse{}, err
				}
				resp := ListByPopularityResponse{
					VideoList:  items,
					AsOf:       asOf.Unix(),
					NextOffset: offset + len(items),
					HasMore:    len(items) == limit,
				}
				if len(ordered) > 0 {
					last := ordered[len(ordered)-1]
					nextPopularity := last.Popularity
					nextBefore := last.CreateTime
					nextID := last.ID
					resp.NextLatestPopularity = &nextPopularity
					resp.NextLatestBefore = &nextBefore
					resp.NextLatestIDBefore = &nextID
				}
				return resp, nil
			}
		}
	}

	videos, err := f.repo.ListByPopularity(ctx, limit, latestPopularity, latestBefore, latestIDBefore)
	if err != nil {
		return ListByPopularityResponse{}, err
	}
	items, err := f.buildFeedVideos(ctx, videos, viewerAccountID)
	if err != nil {
		return ListByPopularityResponse{}, err
	}
	resp := ListByPopularityResponse{
		VideoList:  items,
		AsOf:       0,
		NextOffset: 0,
		HasMore:    len(items) == limit,
	}
	if len(videos) > 0 {
		last := videos[len(videos)-1]
		nextPopularity := last.Popularity
		nextBefore := last.CreateTime
		nextID := last.ID
		resp.NextLatestPopularity = &nextPopularity
		resp.NextLatestBefore = &nextBefore
		resp.NextLatestIDBefore = &nextID
	}
	return resp, nil
}

func (f *FeedService) buildFeedVideos(ctx context.Context, videos []*video.Video, viewerAccountID uint) ([]FeedVideoItem, error) {
	feedVideos := make([]FeedVideoItem, 0, len(videos))
	videoIDs := make([]uint, len(videos))
	for i, v := range videos {
		videoIDs[i] = v.ID
	}
	counts, err := f.repo.GetLikeCounts(ctx, videoIDs)
	if err != nil {
		return nil, err
	}
	likedMap, err := f.likeRepo.BatchGetLiked(ctx, videoIDs, viewerAccountID)
	if err != nil {
		return nil, err
	}
	for _, video := range videos {
		feedVideos = append(feedVideos, FeedVideoItem{
			ID:          video.ID,
			Author:      FeedAuthor{ID: video.AuthorID, Username: video.Username},
			Title:       video.Title,
			Description: video.Description,
			PlayURL:     video.PlayURL,
			CoverURL:    video.CoverURL,
			CreateTime:  video.CreateTime.Unix(),
			LikesCount:  counts[video.ID],
			IsLiked:     likedMap[video.ID],
		})
	}
	return feedVideos, nil
}

// 去除不存在id的video
func buildOrderedResult(orderedIDs []uint, dataMap map[uint]*video.Video) []*video.Video {
	res := make([]*video.Video, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if v, exits := dataMap[id]; exits && v != nil {
			res = append(res, v)
		}
	}
	return res
}

func (f *FeedService) ListByTag(ctx context.Context, tagName string, limit int, viewerAccountID uint) ([]FeedVideoItem, error) {
	videos, err := f.repo.ListByTag(ctx, tagName, limit)
	if err != nil {
		return nil, err
	}
	return f.buildFeedVideos(ctx, videos, viewerAccountID)
}
