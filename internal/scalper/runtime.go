package scalper

import (
	"context"
	"log"
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
)

type LiveRuntime struct {
	cfg         config.Scalper
	book        *BookState
	signals     *SignalEngine
	risk        *RiskGuard
	orders      *OrderManager
	sessionID   string
	current     *LadderContext
	lastHandled time.Time
	lastDiag    time.Time
}

func NewLiveRuntime(cfg config.Scalper, orders *OrderManager) *LiveRuntime {
	return &LiveRuntime{
		cfg:       cfg,
		book:      NewBookState(cfg),
		signals:   NewSignalEngine(cfg),
		risk:      NewRiskGuard(cfg),
		orders:    orders,
		sessionID: orders.session,
	}
}

func (r *LiveRuntime) HandleMessage(ctx context.Context, messageJSON, symbol, channel string, ingestedAt time.Time) error {
	if !r.book.ProcessMarketMessage(messageJSON, symbol, channel, ingestedAt) {
		return nil
	}
	return r.tick(ctx, ingestedAt.UTC())
}

func (r *LiveRuntime) tick(ctx context.Context, now time.Time) error {
	r.lastHandled = now
	features := r.book.Features(now)
	if !features.Snapshot.HasBook {
		return nil
	}
	if r.current != nil {
		exitMark := exitReferencePrice(features.Snapshot, r.current.Side)
		r.current.RecalculateExcursions(exitMark, r.cfg.TickSize)
		_ = r.orders.SyncLadder(ctx, r.current, exitMark, now)
		// Finish round-trip and drop ladder *before* Evaluate so we never overwrite a completed
		// ladder with NewLadderContext on the same tick (DecisionEnter + ReadyForCleanup).
		if r.current.ReadyForCleanup(now) {
			r.orders.FlushRoundTrip(ctx, r.current)
			r.current = nil
		}
	}
	if r.current != nil {
		if !r.current.HasInventory() && !hasOpenEntries(r.current) && r.current.Phase != PhaseCooldown {
			r.current.Phase = PhaseCooldown
			r.current.CooldownUntil = now.Add(r.cfg.Cooldown)
		}
		if r.current.HasInventory() {
			if flatten, reason := r.risk.ShouldFlatten(now, features, r.current); flatten {
				r.book.PauseVolatility(now.Add(r.cfg.VolatilityPause))
				if r.current.EmergencyOrder == nil || isTerminalState(r.current.EmergencyOrder.StateCode) {
					order, err := r.orders.EmergencyFlatten(ctx, r.current, exitReferencePrice(features.Snapshot, r.current.Side), reason)
					if err == nil {
						r.current.MarkEmergency(order, reason)
						r.current.ExitReason = "emergency_flatten"
					}
				}
			} else {
				_ = r.orders.EnsureExit(ctx, r.current, features.Snapshot, now)
			}
		}
	}

	decision := r.signals.Evaluate(now, features, r.current)
	allowed, denyReason, pauseUntil := r.risk.AllowEntry(now, features, r.current)
	entryLimitsCancelled := false
	if r.current != nil && !r.current.HasInventory() && hasOpenEntries(r.current) {
		if shouldCancelPendingCorridorEntries(r.cfg, r.current, now, features, decision, allowed) {
			_ = r.orders.cancelOpenEntryOrders(ctx, r.current, "entry_limits_cancelled")
			r.current.EntryWaveStartedAt = time.Time{}
			entryLimitsCancelled = true
		}
	}
	ladderID := ""
	if r.current != nil {
		ladderID = r.current.LadderID
	}
	r.orders.EmitSignal(ctx, decision, ladderID, allowed, denyReason)
	if r.cfg.DiagLog && (r.lastDiag.IsZero() || now.Sub(r.lastDiag) >= 30*time.Second) {
		r.lastDiag = now
		inv := 0.0
		if r.current != nil {
			inv = r.current.NetQuantity
		}
		log.Printf("[scalper-diag] signal=%s action=%s score=%.3f chaos=%v upd/s=%.0f max_upd=%.0f spread_ticks=%.2f max_spread=%.1f stale=%v vol_pause=%v allow_entry=%v deny=%q inv=%.4f",
			decision.Reason, decision.Action, decision.Score, features.Chaos, features.UpdateRate, r.cfg.MaxUpdateRate,
			features.SpreadTicks, r.cfg.MaxSpreadTicks, features.Stale, features.VolatilityPause, allowed, denyReason, inv)
	}
	if !allowed && !pauseUntil.IsZero() {
		r.book.PauseVolatility(pauseUntil)
		_ = denyReason
	}
	if allowed && !(entryLimitsCancelled && decision.Side == SideNone) {
		switch decision.Action {
		case DecisionEnter:
			if r.current == nil || r.current.ReadyForCleanup(now) {
				r.current = NewLadderContext(r.cfg, executionSide(r.cfg, decision.Side), r.sessionID, now)
				r.current.LastSignalReason = decision.Reason
				if err := r.placeNewStep(ctx, decision, features); err != nil {
					return err
				}
			}
		case DecisionAddLadder:
			if r.current != nil && r.current.Side == executionSide(r.cfg, decision.Side) {
				r.current.LastSignalReason = decision.Reason
				if err := r.placeNewStep(ctx, decision, features); err != nil {
					return err
				}
			}
		}
	}

	if r.current != nil && r.current.Phase == PhaseCooldown && r.current.NetQuantity == 0 {
		r.orders.FlushRoundTrip(ctx, r.current)
		if now.After(r.current.CooldownUntil) {
			r.current = nil
		}
	}
	return nil
}

