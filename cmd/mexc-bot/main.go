package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	err = bot.Run(ctx)
	if errors.Is(err, context.Canceled) {
		shCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if e := bot.ShutdownFlattenAll(shCtx); e != nil {
			log.Printf("[shutdown] %v", e)
		}
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("run: %v", err)
	}
}
