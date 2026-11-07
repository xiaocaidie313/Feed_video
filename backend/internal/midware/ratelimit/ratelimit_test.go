package ratelimit

import (
	rediscache "feedvid/internal/midware/redis"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLimitIsolatedByClientNamespaceAndExpires(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := miniredis.RunT(t)
	router := func(prefix string) *gin.Engine {
		client := rediscache.NewClient(goredis.NewClient(&goredis.Options{Addr: server.Addr()}), prefix)
		t.Cleanup(func() { client.Close() })
		r := gin.New()
		r.GET("/", Limit(client, "login", 1, time.Minute, func(*gin.Context) (string, bool) { return "same-user", true }), func(c *gin.Context) { c.Status(200) })
		return r
	}
	a, b := router("environment-a:"), router("environment-b:")
	check := func(r *gin.Engine, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != want {
			t.Fatalf("status %d, want %d", w.Code, want)
		}
	}
	check(a, 200)
	check(a, 429)
	check(b, 200)
	server.FastForward(time.Minute)
	check(a, 200)
}
