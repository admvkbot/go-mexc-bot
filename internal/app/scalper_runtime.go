package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/chscalper"
	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
	"github.com/mexc-bot/go-mexc-bot/internal/scalper"
)

func (b *Bot) runLiveScalper(ctx context.Context) {
	if b.ch == nil {
		log.Printf("mexc-bot: scalper disabled, clickhouse client is nil")
		return
	}
	journal := chscalper.NewWriter(b.ch)
	if err := journal.InitScalperSchema(ctx); err != nil {
		log.Printf("mexc-bot: scalper schema: %v", err)
		return
	}
	sessionID := fmt.Sprintf("live-%d", time.Now().UTC().UnixMilli())
	runtime := scalper.NewLiveRuntime(
		b.cfg.Scalper,
		scalper.NewOrderManager(b.cfg.Scalper, b.client, journal, sessionID, "live"),
	)
	for {
		if ctx.Err() != nil {
			return
		}
		if err := b.runOneLiveScalperSession(ctx, runtime); err != nil && ctx.Err() == nil {
			log.Printf("mexc-bot: scalper session ended: %v; reconnecting in 2s", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (b *Bot) runOneLiveScalperSession(ctx context.Context, runtime *scalper.LiveRuntime) error {
	ws, err := mexcfutures.NewContractWS(mexcfutures.WSConfig{PingInterval: 15 * time.Second})
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()
	if err := ws.Connect(ctx); err != nil {
		return fmt.Errorf("scalper ws connect: %w", err)
	}
	if err := ws.SubscribeDepth(b.cfg.Scalper.Symbol, false); err != nil {
		return fmt.Errorf("scalper sub.depth: %w", err)
	}
	if err := ws.SubscribeFullDepth(b.cfg.Scalper.Symbol, 20); err != nil {
		return fmt.Errorf("scalper sub.depth.full: %w", err)
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := ws.SetReadDeadline(time.Now().Add(8 * time.Second)); err != nil {
			return err
		}
		mt, raw, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		messageJSON, ok := decodeWSJSON(mt, raw)
		if !ok {
			continue
		}
		var frame mexcfutures.WSFrame
		if err := json.Unmarshal(messageJSON, &frame); err != nil {
			continue
		}
		if frame.Channel != "push.depth" && !strings.HasPrefix(frame.Channel, "push.depth.full") {
			continue
		}
		if err := runtime.HandleMessage(ctx, string(messageJSON), b.cfg.Scalper.Symbol, frame.Channel, time.Now().UTC()); err != nil {
			return err
		}
	}
}

func (b *Bot) runReplayScalper(ctx context.Context) error {
	if b.ch == nil {
		return fmt.Errorf("app: replay requires clickhouse")
	}
	journal := chscalper.NewWriter(b.ch)
	if err := journal.InitScalperSchema(ctx); err != nil {
		return fmt.Errorf("app: replay schema: %w", err)
	}
	sessionID := fmt.Sprintf("replay-%d", time.Now().UTC().UnixMilli())
	runtime := scalper.NewReplayRuntime(
		b.cfg.Scalper,
		scalper.NewOrderManager(b.cfg.Scalper, nil, journal, sessionID, "replay"),
	)
	rows, err := b.ch.QueryWSMarketRows(ctx, b.cfg.Scalper.Symbol, b.cfg.Scalper.ReplayStart, b.cfg.Scalper.ReplayEnd, b.cfg.Scalper.ReplayLimit)
	if err != nil {
		return fmt.Errorf("app: replay rows: %w", err)
	}
	log.Printf("mexc-bot: replaying %d ws rows for %s", len(rows), b.cfg.Scalper.Symbol)
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if row.Channel != "push.depth" && !strings.HasPrefix(row.Channel, "push.depth.full") {
			continue
		}
		if err := runtime.HandleMessage(ctx, row.MessageRaw, row.Symbol, row.Channel, row.IngestedAt); err != nil {
			return err
		}
	}
	return nil
}

func decodeWSJSON(messageType int, raw []byte) ([]byte, bool) {
	data := raw
	if messageType == websocket.BinaryMessage {
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, false
		}
		decompressed, err := io.ReadAll(gz)
		_ = gz.Close()
		if err != nil {
			return nil, false
		}
		data = decompressed
	}
	if len(data) == 0 || data[0] != '{' {
		return nil, false
	}
	return data, true
}