func (r *LiveRuntime) placeNewStep(ctx context.Context, decision Decision, features Features) error {
	if r.current == nil {
		return nil
	}
	execSide := executionSide(r.cfg, decision.Side)
	stepVol := r.cfg.EffectiveStepVolume()
	switch decision.Action {
	case DecisionEnter:
		if hasOpenEntries(r.current) {
			return nil
		}
		if err := r.orders.PlaceCorridorEntryWave(ctx, r.current, execSide, stepVol, features, decision.Reason); err != nil {
			log.Printf("[scalper] placeNewStep PlaceCorridorEntryWave failed session=%s symbol=%s side=%s vol=%.8f err=%v",
				r.sessionID, r.cfg.Symbol, execSide, stepVol, err)
			return nil
		}
	case DecisionAddLadder:
		edge, ok := corridorEdgeLimitPrice(execSide, features)
		if !ok {
			return nil
		}
		order, err := r.orders.PlaceEntryLimit(ctx, r.current, execSide, stepVol, edge, features.Snapshot, decision.Reason)
		if err != nil {
			log.Printf("[scalper] placeNewStep PlaceEntryLimit (ladder) failed session=%s symbol=%s side=%s vol=%.8f err=%v",
				r.sessionID, r.cfg.Symbol, execSide, stepVol, err)
			return nil
		}
		r.current.RegisterEntryOrder(order)
	default:
		return nil
	}
	action := "entry_submit"
	if r.current.StepCount > 1 {
		action = "ladder_submit"
	}
	r.orders.EmitEntrySignal(ctx, decision, r.current.LadderID, action)
	return nil
}

type ReplayRuntime struct {
	cfg       config.Scalper
	book      *BookState
	signals   *SignalEngine
	risk      *RiskGuard
	orders    *OrderManager
	sessionID string
	current   *LadderContext
}

func NewReplayRuntime(cfg config.Scalper, orders *OrderManager) *ReplayRuntime {
	return &ReplayRuntime{
		cfg:       cfg,
		book:      NewBookState(cfg),
		signals:   NewSignalEngine(cfg),
		risk:      NewRiskGuard(cfg),
		orders:    orders,
		sessionID: orders.session,
	}
}

