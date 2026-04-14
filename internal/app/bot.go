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
	client    ports.FuturesREST // nil in replay-only mode (no MEXC REST needed)
	ch        *chstore.Client
	wsSymbols []string
}

// New constructs a Bot with an injected futures REST client (tests may pass a stub).
func New(client ports.FuturesREST) *Bot {
	return &Bot{client: client}
}

// NewFromConfig builds the default MEXC REST client and ClickHouse writer from Bot settings.
func NewFromConfig(cfg config.Bot) (*Bot, error) {
	chCli, err := chstore.Dial(context.Background(), chstore.ConfigFromEnv())
	if err != nil {
		return nil, fmt.Errorf("app: clickhouse: %w", err)
	}

	replayOnly := cfg.Mode.Replay && !cfg.Mode.Capture && !cfg.Mode.Scalper
	var cli ports.FuturesREST
	switch {
	case replayOnly:
		cli = nil
	case cfg.Mode.Scalper:
		if cfg.TradeWebKey == "" {
			return nil, fmt.Errorf("app: %s is empty (required for scalper)", config.TradeWebKeyEnv)
		}
		c, err := mexcfutures.NewClient(mexcfutures.Config{WebKey: cfg.TradeWebKey})
		if err != nil {
			return nil, fmt.Errorf("app: mexc trade client: %w", err)
		}
		cli = c
	case cfg.Mode.Capture:
		if cfg.SourceWebKey == "" {
			return nil, fmt.Errorf("app: %s is empty (required for capture)", config.SourceWebKeyEnv)
		}
		c, err := mexcfutures.NewClient(mexcfutures.Config{WebKey: cfg.SourceWebKey})
		if err != nil {
			return nil, fmt.Errorf("app: mexc source client: %w", err)
		}
		cli = c
	default:
		return nil, fmt.Errorf("app: unsupported MEXC_BOT_MODE combination")
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
	replayOnly := b.cfg.Mode.Replay && !b.cfg.Mode.Capture && !b.cfg.Mode.Scalper
	if replayOnly {
		log.Printf("mexc-bot: replay mode (ClickHouse only)")
		return b.runReplayScalper(ctx)
	}
	if b.client != nil {
		if err := b.client.TestConnection(ctx); err != nil {
			return fmt.Errorf("app: connectivity: %w", err)
		}
	}
	if b.cfg.Mode.Scalper {
		log.Printf("mexc-bot: MEXC REST OK (%s)", config.TradeWebKeyEnv)
	} else if b.cfg.Mode.Capture {
		log.Printf("mexc-bot: MEXC REST OK (%s)", config.SourceWebKeyEnv)
	} else {
		log.Printf("mexc-bot: ready")
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
