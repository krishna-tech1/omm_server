package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"one-more-mile/server/internal/config"
	"one-more-mile/server/internal/http"
	"one-more-mile/server/internal/infra"
	db "one-more-mile/server/internal/sqlc"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := infra.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	redisClient, err := infra.NewRedisClient(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer redisClient.Close()

	queries := db.New(pool)
	app := http.NewServer(ctx, cfg, queries, pool, redisClient)

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Printf("server stopped: %v", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	<-shutdown

	cancel() // cancel the context to stop background workers
	_ = app.Shutdown()
}
