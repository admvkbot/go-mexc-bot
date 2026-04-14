package scalper

import (
	"context"
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
	if !r.book.ApplyMessage(messageJSON, symbol, channel, ingestedAt) {
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
		r.current.RecalculateExcursions(exitReferencePrice(features.Snapshot, r.current.Side), r.cfg.TickSize)
		_ = r.orders.SyncLadder(ctx, r.current)
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
		_ = r.orders.RepriceEntries(ctx, r.current, features.Snapshot, now)
	}

	decision := r.signals.Evaluate(now, features, r.current)
	r.orders.EmitSignal(ctx, decision)
	allowed, denyReason, pauseUntil := r.risk.AllowEntry(now, features, r.current)
	if !allowed && !pauseUntil.IsZero() {
		r.book.PauseVolatility(pauseUntil)
		_ = denyReason
	}
	if allowed {
		switch decision.Action {
		case DecisionEnter:
			if r.current == nil || r.current.ReadyForCleanup(now) {
				r.current = NewLadderContext(r.cfg, decision.Side, r.sessionID, now)
				r.current.LastSignalReason = decision.Reason
				if err := r.placeNewStep(ctx, decision, features.Snapshot); err != nil {
					return err
				}
			}
		case DecisionAddLadder:
			if r.current != nil && r.current.Side == decision.Side {
				if err := r.placeNewStep(ctx, decision, features.Snapshot); err != nil {
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

func (r *LiveRuntime) placeNewStep(ctx context.Context, decision Decision, snapshot Snapshot) error {
	if r.current == nil {
		return nil
	}
	price := entryReferencePrice(snapshot, decision.Side)
	if price <= 0 {
		return nil
	}
	if price*r.cfg.StepVolume+r.current.NetQuantity*price > r.cfg.MaxInventoryNotional {
		return nil
	}
	order, err := r.orders.PlaceEntry(ctx, r.current, decision.Side, r.cfg.StepVolume, price, decision.Reason)
	if err != nil {
		return err
	}
	r.current.RegisterEntryOrder(order)
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
	if !r.book.ApplyMessage(messageJSON, symbol, channel, ingestedAt) {
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
	r.orders.EmitSignal(ctx, decision)
	allowed, _, pauseUntil := r.risk.AllowEntry(now, features, r.current)
	if !allowed && !pauseUntil.IsZero() {
		r.book.PauseVolatility(pauseUntil)
		return nil
	}
	switch decision.Action {
	case DecisionEnter:
		if r.current == nil || r.current.ReadyForCleanup(now) {
			r.current = NewLadderContext(r.cfg, decision.Side, r.sessionID, now)
			r.current.LastSignalReason = decision.Reason
			r.simulateEntry(decision, features, now)
		}
	case DecisionAddLadder:
		if r.current != nil && r.current.Side == decision.Side && r.current.StepCount < r.current.MaxSteps {
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
	price := entryReferencePrice(features.Snapshot, decision.Side)
	if price <= 0 {
		return
	}
	order := &ManagedOrder{
		Class:       OrderClassEntry,
		Side:        decision.Side,
		ExternalOID: newSessionID("replay-entry"),
		Price:       price,
		Quantity:    r.cfg.StepVolume,
		FilledQty:   r.cfg.StepVolume,
		AvgFillPx:   price,
		StateCode:   3,
		SubmittedAt: now,
		LastUpdate:  now,
		Reason:      decision.Reason,
	}
	r.current.RegisterEntryOrder(order)
	r.current.ApplyEntryFill(order, order.Quantity, order.Quantity*price, now)
	r.orders.EmitReplayCandidate(context.Background(), ReplayCandidate{
		SessionID:     r.sessionID,
		Symbol:        r.cfg.Symbol,
		SignalAt:      now,
		Side:          sideToString(decision.Side),
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
		Class:       OrderClassExit,
		Side:        r.current.Side,
		ExternalOID: newSessionID("replay-exit"),
		Price:       mark,
		Quantity:    r.current.NetQuantity,
		FilledQty:   r.current.NetQuantity,
		AvgFillPx:   mark,
		StateCode:   3,
		SubmittedAt: now,
		LastUpdate:  now,
		Reason:      r.current.ExitReason,
	}
	r.current.UpsertExitOrder(exitOrder)
	r.current.ApplyExitFill(r.current.NetQuantity, mark, now, r.cfg.TickSize)
	r.current.ExitFilledAt = now
	r.orders.FlushRoundTrip(ctx, r.current)
	r.current.CooldownUntil = now.Add(r.cfg.Cooldown)
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
