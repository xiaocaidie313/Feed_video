//go:build integration

package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"feedvid/internal/account"
	appdb "feedvid/internal/db"
	apphttp "feedvid/internal/http"
	"feedvid/internal/midware/rabbitmq"
	rediscache "feedvid/internal/midware/redis"
	"feedvid/internal/video"
	"github.com/gin-gonic/gin"
	mysqldriver "github.com/go-sql-driver/mysql"
	amqp "github.com/rabbitmq/amqp091-go"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func database(t *testing.T) *gorm.DB {
	t.Helper()
	raw := os.Getenv("INTEGRATION_MYSQL_DSN")
	if raw == "" {
		raw = "root:feedvid_root_dev_password@tcp(127.0.0.1:13306)/?parseTime=true"
	}
	cfg, err := mysqldriver.ParseDSN(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Timeout = 3 * time.Second
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "feedvid_test_" + hex.EncodeToString(suffix[:])
	// The identifier contains only the fixed prefix and generated hex, never input.
	if _, err := admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4"); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := admin.Exec("DROP DATABASE " + name)
		if err != nil {
			t.Errorf("test database cleanup failed: %v", err)
		}
		admin.Close()
	})
	cfg.DBName = name
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := appdb.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}
func redisClient(t *testing.T) *rediscache.Client {
	t.Helper()
	addr := os.Getenv("INTEGRATION_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:16379"
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	prefix := "test:" + hex.EncodeToString(suffix[:]) + ":"
	client := rediscache.NewClient(goredis.NewClient(&goredis.Options{Addr: addr, MaxRetries: 0}), prefix)
	if err := client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup := rediscache.NewClient(goredis.NewClient(&goredis.Options{Addr: addr}), prefix)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := cleanup.DelByPattern(ctx, cleanup.Key("*")); err != nil {
			t.Errorf("Redis test cleanup: %v", err)
		}
		cleanup.Close()
		client.Close()
	})
	return client
}
func request(t *testing.T, server *httptest.Server, path, token string, body interface{}, want int) map[string]interface{} {
	t.Helper()
	payload, err := json.Marshal(body)
	if raw, ok := body.([]byte); ok {
		payload = raw
		err = nil
	}
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("POST %s: status %d, want %d", strings.SplitN(path, "?", 2)[0], response.StatusCode, want)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
func upload(t *testing.T, server *httptest.Server, path, token, name string) string {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("integration upload fixture")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+path, &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("upload status %d", response.StatusCode)
	}
	var result map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result["url"].(string)
}

func TestBusinessFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, useRedis := range []bool{false, true} {
		name := "without_redis"
		if useRedis {
			name = "with_redis"
		}
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			db := database(t)
			var cache *rediscache.Client
			if useRedis {
				cache = redisClient(t)
			}
			// The HTTP flow explicitly exercises the supported MQ-unavailable fallback.
			// RabbitMQ's wire contract is checked separately below.
			server := httptest.NewServer(apphttp.SetRouter(db, cache, nil))
			defer server.Close()
			server.Client().Timeout = 5 * time.Second
			credentials := map[string]string{"username": "alice", "password": "correct-password"}
			request(t, server, "/account/register", "", credentials, 200)
			request(t, server, "/account/register", "", credentials, 409)
			request(t, server, "/account/register", "", map[string]string{"username": "bob", "password": "short"}, 400)
			request(t, server, "/account/login", "", []byte("{"), 400)
			request(t, server, "/account/login", "", map[string]string{"username": "alice", "password": "wrong-password"}, 401)
			request(t, server, "/account/login", "", map[string]string{"username": "unknown", "password": "wrong-password"}, 401)
			session := request(t, server, "/account/login", "", credentials, 200)
			token, refresh := session["token"].(string), session["refresh_token"].(string)
			id := uint(session["account_id"].(float64))
			request(t, server, "/notification/unreadCount", "", nil, 401)
			request(t, server, "/notification/unreadCount", token, nil, 200)
			request(t, server, "/notification/unreadCount?token="+token, "", nil, 200)
			play := upload(t, server, "/video/uploadVideo", token, "clip.mp4")
			cover := upload(t, server, "/video/uploadCover", token, "cover.jpg")
			response, err := server.Client().Get(play)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatal("uploaded file is not accessible")
			}
			published := request(t, server, "/video/publish", token, map[string]string{"title": "hello #backend", "play_url": play, "cover_url": cover}, 200)
			videoID := uint(published["id"].(float64))
			feed := request(t, server, "/feed/listLatest", "", map[string]int{"limit": 10}, 200)
			videos, ok := feed["video_list"].([]interface{})
			if !ok || len(videos) != 1 {
				t.Fatal("published video missing from latest feed")
			}
			request(t, server, "/like/like", token, map[string]uint{"video_id": videoID}, 200)
			liked := request(t, server, "/like/isLiked", token, map[string]uint{"video_id": videoID}, 200)
			if liked["is_liked"] != true {
				t.Fatal("like was not persisted")
			}
			request(t, server, "/like/unlike", token, map[string]uint{"video_id": videoID}, 200)
			liked = request(t, server, "/like/isLiked", token, map[string]uint{"video_id": videoID}, 200)
			if liked["is_liked"] != false {
				t.Fatal("unlike was not persisted")
			}
			rotated := request(t, server, "/account/refresh", "", map[string]string{"refresh_token": refresh}, 200)
			access := rotated["token"].(string)
			if access == token {
				t.Fatal("refresh reused access token")
			}
			request(t, server, "/notification/unreadCount", token, nil, 401)
			request(t, server, "/account/logout", access, nil, 200)
			// A stale authorization entry must never override a database revocation.
			if cache != nil {
				if err := cache.SetBytes(context.Background(), cache.Key("accountID:%d", id), []byte(access), time.Hour); err != nil {
					t.Fatal(err)
				}
			}
			request(t, server, "/notification/unreadCount", access, nil, 401)
			request(t, server, "/notification/unreadCount?token="+access, "", nil, 401)
			request(t, server, "/account/refresh", "", map[string]string{"refresh_token": refresh}, 401)
			session = request(t, server, "/account/login", "", credentials, 200)
			access, refresh = session["token"].(string), session["refresh_token"].(string)
			if cache != nil {
				if err := cache.Close(); err != nil {
					t.Fatal(err)
				}
			} // Redis failure cannot block correct authentication or revocation.
			request(t, server, "/notification/unreadCount", access, nil, 200)
			request(t, server, "/account/changePassword", "", map[string]string{"old_password": "correct-password", "new_password": "new-password"}, 401)
			request(t, server, "/account/changePassword", access, map[string]string{"old_password": "wrong-password", "new_password": "new-password"}, 401)
			request(t, server, "/account/changePassword", access, map[string]string{"old_password": "correct-password", "new_password": "correct-password"}, 400)
			request(t, server, "/account/changePassword", access, map[string]string{"old_password": "correct-password", "new_password": "new-password"}, 200)
			request(t, server, "/notification/unreadCount", access, nil, 401)
			request(t, server, "/account/refresh", "", map[string]string{"refresh_token": refresh}, 401)
			request(t, server, "/account/login", "", credentials, 401)
			session = request(t, server, "/account/login", "", map[string]string{"username": "alice", "password": "new-password"}, 200)
			access, refresh = session["token"].(string), session["refresh_token"].(string)
			renamed := request(t, server, "/account/rename", access, map[string]string{"new_username": "alice_new"}, 200)
			request(t, server, "/notification/unreadCount", access, nil, 401)
			request(t, server, "/notification/unreadCount", renamed["token"].(string), nil, 200)
			if err := db.Model(&account.Account{}).Where("id = ?", id).Update("refresh_expires_at", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
				t.Fatal(err)
			}
			request(t, server, "/account/refresh", "", map[string]string{"refresh_token": refresh}, 401)
			var outboxCount int64
			if err := db.Model(&video.OutboxMsg{}).Count(&outboxCount).Error; err != nil || outboxCount != 1 {
				t.Fatal("video and outbox were not persisted together")
			}
		})
	}
}

