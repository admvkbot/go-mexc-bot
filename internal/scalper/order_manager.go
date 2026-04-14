package scalper

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
	"github.com/mexc-bot/go-mexc-bot/internal/ports"
)

type Journal interface {
	InitScalperSchema(ctx context.Context) error
	InsertScalperSignalEvents(ctx context.Context, rows []SignalEvent) error
	InsertScalperOrderEvents(ctx context.Context, rows []OrderEvent) error
	InsertScalperRoundTrips(ctx context.Context, rows []RoundTrip) error
	InsertScalperReplayCandidates(ctx context.Context, rows []ReplayCandidate) error
}

type OrderManager struct {
	cfg      config.Scalper
	trader   ports.FuturesREST
	journal  Journal
	session  string
	modeName string
}

func NewOrderManager(cfg config.Scalper, trader ports.FuturesREST, journal Journal, sessionID, modeName string) *OrderManager {
	return &OrderManager{
		cfg:      cfg,
		trader:   trader,
		journal:  journal,
		session:  sessionID,
		modeName: modeName,
	}
}

func (m *OrderManager) EmitSignal(ctx context.Context, decision Decision) {
	if m == nil || m.journal == nil {
		return
	}
	s := decision.Features.Snapshot
	_ = m.journal.InsertScalperSignalEvents(ctx, []SignalEvent{{
		SessionID:      m.session,
		Mode:           m.modeName,
		Symbol:         s.Symbol,
		EventAt:        s.At,
		Action:         string(decision.Action),
		Side:           sideToString(decision.Side),
		Score:          decision.Score,
		Reason:         decision.Reason,
		BestBidPx:      s.BestBidPx,
		BestAskPx:      s.BestAskPx,
		Spread:         s.Spread,
		BidVol5:        s.BidVol5,
		AskVol5:        s.AskVol5,
		Imbalance5:     s.Imbalance5,
		BidPulseTicks:  decision.Features.BidPulseTicks,
		AskPulseTicks:  decision.Features.AskPulseTicks,
		PressureDelta:  decision.Features.PressureDelta,
		UpdateRate:     decision.Features.UpdateRate,
		MicroPriceDiff: decision.Features.MicroPriceDelta,
	}})
}

func (m *OrderManager) EmitReplayCandidate(ctx context.Context, row ReplayCandidate) {
	if m == nil || m.journal == nil {
		return
	}
	_ = m.journal.InsertScalperReplayCandidates(ctx, []ReplayCandidate{row})
}

func (m *OrderManager) PlaceEntry(ctx context.Context, ladder *LadderContext, side Side, quantity, price float64, reason string) (*ManagedOrder, error) {
	req := mexcfutures.SubmitOrderRequest{
		Symbol:       m.cfg.Symbol,
		Price:        normalizePrice(price, m.cfg.TickSize),
		Vol:          quantity,
		Side:         entryOrderSide(side),
		Type:         mexcfutures.OrderTypeLimit,
		OpenType:     m.cfg.OpenType,
		Leverage:     m.cfg.Leverage,
		ExternalOid:  newSessionID("entry"),
		PositionMode: m.cfg.PositionMode,
		STPMode:      0,
	}
	raw, err := m.trader.SubmitOrder(ctx, req)
	order := &ManagedOrder{
		Class:       OrderClassEntry,
		Side:        side,
		ExternalOID: req.ExternalOid,
		Price:       req.Price,
		Quantity:    quantity,
		SubmittedAt: time.Now().UTC(),
		LastUpdate:  time.Now().UTC(),
		Reason:      reason,
	}
	if err != nil {
		m.emitOrderEvent(ctx, ladder, order, "submit_error", reason, "")
		return nil, err
	}
	order.OrderID = parseSubmitOrderID(raw)
	m.emitOrderEvent(ctx, ladder, order, "submit", reason, string(raw))
	return order, nil
}

