package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/chstore"
	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
	"github.com/mexc-bot/go-mexc-bot/internal/ports"
)

// Bot wires infrastructure and orchestrates the process lifecycle.
type Bot struct {
	cfg       config.Bot
	client    ports.FuturesREST
	ch        *chstore.Client
	wsSymbols []string
}

// New constructs a Bot with an injected futures REST client (tests may pass a stub).
func New(client ports.FuturesREST) *Bot {
	return &Bot{client: client}
}

// NewFromConfig builds the default MEXC REST client and ClickHouse writer from Bot settings.
func NewFromConfig(cfg config.Bot) (*Bot, error) {
	cli, err := mexcfutures.NewClient(mexcfutures.Config{WebKey: cfg.WebKey})
	if err != nil {
		return nil, fmt.Errorf("app: mexc client: %w", err)
	}
	chCli, err := chstore.Dial(context.Background(), chstore.ConfigFromEnv())
	if err != nil {
		return nil, fmt.Errorf("app: clickhouse: %w", err)
	}
	return &Bot{cfg: cfg, client: cli, ch: chCli, wsSymbols: cfg.WSSymbols}, nil
}

// Close releases outbound resources (ClickHouse connection).
func (b *Bot) Close() error {
	if b == nil || b.ch == nil {
		return nil
	}
	return b.ch.Close()
}

// Run executes the main loop (or one-shot bootstrap) until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	if err := b.client.TestConnection(ctx); err != nil {
		return fmt.Errorf("app: connectivity: %w", err)
	}
	log.Printf("mexc-bot: %s OK, ready", config.WebKeyEnv)

	if b.cfg.Mode.Replay && !b.cfg.Mode.Capture && !b.cfg.Mode.Scalper {
		return b.runReplayScalper(ctx)
	}

	if b.cfg.Mode.Capture {
		for _, sym := range b.wsSymbols {
			sym := sym
			go b.runWSMarketToClickHouse(ctx, sym)
		}
		log.Printf("mexc-bot: writing WS order book + deals for [%s] to ClickHouse (depth + depth.full 5/10/20 + deal)", strings.Join(b.wsSymbols, ","))
	}
	if b.cfg.Mode.Scalper {
		go b.runLiveScalper(ctx)
	}
	<-ctx.Done()
	return ctx.Err()
}