func TestRevokedSessionCannotBeRestoredByInFlightWrites(t *testing.T) {
	db := database(t)
	service := account.NewAccountService(account.NewAccountRepo(db), nil)
	a := &account.Account{Username: "alice", Password: "correct-password"}
	if err := service.CreateAccount(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	_, refresh, err := service.Login(context.Background(), "alice", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.FindByID(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	repo := service.Repo()
	if err := repo.RefreshAccessToken(context.Background(), a.ID, refresh, "stale-access", time.Now().UTC()); err == nil {
		t.Fatal("stale refresh restored a logged-out session")
	}
	if err := repo.RenameWithToken(context.Background(), snapshot.Token, "stale-access", a.ID, "alice_new"); err == nil {
		t.Fatal("stale rename restored a logged-out session")
	}
	if _, _, err := service.Login(context.Background(), "alice", "correct-password"); err != nil {
		t.Fatal(err)
	}
	if err := service.ChangePassword(context.Background(), a.ID, "correct-password", "new-password"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Login(context.Background(), a.ID, snapshot.Password, "stale-access", "stale-refresh", time.Now().Add(time.Hour)); err == nil {
		t.Fatal("an in-flight old-password login restored a session after password change")
	}
}

func TestRabbitMQPopularityWireContract(t *testing.T) {
	raw := os.Getenv("INTEGRATION_AMQP_URL")
	if raw == "" {
		raw = "amqp://feedvid:feedvid_mq_dev_password@127.0.0.1:15672/"
	}
	conn, err := amqp.Dial(raw)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	base := &rabbitmq.RabbitMQ{Conn: conn}
	publisher, err := rabbitmq.NewPopularityMQ(base)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	queue, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.QueueBind(queue.Name, "video.popularity.update", "video.popularity.events", false, nil); err != nil {
		t.Fatal(err)
	}
	deliveries, err := ch.Consume(queue.Name, "", false, true, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := publisher.Update(ctx, 123, 1); err != nil {
		t.Fatal(err)
	}
	select {
	case delivery := <-deliveries:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(delivery.Body, &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"event_id", "video_id", "change", "occurred_at"} {
			if _, ok := fields[key]; !ok {
				t.Fatalf("missing event field %s", key)
			}
		}
		var event rabbitmq.PopularityEvent
		if err := json.Unmarshal(delivery.Body, &event); err != nil {
			t.Fatal(err)
		}
		if event.EventID == "" || event.VideoID != 123 || event.Change != 1 {
			t.Fatal("event did not survive the broker roundtrip")
		}
		if err := delivery.Ack(false); err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("RabbitMQ delivery timed out")
	}
}
