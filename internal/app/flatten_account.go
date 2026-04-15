package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
)

// flattenAccountPositions cancels all open futures orders on the account and submits market reduce
// orders for every open position from GET open_positions.
func (b *Bot) flattenAccountPositions(ctx context.Context, logTag, extOIDPrefix string) error {
	if b == nil || b.client == nil {
		return nil
	}
	log.Printf("mexc-bot: [%s] cancel all orders (futures REST)", logTag)
	if _, err := b.client.CancelAllOrders(ctx, mexcfutures.CancelAllOrdersRequest{}); err != nil {
		return fmt.Errorf("cancel_all_orders: %w", err)
	}
	raw, err := b.client.OpenPositionsFutures(ctx, nil)
	if err != nil {
		return fmt.Errorf("open_positions: %w", err)
	}
	rows, err := mexcfutures.ParseOpenPositionsResponse(raw)
	if err != nil {
		return fmt.Errorf("parse open_positions: %w", err)
	}
	log.Printf("mexc-bot: [%s] open_positions: %d row(s) with holdVol>0 (will market-close each)", logTag, len(rows))
	if len(rows) == 0 {
		return nil
	}
	sc := b.cfg.Scalper
	for i, p := range rows {
		side := mexcfutures.OrderSideCloseLong
		if !p.IsLong {
			side = mexcfutures.OrderSideCloseShort
		}
		lev := p.Leverage
		if lev <= 0 {
			lev = sc.Leverage
		}
		openType := p.OpenType
		if openType <= 0 {
			openType = sc.OpenType
		}
		req := mexcfutures.SubmitOrderRequest{
			Symbol:        p.Symbol,
			Price:         0,
			Vol:           p.HoldVol,
			Side:          side,
			Type:          mexcfutures.OrderTypeMarket,
			OpenType:      openType,
			Leverage:      lev,
			ExternalOid:   fmt.Sprintf("%s-%s-%d-%d", extOIDPrefix, p.Symbol, time.Now().UnixNano(), i),
			PositionMode:  sc.PositionMode,
			ReduceOnly:    sc.PositionMode == mexcfutures.PositionModeOneWay,
			STPMode:       0,
			MarketCeiling: true,
		}
		if p.PositionID > 0 {
			req.PositionID = p.PositionID
		}
		log.Printf("mexc-bot: [%s] market close symbol=%s vol=%.8f long=%v position_id=%d", logTag, p.Symbol, p.HoldVol, p.IsLong, p.PositionID)
		out, serr := b.client.SubmitOrder(ctx, req)
		if serr != nil {
			return fmt.Errorf("submit market close %s: %w", p.Symbol, serr)
		}
		resp := string(out)
		if len(resp) > 512 {
			resp = resp[:512] + "..."
		}
		log.Printf("mexc-bot: [%s] market close submitted symbol=%s response=%s", logTag, p.Symbol, resp)
	}
	return nil
}

// StartupFlattenOpenPositions runs at process start when scalper is enabled: cancel-all then market-close
// every open futures position (same as shutdown flatten, different log / externalOid prefix).
func (b *Bot) StartupFlattenOpenPositions(ctx context.Context) error {
	if b == nil || !b.cfg.Mode.Scalper {
		return nil
	}
	return b.flattenAccountPositions(ctx, "startup", "startup")
}

// ShutdownFlattenAll cancels all orders and market-closes all open positions (SIGINT path).
func (b *Bot) ShutdownFlattenAll(ctx context.Context) error {
	if b == nil || !b.cfg.Mode.Scalper {
		return nil
	}
	return b.flattenAccountPositions(ctx, "shutdown", "shutdown")
}
