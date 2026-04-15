package scalper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
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

func (m *OrderManager) EmitSignal(ctx context.Context, decision Decision, ladderID string, allowEntry bool, denyReason string) {
	m.emitSignalEvent(ctx, decision, string(decision.Action), ladderID, allowEntry, denyReason)
}

func (m *OrderManager) EmitEntrySignal(ctx context.Context, decision Decision, ladderID, action string) {
	m.emitSignalEvent(ctx, decision, action, ladderID, true, "")
}

func (m *OrderManager) emitSignalEvent(ctx context.Context, decision Decision, action, ladderID string, allowEntry bool, denyReason string) {
	if m == nil || m.journal == nil {
		return
	}
	s := decision.Features.Snapshot
	_ = m.journal.InsertScalperSignalEvents(ctx, []SignalEvent{{
		SessionID:      m.session,
		LadderID:       ladderID,
		Mode:           m.modeName,
		Symbol:         s.Symbol,
		EventAt:        s.At,
		Action:         action,
		Side:           sideToString(decision.Side),
		Score:          decision.Score,
		Reason:         decision.Reason,
		AllowEntry:     allowEntry,
		DenyReason:     denyReason,
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
		ConfirmCount:   decision.Features.SignalConfirmCount,
		ConfirmMS:      decision.Features.SignalConfirmAge.Milliseconds(),
		MaxSpreadTicks: decision.Features.MaxRecentSpreadTicks,
	}})
}

func (m *OrderManager) EmitReplayCandidate(ctx context.Context, row ReplayCandidate) {
	if m == nil || m.journal == nil {
		return
	}
	_ = m.journal.InsertScalperReplayCandidates(ctx, []ReplayCandidate{row})
}

func orderPriceDecimalsFromCfg(cfg config.Scalper) int {
	if cfg.OrderPriceScale != nil {
		return *cfg.OrderPriceScale
	}
	return tickDecimalPlaces(cfg.TickSize)
}

func orderVolScaleFromCfg(cfg config.Scalper) int {
	if cfg.OrderVolScale != nil {
		return *cfg.OrderVolScale
	}
	return -1
}

func (m *OrderManager) PlaceEntry(ctx context.Context, ladder *LadderContext, side Side, quantity, price float64, reason string) (*ManagedOrder, error) {
	priceOut := quantizeOrderPrice(price, m.cfg.TickSize, orderPriceDecimalsFromCfg(m.cfg))
	volOut := quantizeOrderVol(quantity, orderVolScaleFromCfg(m.cfg))
	req := mexcfutures.SubmitOrderRequest{
		Symbol:        m.cfg.Symbol,
		Price:         priceOut,
		Vol:           volOut,
		Side:          entryOrderSide(side),
		Type:          mexcfutures.OrderTypeMarket,
		OpenType:      m.cfg.OpenType,
		Leverage:      m.cfg.Leverage,
		ExternalOid:   newSessionID("entry"),
		PositionMode:  m.cfg.PositionMode,
		STPMode:       0,
		MarketCeiling: true,
	}
	if m.cfg.ExitUsesExchangeBracket() {
		sl, tp, berr := bracketSLTPQuantized(side, priceOut, m.cfg.TickSize, m.cfg.ProfitTargetTicks, m.cfg.StopLossTicks, orderPriceDecimalsFromCfg(m.cfg))
		if berr != nil {
			return nil, fmt.Errorf("scalper: bracket SL/TP: %w", berr)
		}
		req.StopLossPrice = sl
		req.TakeProfitPrice = tp
		log.Printf("[trade] entry with exchange bracket symbol=%s side=%s entry_px=%.8f sl=%.8f tp=%.8f ticks_stop=%d ticks_tp=%d",
			m.cfg.Symbol, sideToString(side), priceOut, sl, tp, m.cfg.StopLossTicks, m.cfg.ProfitTargetTicks)
	}
	raw, err := m.trader.SubmitOrder(ctx, req)
	t := time.Now().UTC()
	order := &ManagedOrder{
		Class:         OrderClassEntry,
		Side:          side,
		ExternalOID:   req.ExternalOid,
		Price:         req.Price,
		Quantity:      volOut,
		SubmittedAt:   t,
		LastUpdate:    t,
		LastRepriceAt: t,
		Reason:        reason,
	}
	if err != nil {
		m.emitOrderEvent(ctx, ladder, order, "submit_error", reason, "")
		return nil, err
	}
	rawTrim := bytes.TrimPrefix(bytes.TrimSpace(raw), []byte{0xEF, 0xBB, 0xBF})
	if apiErr := futuresSubmitAPIError(rawTrim); apiErr != nil {
		log.Printf("[trade] submit REJECTED kind=entry symbol=%s ext_oid=%s side=%s vol=%.8f @ %.8f ladder_reason=%q: %v",
			m.cfg.Symbol, order.ExternalOID, sideToString(side), volOut, order.Price, reason, apiErr)
		m.emitOrderEvent(ctx, ladder, order, "submit_error", fmt.Sprintf("%s: %v", reason, apiErr), string(rawTrim))
		return nil, apiErr
	}
	order.OrderID = parseSubmitOrderID(rawTrim)
	logTradeAPIResponse("submit_entry", order.ExternalOID, order.OrderID, rawTrim)
	m.emitOrderEvent(ctx, ladder, order, "submit", reason, string(rawTrim))
	return order, nil
}