func (r *ReplayRuntime) HandleMessage(ctx context.Context, messageJSON, symbol, channel string, ingestedAt time.Time) error {
	if !r.book.ProcessMarketMessage(messageJSON, symbol, channel, ingestedAt) {
		return nil
	}
	now := ingestedAt.UTC()
	features := r.book.Features(now)
	if !features.Snapshot.HasBook {
		return nil
	}
	if r.current != nil && r.current.HasInventory() {
		r.current.RecalculateExcursions(exitReferencePrice(features.Snapshot, r.current.Side), r.cfg.TickSize)
		if r.shouldSimExit(now, features) {
			r.flushSimExit(ctx, now, features)
		}
	}
	decision := r.signals.Evaluate(now, features, r.current)
	allowed, denyReason, pauseUntil := r.risk.AllowEntry(now, features, r.current)
	ladderID := ""
	if r.current != nil {
		ladderID = r.current.LadderID
	}
	r.orders.EmitSignal(ctx, decision, ladderID, allowed, denyReason)
	if !allowed && !pauseUntil.IsZero() {
		r.book.PauseVolatility(pauseUntil)
		return nil
	}
	switch decision.Action {
	case DecisionEnter:
		if r.current == nil || r.current.ReadyForCleanup(now) {
			r.current = NewLadderContext(r.cfg, executionSide(r.cfg, decision.Side), r.sessionID, now)
			r.current.LastSignalReason = decision.Reason
			r.simulateEntry(decision, features, now)
		}
	case DecisionAddLadder:
		if r.current != nil && r.current.Side == executionSide(r.cfg, decision.Side) && r.current.StepCount < r.current.MaxSteps {
			r.simulateEntry(decision, features, now)
		}
	}
	if r.current != nil && r.current.Phase == PhaseCooldown && now.After(r.current.CooldownUntil) {
		r.current = nil
	}
	return nil
}

func (r *ReplayRuntime) simulateEntry(decision Decision, features Features, now time.Time) {
	if r.current == nil {
		return
	}
	execSide := executionSide(r.cfg, decision.Side)
	prices := corridorEntryLimitPrices(execSide, features, r.cfg.TickSize, r.cfg.MaxLadderSteps)
	if len(prices) == 0 {
		return
	}
	price := prices[len(prices)-1]
	if price <= 0 {
		return
	}
	stepVol := r.cfg.EffectiveStepVolume()
	order := &ManagedOrder{
		Class:         OrderClassEntry,
		Side:          execSide,
		ExternalOID:   newSessionID("replay-entry"),
		Price:         price,
		Quantity:      stepVol,
		FilledQty:     stepVol,
		AvgFillPx:     price,
		StateCode:     3,
		SubmittedAt:   now,
		LastUpdate:    now,
		LastRepriceAt: now,
		Reason:        decision.Reason,
	}
	r.current.RegisterEntryOrder(order)
	r.current.ApplyEntryFill(order, order.Quantity, order.Quantity*price, now)
	action := "entry_submit"
	if r.current.StepCount > 1 {
		action = "ladder_submit"
	}
	r.orders.EmitEntrySignal(context.Background(), decision, r.current.LadderID, action)
	r.orders.EmitReplayCandidate(context.Background(), ReplayCandidate{
		SessionID:     r.sessionID,
		Symbol:        r.cfg.Symbol,
		SignalAt:      now,
		Side:          sideToString(execSide),
		Score:         decision.Score,
		Reason:        decision.Reason,
		EntryPx:       price,
		TargetPx:      targetExitPrice(r.current, features.Snapshot, r.cfg),
		StopPx:        stopPrice(r.current, r.cfg),
		ExpireAt:      now.Add(r.cfg.ReplayTimeStop),
		StepCount:     r.current.StepCount,
		BestBidPx:     features.Snapshot.BestBidPx,
		BestAskPx:     features.Snapshot.BestAskPx,
		Imbalance5:    features.Snapshot.Imbalance5,
		PressureDelta: features.PressureDelta,
	})
}