func (m *OrderManager) PlaceExit(ctx context.Context, ladder *LadderContext, price float64, reason string) (*ManagedOrder, error) {
	if ladder == nil || ladder.NetQuantity <= 0 {
		return nil, fmt.Errorf("scalper: exit requires inventory")
	}
	req := mexcfutures.SubmitOrderRequest{
		Symbol:       m.cfg.Symbol,
		Price:        normalizePrice(price, m.cfg.TickSize),
		Vol:          ladder.NetQuantity,
		Side:         exitOrderSide(ladder.Side),
		Type:         mexcfutures.OrderTypeLimit,
		OpenType:     m.cfg.OpenType,
		ExternalOid:  newSessionID("exit"),
		PositionMode: m.cfg.PositionMode,
		ReduceOnly:   m.cfg.PositionMode == mexcfutures.PositionModeOneWay,
		STPMode:      0,
	}
	raw, err := m.trader.SubmitOrder(ctx, req)
	order := &ManagedOrder{
		Class:       OrderClassExit,
		Side:        ladder.Side,
		ExternalOID: req.ExternalOid,
		Price:       req.Price,
		Quantity:    req.Vol,
		SubmittedAt: time.Now().UTC(),
		LastUpdate:  time.Now().UTC(),
		Reason:      reason,
		ReduceOnly:  req.ReduceOnly,
	}
	if err != nil {
		m.emitOrderEvent(ctx, ladder, order, "submit_error", reason, "")
		return nil, err
	}
	order.OrderID = parseSubmitOrderID(raw)
	m.emitOrderEvent(ctx, ladder, order, "submit", reason, string(raw))
	return order, nil
}

func (m *OrderManager) EmergencyFlatten(ctx context.Context, ladder *LadderContext, priceHint float64, reason string) (*ManagedOrder, error) {
	if ladder == nil || ladder.NetQuantity <= 0 {
		return nil, nil
	}
	_, _ = m.trader.CancelAllOrders(ctx, mexcfutures.CancelAllOrdersRequest{Symbol: m.cfg.Symbol})
	if !m.cfg.AllowEmergencyMarket {
		return nil, fmt.Errorf("scalper: emergency market disabled")
	}
	req := mexcfutures.SubmitOrderRequest{
		Symbol:        m.cfg.Symbol,
		Price:         normalizePrice(priceHint, m.cfg.TickSize),
		Vol:           ladder.NetQuantity,
		Side:          exitOrderSide(ladder.Side),
		Type:          mexcfutures.OrderTypeMarket,
		OpenType:      m.cfg.OpenType,
		ExternalOid:   newSessionID("flatten"),
		PositionMode:  m.cfg.PositionMode,
		ReduceOnly:    m.cfg.PositionMode == mexcfutures.PositionModeOneWay,
		STPMode:       0,
		MarketCeiling: true,
	}
	raw, err := m.trader.SubmitOrder(ctx, req)
	order := &ManagedOrder{
		Class:       OrderClassEmergency,
		Side:        ladder.Side,
		ExternalOID: req.ExternalOid,
		Price:       req.Price,
		Quantity:    req.Vol,
		SubmittedAt: time.Now().UTC(),
		LastUpdate:  time.Now().UTC(),
		Reason:      reason,
	}
	if err != nil {
		m.emitOrderEvent(ctx, ladder, order, "flatten_error", reason, "")
		return nil, err
	}
	order.OrderID = parseSubmitOrderID(raw)
	m.emitOrderEvent(ctx, ladder, order, "flatten_submit", reason, string(raw))
	return order, nil
}

func (m *OrderManager) SyncLadder(ctx context.Context, ladder *LadderContext) error {
	if ladder == nil {
		return nil
	}
	for _, order := range ladder.EntryOrders {
		if order == nil || isTerminalState(order.StateCode) {
			continue
		}
		remote, raw, err := m.fetchOrder(ctx, order.ExternalOID)
		if err != nil {
			continue
		}
		m.applyRemoteOrder(ctx, ladder, order, remote, raw)
	}
	if ladder.ExitOrder != nil && !isTerminalState(ladder.ExitOrder.StateCode) {
		remote, raw, err := m.fetchOrder(ctx, ladder.ExitOrder.ExternalOID)
		if err == nil {
			m.applyRemoteOrder(ctx, ladder, ladder.ExitOrder, remote, raw)
		}
	}
	if ladder.EmergencyOrder != nil && !isTerminalState(ladder.EmergencyOrder.StateCode) {
		remote, raw, err := m.fetchOrder(ctx, ladder.EmergencyOrder.ExternalOID)
		if err == nil {
			m.applyRemoteOrder(ctx, ladder, ladder.EmergencyOrder, remote, raw)
		}
	}
	return nil
}

func (m *OrderManager) RepriceEntries(ctx context.Context, ladder *LadderContext, snapshot Snapshot, now time.Time) error {
	for i, order := range ladder.EntryOrders {
		if order == nil || isTerminalState(order.StateCode) {
			continue
		}
		if order.Reprices >= m.cfg.MaxReprices {
			continue
		}
		if now.Sub(order.SubmittedAt) < m.cfg.EntryTTL && now.Sub(order.LastUpdate) < m.cfg.RepriceInterval {
			continue
		}
		remaining := order.Quantity - order.FilledQty
		if remaining <= 0 {
			continue
		}
		_ = m.cancelByExternalID(ctx, order.ExternalOID)
		repriced, err := m.PlaceEntry(ctx, ladder, ladder.Side, remaining, entryReferencePrice(snapshot, ladder.Side), "entry_reprice")
		if err != nil {
			return err
		}
		repriced.Reprices = order.Reprices + 1
		ladder.RepricesCount++
		ladder.EntryOrders[i] = repriced
	}
	return nil
}

