package scalper

import (
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
)

// SignalEngine evaluates book-only entry and ladder-extension opportunities.
type SignalEngine struct {
	cfg config.Scalper
}

func NewSignalEngine(cfg config.Scalper) *SignalEngine {
	return &SignalEngine{cfg: cfg}
}

// WarmupScores returns raw long/short scores (same components as Evaluate before MinSignalScore gate).
func (e *SignalEngine) WarmupScores(f Features) (longScore, shortScore float64) {
	return e.longScore(f), e.shortScore(f)
}

func (e *SignalEngine) Evaluate(now time.Time, features Features, ladder *LadderContext) Decision {
	decision := Decision{
		Action:      DecisionHold,
		Side:        SideNone,
		Reason:      "no_signal",
		TargetTicks: e.cfg.ProfitTargetTicks,
		StopTicks:   e.cfg.StopLossTicks,
		Features:    features,
	}
	if !features.Snapshot.HasBook {
		decision.Reason = "book_unavailable"
		return decision
	}
	if features.VolatilityPause {
		decision.Reason = "volatility_pause"
		return decision
	}
	if features.Stale {
		decision.Reason = "stale_book"
		return decision
	}
	if features.WideSpread {
		decision.Reason = "spread_too_wide"
		return decision
	}
	if features.Chaos {
		decision.Reason = "book_churn_too_high"
		return decision
	}

	longScore := e.longScore(features)
	shortScore := e.shortScore(features)
	if longScore >= e.cfg.MinSignalScore && longScore > shortScore {
		decision.Score = longScore
		decision.Side = SideLong
		decision.Reason = "book_pressure_long"
	} else if shortScore >= e.cfg.MinSignalScore && shortScore > longScore {
		decision.Score = shortScore
		decision.Side = SideShort
		decision.Reason = "book_pressure_short"
	} else {
		decision.Reason = "score_below_threshold"
		return decision
	}
	if rejectReason := e.rejectEntrySignal(decision.Side, features); rejectReason != "" {
		decision.Action = DecisionHold
		decision.Side = SideNone
		decision.Score = 0
		decision.Reason = rejectReason
		return decision
	}

	// New trading cycle only when there is no active ladder, or cooldown has fully elapsed.
	// Do NOT use now.After(CooldownUntil) alone: CooldownUntil zero-value is year 1 → would always
	// short-circuit to DecisionEnter and never reach DecisionAddLadder / manage exit paths.
	canFreshEnter := ladder == nil || ladder.Phase == PhaseIdle ||
		(ladder.Phase == PhaseCooldown && !ladder.CooldownUntil.IsZero() && now.After(ladder.CooldownUntil))
	if canFreshEnter {
		decision.Action = DecisionEnter
		return decision
	}
	if ladder.Side == executionSide(e.cfg, decision.Side) && ladder.StepCount < ladder.MaxSteps && now.Sub(ladder.LastStepAt) >= e.cfg.MinStepInterval {
		decision.Action = DecisionAddLadder
		decision.Reason = reasonJoin(decision.Reason, "ladder_extend")
		return decision
	}
	decision.Action = DecisionManageExit
	decision.Reason = "manage_existing_position"
	return decision
}

func (e *SignalEngine) longScore(f Features) float64 {
	score := 0.0
	if f.Snapshot.Imbalance5 >= e.cfg.MinImbalance {
		score += f.Snapshot.Imbalance5
	}
	if f.PressureDelta >= e.cfg.MinPressureDelta {
		score += f.PressureDelta
	}
	if f.HasLookback && f.BidPulseTicks >= e.cfg.MinPulseTicks {
		score += float64(f.BidPulseTicks) * 0.6
	}
	if f.MicroPriceDelta > 0 {
		score += f.MicroPriceDelta / maxFloat(e.cfg.TickSize, 1e-9)
	}
	if f.HasLookback && (f.Snapshot.Imbalance5-f.Previous.Imbalance5) >= e.cfg.MinImbalanceDelta {
		score += f.Snapshot.Imbalance5 - f.Previous.Imbalance5
	}
	return score
}