func (r *ReplayRuntime) shouldSimExit(now time.Time, features Features) bool {
	if r.current == nil || !r.current.HasInventory() {
		return false
	}
	mark := exitReferencePrice(features.Snapshot, r.current.Side)
	if mark == 0 {
		return false
	}
	if now.Sub(r.current.EntryStartedAt) >= r.cfg.ReplayTimeStop {
		r.current.ExitReason = "replay_time_stop"
		return true
	}
	if pnlTicks(r.current.AvgEntryPrice, mark, r.cfg.TickSize, r.current.Side) >= float64(r.cfg.ReplayTargetTicks) {
		r.current.ExitReason = "replay_target"
		return true
	}
	if -pnlTicks(r.current.AvgEntryPrice, mark, r.cfg.TickSize, r.current.Side) >= float64(r.cfg.ReplayStopTicks) {
		r.current.ExitReason = "replay_stop"
		return true
	}
	return false
}

func (r *ReplayRuntime) flushSimExit(ctx context.Context, now time.Time, features Features) {
	if r.current == nil {
		return
	}
	mark := exitReferencePrice(features.Snapshot, r.current.Side)
	exitOrder := &ManagedOrder{
		Class:         OrderClassExit,
		Side:          r.current.Side,
		ExternalOID:   newSessionID("replay-exit"),
		Price:         mark,
		Quantity:      r.current.NetQuantity,
		FilledQty:     r.current.NetQuantity,
		AvgFillPx:     mark,
		StateCode:     3,
		SubmittedAt:   now,
		LastUpdate:    now,
		LastRepriceAt: now,
		Reason:        r.current.ExitReason,
	}
	r.current.UpsertExitOrder(exitOrder)
	r.current.ApplyExitFill(r.current.NetQuantity, mark, now, r.cfg.TickSize)
	r.current.ExitFilledAt = now
	r.orders.FlushRoundTrip(ctx, r.current)
	r.current.CooldownUntil = now.Add(r.cfg.Cooldown)
}

func shouldCancelPendingCorridorEntries(cfg config.Scalper, ladder *LadderContext, now time.Time, f Features, d Decision, allowed bool) bool {
	if !allowed {
		return true
	}
	if d.Side == SideNone {
		return true
	}
	if executionSide(cfg, d.Side) != ladder.Side {
		return true
	}
	if cfg.PriceCorridorWindow > 0 && !f.HasPriceCorridor {
		return true
	}
	if ttl := cfg.EntryLimitPendingTTL; ttl > 0 && !ladder.EntryWaveStartedAt.IsZero() && now.Sub(ladder.EntryWaveStartedAt) >= ttl {
		return true
	}
	if ladder.Side == SideLong && f.HasPriceCorridor && f.Snapshot.Mid > f.PriceUpperBound {
		return true
	}
	if ladder.Side == SideShort && f.HasPriceCorridor && f.Snapshot.Mid < f.PriceLowerBound {
		return true
	}
	return false
}

func stopPrice(ladder *LadderContext, cfg config.Scalper) float64 {
	if ladder == nil {
		return 0
	}
	switch ladder.Side {
	case SideLong:
		return ladder.AvgEntryPrice - float64(cfg.ReplayStopTicks)*cfg.TickSize
	case SideShort:
		return ladder.AvgEntryPrice + float64(cfg.ReplayStopTicks)*cfg.TickSize
	default:
		return 0
	}
}

func hasOpenEntries(ladder *LadderContext) bool {
	if ladder == nil {
		return false
	}
	for _, ord := range ladder.EntryOrders {
		if ord != nil && !isTerminalState(ord.StateCode) {
			return true
		}
	}
	return false
}
