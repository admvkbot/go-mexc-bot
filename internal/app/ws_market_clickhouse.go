package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/chstore"
	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
)

func (b *Bot) runWSMarketToClickHouse(ctx context.Context, symbol string) {
	if b.ch == nil {
		return
	}
	if err := b.ch.InitMarketWSSchema(ctx); err != nil {
		log.Printf("mexc-bot: clickhouse schema: %v", err)
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		err := b.captureWSOneSession(ctx, symbol)
		if ctx.Err() != nil {
			return
		}
		log.Printf("mexc-bot: ws→clickhouse session ended: %v; reconnecting in 3s", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (b *Bot) captureWSOneSession(ctx context.Context, symbol string) error {
	ws, err := mexcfutures.NewContractWS(mexcfutures.WSConfig{PingInterval: 15 * time.Second})
	if err != nil {
		return err
	}
	defer func() { _ = ws.Close() }()

	if err := ws.Connect(ctx); err != nil {
		return fmt.Errorf("ws connect: %w", err)
	}
	if err := ws.SubscribeDepth(symbol, false); err != nil {
		return fmt.Errorf("sub.depth: %w", err)
	}
	for _, lim := range []int{5, 10, 20} {
		if err := ws.SubscribeFullDepth(symbol, lim); err != nil {
			return fmt.Errorf("sub.depth.full %d: %w", lim, err)
		}
	}
	if err := ws.SubscribeDeals(symbol); err != nil {
		return fmt.Errorf("sub.deal: %w", err)
	}
	log.Printf("mexc-bot: ws capture %s: connected, subscribed (depth + depth.full + deal); streaming to ClickHouse", symbol)

	var batch []chstore.WSMarketRow
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		rows := batch
		batch = batch[:0]
		sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return b.ch.InsertWSMarketRows(sendCtx, rows)
	}

	readWait := 8 * time.Second
	lastFlush := time.Now()
	lastStat := time.Now()
	var framesWindow int64
	for {
		if ctx.Err() != nil {
			_ = ws.Close()
			_ = flush()
			return ctx.Err()
		}
		if time.Since(lastStat) >= 30*time.Second {
			log.Printf("mexc-bot: ws capture %s: %d market frames last 30s (0 = no push.depth/deal or filter mismatch)", symbol, framesWindow)
			framesWindow = 0
			lastStat = time.Now()
		}
		if time.Since(lastFlush) >= 500*time.Millisecond {
			if err := flush(); err != nil {
				return err
			}
			lastFlush = time.Now()
		}

		if err := ws.SetReadDeadline(time.Now().Add(readWait)); err != nil {
			return err
		}
		mt, data, err := ws.ReadMessage()
		if err != nil {
			_ = flush()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			return err
		}
		row, ok := wsPayloadToRow(mt, data, symbol)
		if !ok {
			continue
		}
		framesWindow++
		if framesWindow == 1 {
			log.Printf("mexc-bot: ws capture %s: first frame (channel=%s)", symbol, row.Channel)
		}
		batch = append(batch, row)
		if len(batch) >= 400 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
}

func wsPayloadToRow(messageType int, raw []byte, wantSymbol string) (chstore.WSMarketRow, bool) {
	data := raw
	if messageType == websocket.BinaryMessage {
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return chstore.WSMarketRow{}, false
		}
		decompressed, err := io.ReadAll(gz)
		_ = gz.Close()
		if err != nil {
			return chstore.WSMarketRow{}, false
		}
		data = decompressed
	}
	if len(data) == 0 || data[0] != '{' {
		return chstore.WSMarketRow{}, false
	}
	var frame mexcfutures.WSFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		return chstore.WSMarketRow{}, false
	}
	if !isOrderBookOrDealChannel(frame.Channel) {
		return chstore.WSMarketRow{}, false
	}
	sym := strings.TrimSpace(frame.Symbol)
	if sym == "" {
		sym = wantSymbol
	}
	if sym != wantSymbol && !strings.Contains(string(data), "\""+wantSymbol+"\"") {
		return chstore.WSMarketRow{}, false
	}
	return chstore.WSMarketRow{
		IngestedAt: time.Now().UTC(),
		ExchangeTS: frame.TS,
		Symbol:     wantSymbol,
		Channel:    frame.Channel,
		MessageRaw: string(data),
	}, true
}

func isOrderBookOrDealChannel(ch string) bool {
	switch ch {
	case "push.depth", "push.deal":
		return true
	default:
		return strings.HasPrefix(ch, "push.depth.full")
	}
}