func (m *OrderManager) PlaceExit(ctx context.Context, ladder *LadderContext, price float64, reason string) (*ManagedOrder, error) {
	if ladder == nil || ladder.NetQuantity <= 0 {
		return nil, fmt.Errorf("scalper: exit requires inventory")
	}
	volOut := quantizeOrderVol(ladder.NetQuantity, orderVolScaleFromCfg(m.cfg))
	req := mexcfutures.SubmitOrderRequest{
		Symbol:       m.cfg.Symbol,
		Price:        quantizeOrderPrice(price, m.cfg.TickSize, orderPriceDecimalsFromCfg(m.cfg)),
		Vol:          volOut,
		Side:         exitOrderSide(ladder.Side),
		Type:         mexcfutures.OrderTypeLimit,
		OpenType:     m.cfg.OpenType,
		ExternalOid:  newSessionID("exit"),
		PositionMode: m.cfg.PositionMode,
		ReduceOnly:   m.cfg.PositionMode == mexcfutures.PositionModeOneWay,
		STPMode:      0,
	}
	raw, err := m.trader.SubmitOrder(ctx, req)
	t := time.Now().UTC()
	order := &ManagedOrder{
		Class:         OrderClassExit,
		Side:          ladder.Side,
		ExternalOID:   req.ExternalOid,
		Price:         req.Price,
		Quantity:      volOut,
		SubmittedAt:   t,
		LastUpdate:    t,
		LastRepriceAt: t,
		Reason:        reason,
		ReduceOnly:    req.ReduceOnly,
	}
	if err != nil {
		m.emitOrderEvent(ctx, ladder, order, "submit_error", reason, "")
		return nil, err
	}
	rawTrim := bytes.TrimPrefix(bytes.TrimSpace(raw), []byte{0xEF, 0xBB, 0xBF})
	if apiErr := futuresSubmitAPIError(rawTrim); apiErr != nil {
		log.Printf("[trade] submit REJECTED kind=exit symbol=%s ext_oid=%s side=%s vol=%.8f @ %.8f ladder_reason=%q: %v",
			m.cfg.Symbol, order.ExternalOID, sideToString(order.Side), volOut, order.Price, reason, apiErr)
		m.emitOrderEvent(ctx, ladder, order, "submit_error", fmt.Sprintf("%s: %v", reason, apiErr), string(rawTrim))
		return nil, apiErr
	}
	order.OrderID = parseSubmitOrderID(rawTrim)
	logTradeAPIResponse("submit_exit", order.ExternalOID, order.OrderID, rawTrim)
	m.emitOrderEvent(ctx, ladder, order, "submit", reason, string(rawTrim))
	return order, nil
}

