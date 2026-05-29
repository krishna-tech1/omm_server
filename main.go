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

	ctx := context.Background()
	pool, err := infra.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	queries := db.New(pool)
	app := http.NewServer(cfg, queries, pool)

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Printf("server stopped: %v", err)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	<-shutdown

	_ = app.Shutdown()
}
