package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mexc-bot/go-mexc-bot/internal/app"
	"github.com/mexc-bot/go-mexc-bot/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	bot, err := app.NewFromConfig(cfg)
	if err != nil {
		log.Fatalf("app: %v", err)
	}
	defer func() { _ = bot.Close() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := bot.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("run: %v", err)
	}
}