func (m *OrderManager) EmergencyFlatten(ctx context.Context, ladder *LadderContext, priceHint float64, reason string) (*ManagedOrder, error) {
	if ladder == nil || ladder.NetQuantity <= 0 {
		return nil, nil
	}
	if _, cerr := m.trader.CancelAllOrders(ctx, mexcfutures.CancelAllOrdersRequest{Symbol: m.cfg.Symbol}); cerr != nil {
		log.Printf("[trade] cancel_all_orders symbol=%s err=%v", m.cfg.Symbol, cerr)
	} else {
		log.Printf("[trade] cancel_all_orders ok symbol=%s", m.cfg.Symbol)
	}
	if !m.cfg.AllowEmergencyMarket {
		return nil, fmt.Errorf("scalper: emergency market disabled")
	}
	volOut := quantizeOrderVol(ladder.NetQuantity, orderVolScaleFromCfg(m.cfg))
	req := mexcfutures.SubmitOrderRequest{
		Symbol:        m.cfg.Symbol,
		Price:         quantizeOrderPrice(priceHint, m.cfg.TickSize, orderPriceDecimalsFromCfg(m.cfg)),
		Vol:           volOut,
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
	t := time.Now().UTC()
	order := &ManagedOrder{
		Class:         OrderClassEmergency,
		Side:          ladder.Side,
		ExternalOID:   req.ExternalOid,
		Price:         req.Price,
		Quantity:      volOut,
		SubmittedAt:   t,
		LastUpdate:    t,
		LastRepriceAt: t,
		Reason:        reason,
	}
	if err != nil {
		m.emitOrderEvent(ctx, ladder, order, "flatten_error", reason, "")
		return nil, err
	}
	rawTrim := bytes.TrimPrefix(bytes.TrimSpace(raw), []byte{0xEF, 0xBB, 0xBF})
	if apiErr := futuresSubmitAPIError(rawTrim); apiErr != nil {
		log.Printf("[trade] submit REJECTED kind=flatten symbol=%s ext_oid=%s side=%s vol=%.8f @ %.8f ladder_reason=%q: %v",
			m.cfg.Symbol, order.ExternalOID, sideToString(order.Side), volOut, order.Price, reason, apiErr)
		m.emitOrderEvent(ctx, ladder, order, "flatten_error", fmt.Sprintf("%s: %v", reason, apiErr), string(rawTrim))
		return nil, apiErr
	}
	order.OrderID = parseSubmitOrderID(rawTrim)
	logTradeAPIResponse("submit_flatten", order.ExternalOID, order.OrderID, rawTrim)
	m.emitOrderEvent(ctx, ladder, order, "flatten_submit", reason, string(rawTrim))
	return order, nil
}

func logTradeAPIResponse(kind, extOID, orderID string, raw []byte) {
	body := string(raw)
	const maxLog = 65536
	if len(body) > maxLog {
		body = body[:maxLog] + "... (truncated)"
	}
	log.Printf("[trade] api_response kind=%s ext_oid=%s order_id=%s body=%s", kind, extOID, orderID, body)
}

// futuresSubmitAPIError returns a non-nil error when MEXC returns HTTP 200 with JSON body success=false.
// If "success" is absent or true, returns nil (including unparseable JSON — handled elsewhere).
func futuresSubmitAPIError(raw []byte) error {
	raw = bytes.TrimPrefix(bytes.TrimSpace(raw), []byte{0xEF, 0xBB, 0xBF})
	var meta struct {
		Success *bool           `json:"success"`
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Extend  json.RawMessage `json:"_extend"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil
	}
	if meta.Success == nil || *meta.Success {
		return nil
	}
	ext := strings.TrimSpace(string(meta.Extend))
	const maxExt = 600
	if len(ext) > maxExt {
		ext = ext[:maxExt] + "..."
	}
	msg := strings.TrimSpace(meta.Message)
	if msg != "" {
		return fmt.Errorf("mexc success=false code=%d message=%q _extend=%s", meta.Code, msg, ext)
	}
	return fmt.Errorf("mexc success=false code=%d _extend=%s", meta.Code, ext)
}

func (m *OrderManager) SyncLadder(ctx context.Context, ladder *LadderContext, exitMarkPx float64, now time.Time) error {
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
	if m.cfg.ExitUsesExchangeBracket() && ladder.HasInventory() && m.trader != nil {
		if ladder.LastPositionSyncAt.IsZero() || now.Sub(ladder.LastPositionSyncAt) >= m.cfg.PollInterval {
			ladder.LastPositionSyncAt = now
			if err := m.syncExchangeBracketClose(ctx, ladder, exitMarkPx, now); err != nil {
				log.Printf("[trade] bracket position sync symbol=%s err=%v", m.cfg.Symbol, err)
			}
		}
	}
	return nil
}

func (m *OrderManager) EnsureExit(ctx context.Context, ladder *LadderContext, snapshot Snapshot, now time.Time) error {
	if ladder == nil || ladder.NetQuantity <= 0 {
		return nil
	}
	if m.cfg.ExitUsesExchangeBracket() {
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
	if now.Sub(ladder.ExitOrder.SubmittedAt) >= m.cfg.ExitTTL && now.Sub(ladder.ExitOrder.LastRepriceAt) >= m.cfg.RepriceInterval {
		_ = m.cancelByExternalID(ctx, ladder.ExitOrder.ExternalOID)
		exitOrder, err := m.PlaceExit(ctx, ladder, target, "exit_reprice")
		if err != nil {
			return err
		}
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
	if m == nil || order == nil {
		return
	}
	ladderID := ""
	if ladder != nil {
		ladderID = ladder.LadderID
	}
	log.Printf("[trade] %s symbol=%s class=%s side=%s ext_oid=%s order_id=%s vol=%.8f fill=%.8f avg_px=%.6f state=%d reason=%s ladder=%s",
		eventType, m.cfg.Symbol, order.Class, sideToString(order.Side), order.ExternalOID, order.OrderID,
		order.Quantity, order.FilledQty, order.AvgFillPx, order.StateCode, reason, ladderID)
	if m.journal == nil {
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
	row.LadderID = ladderID
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
	if err != nil {
		log.Printf("[trade] cancel_by_ext_oid symbol=%s ext_oid=%s err=%v", m.cfg.Symbol, externalOID, err)
		return err
	}
	log.Printf("[trade] cancel_by_ext_oid ok symbol=%s ext_oid=%s", m.cfg.Symbol, externalOID)
	return nil
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

var submitOrderIDKeys = []string{"orderId", "order_id", "orderID"}

func parseSubmitOrderID(raw []byte) string {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return ""
	}
	if s := pickOrderIDFromMap(top); s != "" {
		return s
	}
	data, ok := top["data"]
	if !ok || len(bytes.TrimSpace(data)) == 0 {
		return ""
	}
	data = unwrapJSONStringData(data)
	if s := pickOrderIDFromMapRawObject(data); s != "" {
		return s
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil && len(arr) > 0 {
		if s := pickOrderIDFromMapRawObject(arr[0]); s != "" {
			return s
		}
	}
	if s := rawJSONToOrderID(data); s != "" {
		return s
	}
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(data, &inner); err != nil {
		return ""
	}
	return pickOrderIDFromMap(inner)
}

func pickOrderIDFromMap(m map[string]json.RawMessage) string {
	for _, key := range submitOrderIDKeys {
		if v, ok := m[key]; ok {
			if s := rawJSONToOrderID(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func pickOrderIDFromMapRawObject(obj json.RawMessage) string {
	var inner map[string]json.RawMessage
	if err := json.Unmarshal(obj, &inner); err != nil {
		return ""
	}
	return pickOrderIDFromMap(inner)
}

func unwrapJSONStringData(data json.RawMessage) json.RawMessage {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return data
	}
	st := strings.TrimSpace(s)
	if strings.HasPrefix(st, "{") || strings.HasPrefix(st, "[") {
		return []byte(st)
	}
	return data
}

func rawJSONToOrderID(m json.RawMessage) string {
	m = bytes.TrimSpace(m)
	if len(m) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if err := json.Unmarshal(m, &n); err == nil {
		return strings.TrimSpace(n.String())
	}
	var i int64
	if err := json.Unmarshal(m, &i); err == nil {
		return strconv.FormatInt(i, 10)
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

// syncExchangeBracketClose detects exchange-side bracket fills via open_positions and mirrors them locally.
func (m *OrderManager) syncExchangeBracketClose(ctx context.Context, ladder *LadderContext, exitMarkPx float64, now time.Time) error {
	if ladder == nil || ladder.NetQuantity <= 0 || m.trader == nil || !m.cfg.ExitUsesExchangeBracket() {
		return nil
	}
	sym := m.cfg.Symbol
	rows, err := m.fetchPositionRows(ctx, sym)
	if err != nil {
		return err
	}
	holdVol := positionHoldVolForSide(rows, sym, ladder.Side)
	if holdVol >= ladder.NetQuantity-1e-6 {
		return nil
	}
	remaining := holdVol
	closedQty := ladder.NetQuantity - remaining
	if closedQty <= 1e-6 {
		return nil
	}
	if remaining <= 1e-6 {
		// One retry: transient empty open_positions right after a fill can false-trigger.
		time.Sleep(80 * time.Millisecond)
		rows2, err2 := m.fetchPositionRows(ctx, sym)
		if err2 != nil {
			return err2
		}
		remaining = positionHoldVolForSide(rows2, sym, ladder.Side)
		closedQty = ladder.NetQuantity - remaining
		if closedQty <= 1e-6 {
			return nil
		}
	}
	fillPx, reason := inferBracketExitFill(ladder.Side, ladder.AvgEntryPrice, exitMarkPx, m.cfg.TickSize, m.cfg.ProfitTargetTicks, m.cfg.StopLossTicks)
	exitOrder := m.ensureSyntheticBracketExitOrder(ladder, now)
	closedTotal := exitOrder.FilledQty + closedQty
	if closedTotal <= 0 {
		closedTotal = closedQty
	}
	if exitOrder.AvgFillPx > 0 && exitOrder.FilledQty > 0 {
		exitOrder.AvgFillPx = ((exitOrder.AvgFillPx * exitOrder.FilledQty) + (fillPx * closedQty)) / closedTotal
	} else {
		exitOrder.AvgFillPx = fillPx
	}
	exitOrder.Price = fillPx
	exitOrder.Quantity = closedTotal + remaining
	exitOrder.FilledQty = closedTotal
	exitOrder.LastUpdate = now
	exitOrder.LastRepriceAt = now
	exitOrder.Reason = reason
	if remaining <= 1e-6 {
		exitOrder.StateCode = mexcfutures.OrderStateFilled
	} else {
		exitOrder.StateCode = mexcfutures.OrderStateUnfilled
	}
	ladder.ExitReason = reason
	m.emitOrderEvent(ctx, ladder, exitOrder, "fill", reason, "")
	ladder.ApplyExitFill(closedQty, fillPx, now, m.cfg.TickSize)
	if remaining <= 1e-6 {
		if err := m.cancelOpenEntryOrders(ctx, ladder); err != nil {
			log.Printf("[trade] cancel_open_entries_after_bracket_close symbol=%s ladder=%s err=%v", m.cfg.Symbol, ladder.LadderID, err)
		}
	}
	return nil
}

func (m *OrderManager) fetchPositionRows(ctx context.Context, symbol string) ([]mexcfutures.OpenPositionClose, error) {
	raw, err := m.trader.OpenPositionsFutures(ctx, &symbol)
	if err != nil {
		return nil, err
	}
	return mexcfutures.ParseOpenPositionsResponse(raw)
}

func (m *OrderManager) fetchPositionHoldVol(ctx context.Context, symbol string, side Side) (float64, error) {
	rows, err := m.fetchPositionRows(ctx, symbol)
	if err != nil {
		return 0, err
	}
	return positionHoldVolForSide(rows, symbol, side), nil
}

func positionHoldVolForSide(rows []mexcfutures.OpenPositionClose, symbol string, side Side) float64 {
	var v float64
	for i := range rows {
		r := rows[i]
		if r.Symbol != symbol {
			continue
		}
		switch side {
		case SideLong:
			if r.IsLong {
				v += r.HoldVol
			}
		case SideShort:
			if !r.IsLong {
				v += r.HoldVol
			}
		}
	}
	return v
}

func (m *OrderManager) ensureSyntheticBracketExitOrder(ladder *LadderContext, now time.Time) *ManagedOrder {
	if ladder.ExitOrder != nil {
		return ladder.ExitOrder
	}
	exitOrder := &ManagedOrder{
		Class:         OrderClassExit,
		Side:          ladder.Side,
		ExternalOID:   newSessionID("bracket-exit"),
		StateCode:     mexcfutures.OrderStatePending,
		SubmittedAt:   now,
		LastUpdate:    now,
		LastRepriceAt: now,
	}
	ladder.UpsertExitOrder(exitOrder)
	return exitOrder
}

func (m *OrderManager) cancelOpenEntryOrders(ctx context.Context, ladder *LadderContext) error {
	if ladder == nil {
		return nil
	}
	var firstErr error
	for _, order := range ladder.EntryOrders {
		if order == nil || isTerminalState(order.StateCode) {
			continue
		}
		if err := m.cancelByExternalID(ctx, order.ExternalOID); err != nil && firstErr == nil {
			firstErr = err
			continue
		}
		order.StateCode = mexcfutures.OrderStateCanceled
		order.LastUpdate = time.Now().UTC()
		m.emitOrderEvent(ctx, ladder, order, "cancelled", "entry_cancel_after_bracket_close", "")
	}
	return firstErr
}

func bracketSLTPQuantized(side Side, entryPx, tickSize float64, profitTicks, stopTicks, priceDecimals int) (sl, tp float64, err error) {
	if profitTicks <= 0 || stopTicks <= 0 {
		return 0, 0, fmt.Errorf("profit_ticks and stop_ticks must be > 0 (got profit=%d stop=%d)", profitTicks, stopTicks)
	}
	var slRaw, tpRaw float64
	switch side {
	case SideLong:
		slRaw = entryPx - float64(stopTicks)*tickSize
		tpRaw = entryPx + float64(profitTicks)*tickSize
	case SideShort:
		slRaw = entryPx + float64(stopTicks)*tickSize
		tpRaw = entryPx - float64(profitTicks)*tickSize
	default:
		return 0, 0, fmt.Errorf("unknown side %q", side)
	}
	sl = quantizeOrderPrice(slRaw, tickSize, priceDecimals)
	tp = quantizeOrderPrice(tpRaw, tickSize, priceDecimals)
	switch side {
	case SideLong:
		if !(sl < entryPx && tp > entryPx) {
			return 0, 0, fmt.Errorf("long bracket invalid after quantize entry=%.8f sl=%.8f tp=%.8f", entryPx, sl, tp)
		}
	case SideShort:
		if !(sl > entryPx && tp < entryPx) {
			return 0, 0, fmt.Errorf("short bracket invalid after quantize entry=%.8f sl=%.8f tp=%.8f", entryPx, sl, tp)
		}
	}
	return sl, tp, nil
}

func inferBracketExitFill(side Side, avgEntry, exitMark, tickSize float64, profitTicks, stopTicks int) (float64, string) {
	reason := inferBracketExitReason(side, avgEntry, exitMark, tickSize, profitTicks, stopTicks)
	tpPx, slPx := bracketExitPrices(side, avgEntry, tickSize, profitTicks, stopTicks)
	switch reason {
	case "take_profit_ticks":
		return tpPx, reason
	case "stop_loss_ticks":
		return slPx, reason
	default:
		if exitMark > 0 {
			return exitMark, reason
		}
		return avgEntry, reason
	}
}

func bracketExitPrices(side Side, avgEntry, tickSize float64, profitTicks, stopTicks int) (tpPx, slPx float64) {
	tpPx = avgEntry + float64(profitTicks)*tickSize
	slPx = avgEntry - float64(stopTicks)*tickSize
	if side == SideShort {
		tpPx = avgEntry - float64(profitTicks)*tickSize
		slPx = avgEntry + float64(stopTicks)*tickSize
	}
	return tpPx, slPx
}

func inferBracketExitReason(side Side, avgEntry, exitMark, tickSize float64, profitTicks, stopTicks int) string {
	if avgEntry <= 0 || tickSize <= 0 {
		return "bracket_exit"
	}
	tol := 0.75 * tickSize
	tpPx, slPx := bracketExitPrices(side, avgEntry, tickSize, profitTicks, stopTicks)
	if side == SideLong {
		if exitMark >= tpPx-tol {
			return "take_profit_ticks"
		}
		if exitMark <= slPx+tol {
			return "stop_loss_ticks"
		}
	} else if side == SideShort {
		if exitMark <= tpPx+tol {
			return "take_profit_ticks"
		}
		if exitMark >= slPx-tol {
			return "stop_loss_ticks"
		}
	}
	return "bracket_exit"
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
	return quantizeOrderPrice(target, cfg.TickSize, orderPriceDecimalsFromCfg(cfg))
}
