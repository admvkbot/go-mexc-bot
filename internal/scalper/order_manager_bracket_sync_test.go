package scalper

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
)

type stubFuturesREST struct {
	openPositionsBodies [][]byte
	cancelledExternal   []string
}

func (s *stubFuturesREST) TestConnection(ctx context.Context) error { return nil }
func (s *stubFuturesREST) SubmitOrder(ctx context.Context, req mexcfutures.SubmitOrderRequest) ([]byte, error) {
	return nil, nil
}
func (s *stubFuturesREST) CancelOrder(ctx context.Context, orderIDs []int64) ([]byte, error) {
	return nil, nil
}
func (s *stubFuturesREST) CancelOrderByExternalID(ctx context.Context, req mexcfutures.CancelOrderByExternalIDRequest) ([]byte, error) {
	s.cancelledExternal = append(s.cancelledExternal, req.ExternalOid)
	return []byte(`{"success":true}`), nil
}
func (s *stubFuturesREST) CancelAllOrders(ctx context.Context, req mexcfutures.CancelAllOrdersRequest) ([]byte, error) {
	return nil, nil
}
func (s *stubFuturesREST) GetOrder(ctx context.Context, orderID string) ([]byte, error) {
	return nil, nil
}
func (s *stubFuturesREST) GetOrderByExternalID(ctx context.Context, symbol, externalOid string) ([]byte, error) {
	return nil, nil
}
func (s *stubFuturesREST) OpenPositionsFutures(ctx context.Context, symbol *string) ([]byte, error) {
	if len(s.openPositionsBodies) == 0 {
		return openPositionsBody(nil)
	}
	body := s.openPositionsBodies[0]
	s.openPositionsBodies = s.openPositionsBodies[1:]
	return body, nil
}

func TestSyncExchangeBracketClose_partialFill(t *testing.T) {
	cfg := config.Scalper{
		Symbol:            "TAO_USDT",
		ExitMode:          "bracket",
		TickSize:          0.01,
		ProfitTargetTicks: 3,
		StopLossTicks:     1,
	}
	trader := &stubFuturesREST{
		openPositionsBodies: [][]byte{mustOpenPositionsBody(t, "TAO_USDT", true, 1)},
	}
	mgr := NewOrderManager(cfg, trader, nil, "session", "live")
	now := time.Now().UTC()
	ladder := &LadderContext{
		LadderID:      "ladder-1",
		Symbol:        cfg.Symbol,
		Side:          SideLong,
		Phase:         PhaseInventoryOpen,
		NetQuantity:   3,
		AvgEntryPrice: 100,
		EntryOrders: []*ManagedOrder{{
			Class:       OrderClassEntry,
			Side:        SideLong,
			ExternalOID: "entry-1",
			StateCode:   mexcfutures.OrderStateUnfilled,
			Quantity:    3,
			SubmittedAt: now,
		}},
	}

	if err := mgr.syncExchangeBracketClose(context.Background(), ladder, 100.04, now); err != nil {
		t.Fatal(err)
	}
	if ladder.NetQuantity != 1 {
		t.Fatalf("want remaining qty 1, got %v", ladder.NetQuantity)
	}
	if ladder.ExitOrder == nil {
		t.Fatal("expected synthetic exit order")
	}
	if ladder.ExitOrder.FilledQty != 2 {
		t.Fatalf("want synthetic filled qty 2, got %v", ladder.ExitOrder.FilledQty)
	}
	if ladder.ExitOrder.StateCode != mexcfutures.OrderStateUnfilled {
		t.Fatalf("want partial exit state %d, got %d", mexcfutures.OrderStateUnfilled, ladder.ExitOrder.StateCode)
	}
	if ladder.ExitReason != "take_profit_ticks" {
		t.Fatalf("want take_profit_ticks, got %q", ladder.ExitReason)
	}
	if len(trader.cancelledExternal) != 0 {
		t.Fatalf("did not expect entry cancels on partial fill, got %v", trader.cancelledExternal)
	}
}

func TestSyncExchangeBracketClose_fullCloseCancelsEntries(t *testing.T) {
	cfg := config.Scalper{
		Symbol:            "TAO_USDT",
		ExitMode:          "bracket",
		TickSize:          0.01,
		ProfitTargetTicks: 3,
		StopLossTicks:     1,
	}
	trader := &stubFuturesREST{
		openPositionsBodies: [][]byte{
			mustOpenPositionsBody(t, "TAO_USDT", true, 0),
			mustOpenPositionsBody(t, "TAO_USDT", true, 0),
		},
	}
	mgr := NewOrderManager(cfg, trader, nil, "session", "live")
	now := time.Now().UTC()
	entry := &ManagedOrder{
		Class:       OrderClassEntry,
		Side:        SideLong,
		ExternalOID: "entry-1",
		StateCode:   mexcfutures.OrderStateUnfilled,
		Quantity:    3,
		SubmittedAt: now,
	}
	ladder := &LadderContext{
		LadderID:      "ladder-1",
		Symbol:        cfg.Symbol,
		Side:          SideLong,
		Phase:         PhaseInventoryOpen,
		NetQuantity:   3,
		AvgEntryPrice: 100,
		EntryOrders:   []*ManagedOrder{entry},
	}

	if err := mgr.syncExchangeBracketClose(context.Background(), ladder, 99.98, now); err != nil {
		t.Fatal(err)
	}
	if ladder.NetQuantity != 0 {
		t.Fatalf("want remaining qty 0, got %v", ladder.NetQuantity)
	}
	if ladder.ExitReason != "stop_loss_ticks" {
		t.Fatalf("want stop_loss_ticks, got %q", ladder.ExitReason)
	}
	if ladder.ExitOrder == nil || ladder.ExitOrder.StateCode != mexcfutures.OrderStateFilled {
		t.Fatalf("expected filled synthetic exit order, got %+v", ladder.ExitOrder)
	}
	if got := len(trader.cancelledExternal); got != 1 || trader.cancelledExternal[0] != "entry-1" {
		t.Fatalf("want entry-1 canceled, got %v", trader.cancelledExternal)
	}
	if entry.StateCode != mexcfutures.OrderStateCanceled {
		t.Fatalf("want entry order canceled locally, got %d", entry.StateCode)
	}
}

func mustOpenPositionsBody(t *testing.T, symbol string, isLong bool, holdVol float64) []byte {
	t.Helper()
	body, err := openPositionsBody([]openPositionSpec{{
		Symbol:  symbol,
		IsLong:  isLong,
		HoldVol: holdVol,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

type openPositionSpec struct {
	Symbol  string
	IsLong  bool
	HoldVol float64
}

func openPositionsBody(specs []openPositionSpec) ([]byte, error) {
	rows := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		if spec.HoldVol <= 0 {
			continue
		}
		positionType := mexcfutures.PositionTypeShort
		if spec.IsLong {
			positionType = mexcfutures.PositionTypeLong
		}
		rows = append(rows, map[string]any{
			"positionId":   1,
			"symbol":       spec.Symbol,
			"positionType": positionType,
			"holdVol":      spec.HoldVol,
			"openType":     1,
			"leverage":     10,
		})
	}
	return json.Marshal(map[string]any{
		"success": true,
		"code":    0,
		"data":    rows,
	})
}
