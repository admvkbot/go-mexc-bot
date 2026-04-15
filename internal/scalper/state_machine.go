package scalper

import (
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
)

func NewLadderContext(cfg config.Scalper, side Side, sessionID string, now time.Time) *LadderContext {
	return &LadderContext{
		SessionID:      sessionID,
		LadderID:       newSessionID("ladder"),
		Symbol:         cfg.Symbol,
		Side:           side,
		Phase:          PhaseEntryPending,
		MaxSteps:       cfg.MaxLadderSteps,
		EntryStartedAt: now.UTC(),
		LastStepAt:     now.UTC(),
		LastDecisionAt: now.UTC(),
	}
}

func (c *LadderContext) RegisterEntryOrder(order *ManagedOrder) {
	if c == nil || order == nil {
		return
	}
	c.EntryOrders = append(c.EntryOrders, order)
	c.StepCount++
	c.LastStepAt = order.SubmittedAt
	c.Phase = PhaseEntryPending
}

// RegisterEntryWave добавляет несколько лимитов одной волной входа по коридору (один шаг StepCount).
func (c *LadderContext) RegisterEntryWave(orders []*ManagedOrder) {
	if c == nil || len(orders) == 0 {
		return
	}
	for _, o := range orders {
		if o != nil {
			c.EntryOrders = append(c.EntryOrders, o)
		}
	}
	c.StepCount++
	if last := orders[len(orders)-1]; last != nil {
		c.LastStepAt = last.SubmittedAt
	}
	c.Phase = PhaseEntryPending
}

func (c *LadderContext) UpsertExitOrder(order *ManagedOrder) {
	if c == nil || order == nil {
		return
	}
	c.ExitOrder = order
	c.ExitStartedAt = order.SubmittedAt
	c.Phase = PhaseExitPending
}

func (c *LadderContext) MarkEmergency(order *ManagedOrder, reason string) {
	if c == nil {
		return
	}
	c.EmergencyOrder = order
	c.WasEmergencyExit = true
	c.FlattenReason = reason
	c.Phase = PhaseEmergencyFlatten
}

func (c *LadderContext) ApplyEntryFill(order *ManagedOrder, deltaQty, deltaNotional float64, at time.Time) {
	if c == nil || deltaQty <= 0 {
		return
	}
	totalNotional := c.AvgEntryPrice*c.NetQuantity + deltaNotional
	c.NetQuantity += deltaQty
	if c.NetQuantity > 0 {
		c.AvgEntryPrice = totalNotional / c.NetQuantity
	}
	if c.EntryFilledAt.IsZero() {
		c.EntryFilledAt = at.UTC()
	}
	if c.NetQuantity > 0 {
		c.Phase = PhaseInventoryOpen
	}
	if order != nil {
		c.LastDecisionAt = order.LastUpdate
	}
}

func (c *LadderContext) ApplyExitFill(deltaQty, exitPx float64, at time.Time, tickSize float64) {
	if c == nil || deltaQty <= 0 {
		return
	}
	qty := minFloat(deltaQty, c.NetQuantity)
	c.RealizedPnL += grossPnL(c.AvgEntryPrice, exitPx, qty, c.Side)
	c.NetQuantity -= qty
	if c.NetQuantity < 1e-9 {
		c.NetQuantity = 0
		c.ExitFilledAt = at.UTC()
		c.Phase = PhaseCooldown
		c.CooldownUntil = at.Add(500 * time.Millisecond).UTC()
	}
	c.RecalculateExcursions(exitPx, tickSize)
}

func (c *LadderContext) RecalculateExcursions(markPx, tickSize float64) {
	if c == nil || c.AvgEntryPrice == 0 || markPx == 0 {
		return
	}
	pnl := pnlTicks(c.AvgEntryPrice, markPx, tickSize, c.Side)
	if pnl > c.MaxFavorableTicks {
		c.MaxFavorableTicks = pnl
	}
	if -pnl > c.MaxAdverseTicks {
		c.MaxAdverseTicks = -pnl
	}
}

func (c *LadderContext) ReadyForCleanup(now time.Time) bool {
	if c == nil {
		return true
	}
	if c.Phase == PhaseIdle {
		return true
	}
	return c.Phase == PhaseCooldown && now.After(c.CooldownUntil)
}

func (c *LadderContext) BuildRoundTrip(tickSize float64) RoundTrip {
	rt := RoundTrip{
		SessionID:           c.SessionID,
		LadderID:            c.LadderID,
		Symbol:              c.Symbol,
		Side:                sideToString(c.Side),
		EntryStartedAt:      c.EntryStartedAt,
		EntryFilledAt:       c.EntryFilledAt,
		ExitStartedAt:       c.ExitStartedAt,
		ExitFilledAt:        c.ExitFilledAt,
		EntryAvgPx:          c.AvgEntryPrice,
		ExitAvgPx:           exitAveragePrice(c),
		FilledQty:           0,
		GrossPnL:            c.RealizedPnL,
		PNLTicks:            pnlTicks(c.AvgEntryPrice, exitAveragePrice(c), tickSize, c.Side),
		MaxAdverseTicks:     c.MaxAdverseTicks,
		MaxFavorableTicks:   c.MaxFavorableTicks,
		ExitReason:          c.ExitReason,
		FlattenReason:       c.FlattenReason,
		RepricesCount:       c.RepricesCount,
		LadderStepsUsed:     c.StepCount,
		WasEmergencyFlatten: c.WasEmergencyExit,
	}
	if !c.EntryStartedAt.IsZero() && !c.ExitFilledAt.IsZero() {
		rt.HoldingMS = c.ExitFilledAt.Sub(c.EntryStartedAt).Milliseconds()
	}
	for _, ord := range c.EntryOrders {
		if ord != nil {
			rt.FilledQty += ord.FilledQty
		}
	}
	return rt
}

func exitAveragePrice(c *LadderContext) float64 {
	if c == nil {
		return 0
	}
	if c.ExitOrder != nil && c.ExitOrder.AvgFillPx > 0 {
		return c.ExitOrder.AvgFillPx
	}
	if c.EmergencyOrder != nil && c.EmergencyOrder.AvgFillPx > 0 {
		return c.EmergencyOrder.AvgFillPx
	}
	return 0
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
