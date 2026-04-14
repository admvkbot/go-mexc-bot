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

	if ladder == nil || ladder.Phase == PhaseIdle || now.After(ladder.CooldownUntil) {
		decision.Action = DecisionEnter
		return decision
	}
	if ladder.Side == decision.Side && ladder.StepCount < ladder.MaxSteps && now.Sub(ladder.LastStepAt) >= e.cfg.MinStepInterval {
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
