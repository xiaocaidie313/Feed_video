package main

import (
	"context"
	"errors"
	"feedvid/internal/config"
	"feedvid/internal/db"
	"feedvid/internal/video"
	"github.com/joho/godotenv"
	"log"
	"os"
	"os/signal"
	"syscall"
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
	cfg, _, err := config.LoadLocalDev(path)
	if err != nil {
		return err
	}
	database, err := db.NewDB(db.Dbconfig{Host: cfg.Database.Host, Port: cfg.Database.Port, User: cfg.Database.User, Password: cfg.Database.Password, DbName: cfg.Database.DbName})
	if err != nil {
		return err
	}
	defer db.CloseDB(database)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	count, err := video.NewLikeRepo(database).ReconcileCounts(ctx)
	if err == nil {
		log.Printf("Reconciled like counters for %d videos", count)
	}
	return err
}
