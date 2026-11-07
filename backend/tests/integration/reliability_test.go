//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"feedvid/internal/config"
	"feedvid/internal/eventstore"
	"feedvid/internal/feed"
	"feedvid/internal/midware/rabbitmq"
	rediscache "feedvid/internal/midware/redis"
	"feedvid/internal/social"
	"feedvid/internal/video"
	"feedvid/internal/worker"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/gorm"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureVideo(t *testing.T, db *gorm.DB) *video.Video {
	t.Helper()
	v := &video.Video{AuthorID: 1, Username: "author", Title: "test", PlayURL: "video.mp4", CoverURL: "cover.jpg", CreateTime: time.Now().UTC().Truncate(time.Millisecond)}
	if err := db.Create(v).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(16)
	return v
}
func assertLikes(t *testing.T, db *gorm.DB, id uint, want int64) {
	t.Helper()
	var count int64
	if err := db.Model(&video.Like{}).Where("video_id = ?", id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	var row video.Video
	if err := db.First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	if count != want || row.LikesCount != want {
		t.Fatalf("like rows=%d, counter=%d, want %d", count, row.LikesCount, want)
	}
}
func TestConcurrentLikeChangesAreAtomicAndIdempotent(t *testing.T) {
	db := database(t)
	v := fixtureVideo(t, db)
	service := video.NewLikeService(video.NewLikeRepo(db), video.NewVideoRepo(db), nil)
	run := func(wanted bool) {
		t.Helper()
		var wg sync.WaitGroup
		failures := make(chan error, 100)
		for range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				like := &video.Like{VideoID: v.ID, AccountID: 2}
				if wanted {
					failures <- service.Like(context.Background(), like)
				} else {
					failures <- service.Unlike(context.Background(), like)
				}
			}()
		}
		wg.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	run(true)
	assertLikes(t, db, v.ID, 1)
	run(false)
	assertLikes(t, db, v.ID, 0)
	var count int64
	if err := db.Model(&eventstore.Message{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("want exactly two transition events, got %d (%v)", count, err)
	}
}
func TestLikeFailureRollsBackRecordCounterAndEvent(t *testing.T) {
	db := database(t)
	v := fixtureVideo(t, db)
	injected := errors.New("outbox insert unavailable")
	name := "test:fail-outbox"
	if err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "outbox_msgs" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Create().Remove(name)
	_, err := video.NewLikeRepo(db).ChangeLike(context.Background(), v.ID, 2, true, "")
	if !errors.Is(err, injected) {
		t.Fatalf("expected outbox failure, got %v", err)
	}
	assertLikes(t, db, v.ID, 0)
	var count int64
	if err := db.Model(&eventstore.Message{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("failed transaction left an outbox row")
	}
}

type recordingPublisher struct {
	messages []eventstore.Message
	fail     bool
}

func (p *recordingPublisher) Publish(_ context.Context, m eventstore.Message) error {
	p.messages = append(p.messages, m)
	if p.fail {
		return errors.New("connection lost after write")
	}
	return nil
}
func TestOutboxRetryPreservesIdentityAndPayload(t *testing.T) {
	db := database(t)
	v := fixtureVideo(t, db)
	if _, err := video.NewLikeRepo(db).ChangeLike(context.Background(), v.ID, 2, true, ""); err != nil {
		t.Fatal(err)
	}
	publisher := &recordingPublisher{fail: true}
	dispatcher := eventstore.NewDispatcher(db, publisher)
	if found, err := dispatcher.DispatchOne(context.Background()); !found || err == nil {
		t.Fatal("failed publish must be reported")
	}
	var pending eventstore.Message
	if err := db.First(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Attempts != 1 || pending.EventID == "" {
		t.Fatal("retry metadata was not retained")
	}
	if err := db.Model(&pending).Update("next_attempt_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	publisher.fail = false
	if found, err := dispatcher.DispatchOne(context.Background()); !found || err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if len(publisher.messages) != 2 || publisher.messages[0].EventID != publisher.messages[1].EventID || !bytes.Equal(publisher.messages[0].Payload, publisher.messages[1].Payload) {
		t.Fatal("retry changed event identity or content")
	}
	var count int64
	db.Model(&eventstore.Message{}).Count(&count)
	if count != 0 {
		t.Fatal("confirmed event was not removed")
	}
}
func TestLegacyOutboxRowsKeepStableIdentity(t *testing.T) {
	db := database(t)
	row := eventstore.Message{VideoID: 123, EventType: "video_published", CreateTime: time.Now().UTC(), Status: "pending"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	publisher := &recordingPublisher{fail: true}
	dispatcher := eventstore.NewDispatcher(db, publisher)
	_, _ = dispatcher.DispatchOne(context.Background())
	if err := db.Model(&row).Update("next_attempt_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	publisher.fail = false
	if _, err := dispatcher.DispatchOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(publisher.messages) != 2 || publisher.messages[0].EventID != publisher.messages[1].EventID || !bytes.Equal(publisher.messages[0].Payload, publisher.messages[1].Payload) {
		t.Fatal("legacy retry is unstable")
	}
}
func TestRedeliveredLikeEventDoesNotDuplicateEffects(t *testing.T) {
	db := database(t)
	v := fixtureVideo(t, db)
	cache := redisClient(t)
	repo := video.NewLikeRepo(db)
	if _, err := repo.ChangeLike(context.Background(), v.ID, 2, true, ""); err != nil {
		t.Fatal(err)
	}
	var row eventstore.Message
	if err := db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	likeWorker := worker.NewLikeWorker(nil, repo, video.NewVideoRepo(db), "test", cache)
	notifications := worker.NewNotificationWorker(nil, db, "test", nil)
	for range 2 {
		if err := likeWorker.Process(context.Background(), row.Payload); err != nil {
			t.Fatal(err)
		}
		if err := notifications.Process(context.Background(), amqp.Delivery{RoutingKey: row.RoutingKey, Body: row.Payload}); err != nil {
			t.Fatal(err)
		}
	}
	assertLikes(t, db, v.ID, 1)
	var count int64
	if err := db.Model(&worker.Notification{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("duplicate notifications: %d (%v)", count, err)
	}
	// The canonical event is already committed; replay cannot restore a cancelled like.
	if _, err := repo.ChangeLike(context.Background(), v.ID, 2, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := likeWorker.Process(context.Background(), row.Payload); err != nil {
		t.Fatal(err)
	}
	assertLikes(t, db, v.ID, 0)
}
func TestLegacyLikeDeliveryCannotReplayAfterUnlike(t *testing.T) {
	db := database(t)
	v := fixtureVideo(t, db)
	repo := video.NewLikeRepo(db)
	handler := worker.NewLikeWorker(nil, repo, video.NewVideoRepo(db), "test")
	event := rabbitmq.LikeEvent{EventID: "legacy-one", UserID: 2, VideoID: v.ID, Action: "like"}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Process(context.Background(), body); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ChangeLike(context.Background(), v.ID, 2, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := handler.Process(context.Background(), body); err != nil {
		t.Fatal(err)
	}
	assertLikes(t, db, v.ID, 0)
}
func TestDetailCacheLockIsReleased(t *testing.T) {
	db := database(t)
	v := fixtureVideo(t, db)
	cache := redisClient(t)
	service := video.NewVideoService(video.NewVideoRepo(db), cache)
	if _, err := service.GetDetail(context.Background(), v.ID); err != nil {
		t.Fatal(err)
	}
	key := "lock:" + cache.Key("video:detail:id=%d", v.ID)
	token, locked, err := cache.Lock(context.Background(), key, time.Second)
	if err != nil || !locked {
		t.Fatalf("detail left its lock behind: %v", err)
	}
	if err := cache.Unlock(context.Background(), key, token); err != nil {
		t.Fatal(err)
	}
}
func TestFeedBatchLoadsAndReportsDatabaseErrors(t *testing.T) {
	db := database(t)
	a := fixtureVideo(t, db)
	b := fixtureVideo(t, db)
	service := feed.NewFeedService(feed.NewFeedRepository(db), video.NewLikeRepo(db), nil)
	var queries atomic.Int64
	name := "test:count-video-queries"
	if err := db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "videos" {
			queries.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(name)
	rows, err := service.GetVideoByIDs(context.Background(), []uint{b.ID, a.ID, b.ID})
	if err != nil || len(rows) != 3 || rows[0].ID != b.ID || rows[1].ID != a.ID {
		t.Fatalf("batch result lost order: %v", err)
	}
	if queries.Load() != 1 {
		t.Fatalf("want one query, got %d", queries.Load())
	}
	if _, err := service.GetVideoByIDs(context.Background(), []uint{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	if queries.Load() != 1 {
		t.Fatal("warm entity cache was ignored")
	}
	fail := "test:fail-query"
	if err := db.Callback().Query().Before("gorm:query").Register(fail, func(tx *gorm.DB) { tx.AddError(errors.New("database unavailable")) }); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(fail)
	fresh := feed.NewFeedService(feed.NewFeedRepository(db), video.NewLikeRepo(db), nil)
	if _, err := fresh.GetVideoByIDs(context.Background(), []uint{a.ID}); err == nil {
		t.Fatal("database failure was returned as an empty success")
	}
}
func TestFeedCompositeCursorWithEqualTimestamps(t *testing.T) {
	for _, useRedis := range []bool{false, true} {
		t.Run(fmt.Sprint(useRedis), func(t *testing.T) {
			db := database(t)
			same := time.Now().UTC().Truncate(time.Millisecond)
			for range 12 {
				v := fixtureVideo(t, db)
				if err := db.Model(v).Update("create_time", same).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(&social.Social{FollowerID: 2, VloggerID: 1}).Error; err != nil {
				t.Fatal(err)
			}
			var cache *rediscache.Client
			if useRedis {
				cache = redisClient(t)
			}
			service := feed.NewFeedService(feed.NewFeedRepository(db), video.NewLikeRepo(db), cache)
			var before time.Time
			var id uint
			seen := map[uint]bool{}
			for page := 0; page < 3; page++ {
				response, err := service.ListLatest(context.Background(), 5, before, 0, id)
				if err != nil {
					t.Fatal(err)
				}
				for _, item := range response.VideoList {
					if seen[item.ID] {
						t.Fatal("duplicate item across pages")
					}
					seen[item.ID] = true
					want := uint(13 - len(seen))
					if item.ID != want {
						t.Fatalf("order %d, want %d", item.ID, want)
					}
				}
				if response.NextIDBefore == nil {
					t.Fatal("missing tie-break cursor")
				}
				before = time.UnixMilli(response.NextTime)
				id = *response.NextIDBefore
				if response.HasMore != (page < 2) {
					t.Fatal("inaccurate has_more")
				}
			}
			if len(seen) != 12 {
				t.Fatalf("cursor skipped %d videos", 12-len(seen))
			}
			first, err := service.ListByFollowing(context.Background(), 5, time.Time{}, 2)
			if err != nil {
				t.Fatal(err)
			}
			second, err := service.ListByFollowing(context.Background(), 5, time.UnixMilli(first.NextTime), 2, *first.NextIDBefore)
			if err != nil {
				t.Fatal(err)
			}
			if first.VideoList[4].ID != 8 || second.VideoList[0].ID != 7 {
				t.Fatal("following cursor skipped equal-time videos")
			}
			// A warmed entity cache must not freeze the mutable like counter.
			if _, err := video.NewLikeRepo(db).ChangeLike(context.Background(), 12, 2, true, ""); err != nil {
				t.Fatal(err)
			}
			updated, err := service.ListLatest(context.Background(), 5, time.Time{}, 0)
			if err != nil {
				t.Fatal(err)
			}
			if updated.VideoList[0].LikesCount != 1 {
				t.Fatal("feed served a stale like count")
			}
		})
	}
}
func TestPublisherDetectsUnroutableMessagesAndReconnects(t *testing.T) {
	raw := amqpURL()
	uri, err := amqp.ParseURI(raw)
	if err != nil {
		t.Fatal(err)
	}
	base, err := rabbitmq.NewRabbitMQ(&config.RabbitMQConfig{Host: uri.Host, Port: uri.Port, Username: uri.Username, Password: uri.Password})
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	publisher, err := rabbitmq.NewOutboxPublisher(base)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	m := eventstore.Message{EventID: "unroutable", Exchange: "like.events", RoutingKey: "missing.route.extra", Payload: []byte("{}")}
	if err := publisher.Publish(ctx, m); err == nil {
		t.Fatal("unroutable event was reported as delivered")
	}
	if err := base.Conn.Close(); err != nil {
		t.Fatal(err)
	}
	m.RoutingKey = "like.like"
	if err := publisher.Publish(ctx, m); err != nil {
		t.Fatalf("publisher failed to recreate connection: %v", err)
	}
}
func amqpURL() string {
	value := os.Getenv("INTEGRATION_AMQP_URL")
	if value == "" {
		value = "amqp://feedvid:feedvid_mq_dev_password@127.0.0.1:15672/"
	}
	return value
}

func TestHistoricalLikeCounterReconciliation(t *testing.T) {
	db := database(t)
	v := fixtureVideo(t, db)
	repo := video.NewLikeRepo(db)
	if err := repo.Like(context.Background(), &video.Like{VideoID: v.ID, AccountID: 2}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(v).UpdateColumn("likes_count", 99).Error; err != nil {
		t.Fatal(err)
	}
	count, err := repo.ReconcileCounts(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("reconciliation failed: count=%d, error=%v", count, err)
	}
	assertLikes(t, db, v.ID, 1)
	if count, err := repo.ReconcileCounts(context.Background()); err != nil || count != 0 {
		t.Fatal("reconciliation is not idempotent")
	}
	if _, err := repo.ChangeLike(context.Background(), v.ID, 3, true, ""); err != nil {
		t.Fatal(err)
	}
	assertLikes(t, db, v.ID, 2)
}
