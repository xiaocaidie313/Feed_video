package main

import (
	"context"
	"errors"
	"feedvid/internal/config"
	"feedvid/internal/db"
	apphttp "feedvid/internal/http"
	"feedvid/internal/midware/rabbitmq"
	rediscache "feedvid/internal/midware/redis"
	"feedvid/internal/observability"
	"github.com/joho/godotenv"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "configs/config.yaml"
	}
	cfg, usedDefault, err := config.LoadLocalDev(path)
	if err != nil {
		return err
	}
	log.Printf("config loaded (development defaults: %t)", usedDefault)
	sqlDB, err := db.NewDB(db.Dbconfig{Host: cfg.Database.Host, Port: cfg.Database.Port, User: cfg.Database.User, Password: cfg.Database.Password, DbName: cfg.Database.DbName})
	if err != nil {
		return err
	}
	defer db.CloseDB(sqlDB)
	if err := db.AutoMigrate(sqlDB); err != nil {
		return err
	}
	var cache *rediscache.Client
	if cfg.Redis.Enabled {
		cache, err = rediscache.NewFromEnv(&cfg.Redis)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			err = cache.Ping(ctx)
			cancel()
		}
		if err != nil {
			log.Printf("Redis unavailable; optional cache and chunk upload disabled: %v", err)
			if cache != nil {
				_ = cache.Close()
			}
			cache = nil
		}
	}
	if cache != nil {
		defer cache.Close()
	}
	var mq *rabbitmq.RabbitMQ
	if cfg.RabbitMQ.Enabled {
		mq, err = rabbitmq.NewRabbitMQ(&cfg.RabbitMQ)
		if err != nil {
			log.Printf("RabbitMQ unavailable; optional asynchronous features disabled: %v", err)
			mq = nil
		}
	}
	if mq != nil {
		defer mq.Close()
	}
	profiler, err := observability.NewPprofServer("API", cfg.Observability.Pprof.Enabled, cfg.Observability.Pprof.ApiAddr)
	if err != nil {
		return err
	}
	if profiler != nil {
		defer profiler.Close()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: ":" + strconv.Itoa(cfg.Server.Port), Handler: apphttp.SetRouter(sqlDB, cache, mq), ReadHeaderTimeout: 5 * time.Second}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	log.Printf("API listening on %s", server.Addr)
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}