func (m *OrderManager) EnsureExit(ctx context.Context, ladder *LadderContext, snapshot Snapshot, now time.Time) error {
	if ladder == nil || ladder.NetQuantity <= 0 {
		return nil
	}
	target := targetExitPrice(ladder, snapshot, m.cfg)
	if ladder.ExitOrder == nil || isTerminalState(ladder.ExitOrder.StateCode) {
		exitOrder, err := m.PlaceExit(ctx, ladder, target, "target_exit")
		if err != nil {
			return err
		}
		ladder.UpsertExitOrder(exitOrder)
		return nil
	}
	if now.Sub(ladder.ExitOrder.SubmittedAt) >= m.cfg.ExitTTL || now.Sub(ladder.ExitOrder.LastUpdate) >= m.cfg.RepriceInterval {
		remaining := ladder.NetQuantity
		_ = m.cancelByExternalID(ctx, ladder.ExitOrder.ExternalOID)
		exitOrder, err := m.PlaceExit(ctx, ladder, target, "exit_reprice")
		if err != nil {
			return err
		}
		exitOrder.Quantity = remaining
		exitOrder.Reprices = ladder.ExitOrder.Reprices + 1
		ladder.RepricesCount++
		ladder.UpsertExitOrder(exitOrder)
	}
	return nil
}

func (m *OrderManager) FlushRoundTrip(ctx context.Context, ladder *LadderContext) {
	if m == nil || m.journal == nil || ladder == nil || ladder.RoundTripWritten {
		return
	}
	ladder.RoundTripWritten = true
	_ = m.journal.InsertScalperRoundTrips(ctx, []RoundTrip{ladder.BuildRoundTrip(m.cfg.TickSize)})
}

func (m *OrderManager) emitOrderEvent(ctx context.Context, ladder *LadderContext, order *ManagedOrder, eventType, reason, raw string) {
	if m == nil || m.journal == nil || order == nil {
		return
	}
	row := OrderEvent{
		SessionID:   m.session,
		Symbol:      m.cfg.Symbol,
		EventAt:     time.Now().UTC(),
		EventType:   eventType,
		OrderClass:  string(order.Class),
		Side:        sideToString(order.Side),
		ExternalOID: order.ExternalOID,
		OrderID:     order.OrderID,
		Price:       order.Price,
		Quantity:    order.Quantity,
		FilledQty:   order.FilledQty,
		AvgFillPx:   order.AvgFillPx,
		StateCode:   order.StateCode,
		Reason:      reason,
		RawJSON:     raw,
	}
	if ladder != nil {
		row.LadderID = ladder.LadderID
	}
	_ = m.journal.InsertScalperOrderEvents(ctx, []OrderEvent{row})
}

func (m *OrderManager) fetchOrder(ctx context.Context, externalOID string) (remoteOrder, string, error) {
	raw, err := m.trader.GetOrderByExternalID(ctx, m.cfg.Symbol, externalOID)
	if err != nil {
		return remoteOrder{}, "", err
	}
	order, err := parseRemoteOrder(raw)
	return order, string(raw), err
}

func (m *OrderManager) applyRemoteOrder(ctx context.Context, ladder *LadderContext, local *ManagedOrder, remote remoteOrder, raw string) {
	local.OrderID = remote.OrderID
	local.StateCode = remote.State
	local.LastUpdate = remote.UpdateTime
	if local.LastUpdate.IsZero() {
		local.LastUpdate = time.Now().UTC()
	}
	if remote.AvgFillPrice > 0 {
		local.AvgFillPx = remote.AvgFillPrice
	}
	deltaQty := remote.DealVol - local.FilledQty
	local.FilledQty = remote.DealVol
	if deltaQty > 0 {
		fillPx := remote.AvgFillPrice
		if fillPx == 0 {
			fillPx = local.Price
		}
		notional := fillPx * deltaQty
		switch local.Class {
		case OrderClassEntry:
			ladder.ApplyEntryFill(local, deltaQty, notional, local.LastUpdate)
			m.emitOrderEvent(ctx, ladder, local, "fill", "entry_fill", raw)
		case OrderClassExit, OrderClassEmergency:
			ladder.ApplyExitFill(deltaQty, fillPx, local.LastUpdate, m.cfg.TickSize)
			if local.Class == OrderClassEmergency {
				ladder.ExitReason = "emergency_flatten"
			}
			m.emitOrderEvent(ctx, ladder, local, "fill", "exit_fill", raw)
		}
	}
	if isTerminalState(remote.State) {
		switch remote.State {
		case mexcfutures.OrderStateCanceled:
			m.emitOrderEvent(ctx, ladder, local, "cancelled", local.Reason, raw)
		case mexcfutures.OrderStateInvalid:
			m.emitOrderEvent(ctx, ladder, local, "invalid", local.Reason, raw)
		}
	}
}

