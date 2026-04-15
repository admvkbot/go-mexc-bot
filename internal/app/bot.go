package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

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

	if cfg.Mode.Scalper && cli != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		enrichScalperOrderScalesFromContract(ctx, cli, &cfg.Scalper)
		cancel()
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

	// Before any capture/scalper WS: flatten account (scalper + trade REST only).
	if b.cfg.Mode.Scalper && b.client != nil {
		log.Printf("mexc-bot: startup: begin flatten (cancel all orders + market-close open positions)")
		flatCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		err := b.StartupFlattenOpenPositions(flatCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("app: startup flatten open positions: %w", err)
		}
		log.Printf("mexc-bot: startup: flatten finished OK")
	}

	if wd := scalperWarmupDuration(); b.cfg.Mode.Scalper && b.client != nil && wd > 0 {
		warmCtx, cancel := context.WithTimeout(ctx, wd+45*time.Second)
		if err := b.RunScalperWarmupMetrics(warmCtx, wd); err != nil {
			log.Printf("mexc-bot: warmup: %v (продолжаем запуск)", err)
		}
		cancel()
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

// enrichScalperOrderScalesFromContract fills OrderPriceScale / OrderVolScale from public GET /contract/detail
// so submit JSON matches MEXC precision (avoids success=false code 2015).
func enrichScalperOrderScalesFromContract(ctx context.Context, cli ports.FuturesREST, s *config.Scalper) {
	if s == nil {
		return
	}
	c, ok := cli.(*mexcfutures.Client)
	if !ok {
		return
	}
	raw, err := c.GetContractDetailContractPublic(ctx, s.Symbol)
	if err != nil {
		log.Printf("mexc-bot: contract/detail symbol=%s: %v", s.Symbol, err)
		return
	}
	detail, err := mexcfutures.ParseContractDetailSummary(raw)
	if err != nil {
		log.Printf("mexc-bot: contract/detail parse symbol=%s: %v", s.Symbol, err)
		return
	}
	if detail.Data == nil {
		return
	}
	if s.OrderPriceScale == nil {
		if _, ok := detail.Data["priceScale"]; ok {
			ps := detail.PriceScale
			s.OrderPriceScale = &ps
		}
	}
	if s.OrderVolScale == nil {
		if _, ok := detail.Data["volScale"]; ok {
			vs := detail.VolScale
			s.OrderVolScale = &vs
		}
	}
	if s.OrderPriceScale != nil && s.OrderVolScale != nil {
		log.Printf("mexc-bot: contract %s: order JSON price decimals=%d vol decimals=%d", s.Symbol, *s.OrderPriceScale, *s.OrderVolScale)
	}
}
