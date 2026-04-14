package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
)

// ShutdownFlattenAll cancels all open futures orders on the account and submits market reduce orders
// for every open position row returned by open_positions. Safe to call when the bot was started with
// scalper (trade REST client); no-ops if there is no trade client or scalper mode is off.
func (b *Bot) ShutdownFlattenAll(ctx context.Context) error {
	if b == nil || b.client == nil || !b.cfg.Mode.Scalper {
		return nil
	}
	log.Printf("[shutdown] cancel all orders (futures REST)")
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
	if len(rows) == 0 {
		log.Printf("[shutdown] no open positions to flatten")
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
			ExternalOid:   fmt.Sprintf("shutdown-%s-%d-%d", p.Symbol, time.Now().UnixNano(), i),
			PositionMode:  sc.PositionMode,
			ReduceOnly:    sc.PositionMode == mexcfutures.PositionModeOneWay,
			STPMode:       0,
			MarketCeiling: true,
		}
		if p.PositionID > 0 {
			req.PositionID = p.PositionID
		}
		log.Printf("[shutdown] market close symbol=%s vol=%.8f long=%v position_id=%d", p.Symbol, p.HoldVol, p.IsLong, p.PositionID)
		out, serr := b.client.SubmitOrder(ctx, req)
		if serr != nil {
			return fmt.Errorf("submit market close %s: %w", p.Symbol, serr)
		}
		resp := string(out)
		if len(resp) > 512 {
			resp = resp[:512] + "..."
		}
		log.Printf("[shutdown] market close submitted symbol=%s response=%s", p.Symbol, resp)
	}
	return nil
}