func (e *SignalEngine) shortScore(f Features) float64 {
	score := 0.0
	if -f.Snapshot.Imbalance5 >= e.cfg.MinImbalance {
		score += -f.Snapshot.Imbalance5
	}
	if -f.PressureDelta >= e.cfg.MinPressureDelta {
		score += -f.PressureDelta
	}
	if f.HasLookback && f.AskPulseTicks >= e.cfg.MinPulseTicks {
		score += float64(f.AskPulseTicks) * 0.6
	}
	if f.MicroPriceDelta < 0 {
		score += -f.MicroPriceDelta / maxFloat(e.cfg.TickSize, 1e-9)
	}
	if f.HasLookback && -(f.Snapshot.Imbalance5-f.Previous.Imbalance5) >= e.cfg.MinImbalanceDelta {
		score += -(f.Snapshot.Imbalance5 - f.Previous.Imbalance5)
	}
	return score
}

func (e *SignalEngine) rejectEntrySignal(side Side, f Features) string {
	if side == SideNone {
		return "no_signal"
	}
	if e.cfg.MaxSpreadTicksInWindow > 0 && f.MaxRecentSpreadTicks > e.cfg.MaxSpreadTicksInWindow {
		return "spread_unstable"
	}
	if dominantSide(f.Snapshot.Imbalance5, f.PressureDelta) != side {
		return "signal_side_misaligned"
	}
	if e.cfg.SignalConfirmMinTicks > 0 && f.SignalConfirmCount < e.cfg.SignalConfirmMinTicks {
		return "signal_not_confirmed"
	}
	if e.cfg.SignalConfirmMinAge > 0 && f.SignalConfirmAge < e.cfg.SignalConfirmMinAge {
		return "signal_too_fresh"
	}
	microTicks := f.MicroPriceDelta / maxFloat(e.cfg.TickSize, 1e-9)
	switch side {
	case SideLong:
		if e.cfg.MinMicroPriceTicks > 0 && microTicks < e.cfg.MinMicroPriceTicks {
			return "microprice_not_aligned"
		}
		if e.cfg.MaxMicroPriceTicks > 0 && microTicks > e.cfg.MaxMicroPriceTicks {
			return "microprice_too_extended"
		}
	case SideShort:
		microTicks = -microTicks
		if e.cfg.MinMicroPriceTicks > 0 && microTicks < e.cfg.MinMicroPriceTicks {
			return "microprice_not_aligned"
		}
		if e.cfg.MaxMicroPriceTicks > 0 && microTicks > e.cfg.MaxMicroPriceTicks {
			return "microprice_too_extended"
		}
	}
	if rejectReason := e.rejectPriceCorridor(side, f); rejectReason != "" {
		return rejectReason
	}
	return ""
}

func (e *SignalEngine) rejectPriceCorridor(signalSide Side, f Features) string {
	if e.cfg.PriceCorridorWindow <= 0 {
		return ""
	}
	if !f.HasPriceCorridor || f.Snapshot.Mid <= 0 {
		return "price_range_unavailable"
	}
	price := f.Snapshot.Mid
	if price > f.PriceLowerBound && price < f.PriceUpperBound {
		return "price_inside_range"
	}
	switch executionSide(e.cfg, signalSide) {
	case SideLong:
		if price > f.PriceLowerBound {
			return "price_not_at_lower_band"
		}
		if f.PriceMaxLowerBound > 0 && price < f.PriceMaxLowerBound {
			return "price_below_range_extension"
		}
	case SideShort:
		if price < f.PriceUpperBound {
			return "price_not_at_upper_band"
		}
		if f.PriceMaxUpperBound > 0 && price > f.PriceMaxUpperBound {
			return "price_above_range_extension"
		}
	}
	return ""
}