func (m *OrderManager) cancelByExternalID(ctx context.Context, externalOID string) error {
	_, err := m.trader.CancelOrderByExternalID(ctx, mexcfutures.CancelOrderByExternalIDRequest{
		Symbol:      m.cfg.Symbol,
		ExternalOid: externalOID,
	})
	return err
}

type apiEnvelope struct {
	Data json.RawMessage `json:"data"`
}

type remoteOrder struct {
	OrderID      string
	ExternalOID  string
	Price        float64
	Quantity     float64
	DealVol      float64
	AvgFillPrice float64
	State        int
	UpdateTime   time.Time
}

func parseSubmitOrderID(raw []byte) string {
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return ""
	}
	var obj struct {
		OrderID any `json:"orderId"`
	}
	if err := json.Unmarshal(env.Data, &obj); err == nil {
		return anyToString(obj.OrderID)
	}
	var scalar any
	if err := json.Unmarshal(env.Data, &scalar); err == nil {
		return anyToString(scalar)
	}
	return ""
}

func parseRemoteOrder(raw []byte) (remoteOrder, error) {
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return remoteOrder{}, err
	}
	var obj struct {
		OrderID         any     `json:"orderId"`
		ExternalOID     string  `json:"externalOid"`
		Price           float64 `json:"price"`
		Vol             float64 `json:"vol"`
		DealVol         float64 `json:"dealVol"`
		DealAvgPrice    float64 `json:"dealAvgPrice"`
		State           int     `json:"state"`
		UpdateTime      int64   `json:"updateTime"`
		PriceStr        string  `json:"priceStr"`
		DealAvgPriceStr string  `json:"dealAvgPriceStr"`
	}
	if err := json.Unmarshal(env.Data, &obj); err != nil {
		return remoteOrder{}, err
	}
	price := obj.Price
	if price == 0 && obj.PriceStr != "" {
		price, _ = strconv.ParseFloat(strings.TrimSpace(obj.PriceStr), 64)
	}
	avgFill := obj.DealAvgPrice
	if avgFill == 0 && obj.DealAvgPriceStr != "" {
		avgFill, _ = strconv.ParseFloat(strings.TrimSpace(obj.DealAvgPriceStr), 64)
	}
	out := remoteOrder{
		OrderID:      anyToString(obj.OrderID),
		ExternalOID:  obj.ExternalOID,
		Price:        price,
		Quantity:     obj.Vol,
		DealVol:      obj.DealVol,
		AvgFillPrice: avgFill,
		State:        obj.State,
	}
	if obj.UpdateTime > 0 {
		out.UpdateTime = time.UnixMilli(obj.UpdateTime).UTC()
	}
	return out, nil
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

func isTerminalState(state int) bool {
	return state == mexcfutures.OrderStateFilled || state == mexcfutures.OrderStateCanceled || state == mexcfutures.OrderStateInvalid
}

func entryOrderSide(side Side) int {
	if side == SideShort {
		return mexcfutures.OrderSideOpenShort
	}
	return mexcfutures.OrderSideOpenLong
}

func exitOrderSide(side Side) int {
	if side == SideShort {
		return mexcfutures.OrderSideCloseShort
	}
	return mexcfutures.OrderSideCloseLong
}

func entryReferencePrice(snapshot Snapshot, side Side) float64 {
	if side == SideShort {
		return snapshot.BestAskPx
	}
	return snapshot.BestBidPx
}

func targetExitPrice(ladder *LadderContext, snapshot Snapshot, cfg config.Scalper) float64 {
	target := ladder.AvgEntryPrice
	switch ladder.Side {
	case SideLong:
		target += float64(cfg.ProfitTargetTicks) * cfg.TickSize
		if snapshot.BestAskPx > 0 && snapshot.BestAskPx > target {
			target = snapshot.BestAskPx
		}
	case SideShort:
		target -= float64(cfg.ProfitTargetTicks) * cfg.TickSize
		if snapshot.BestBidPx > 0 && snapshot.BestBidPx < target {
			target = snapshot.BestBidPx
		}
	}
	return normalizePrice(target, cfg.TickSize)
}
