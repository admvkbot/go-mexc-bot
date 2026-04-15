package scalper

import (
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
)

// RiskGuard blocks new entries during unstable market conditions and escalates exits.
type RiskGuard struct {
	cfg config.Scalper
}

func NewRiskGuard(cfg config.Scalper) *RiskGuard {
	return &RiskGuard{cfg: cfg}
}

func (g *RiskGuard) AllowEntry(now time.Time, features Features, ladder *LadderContext) (bool, string, time.Time) {
	if g.cfg.KillSwitch {
		return false, "kill_switch", now
	}
	if features.VolatilityPause {
		// Pause is already active (BookState.volPauseUntil). Returning a non-zero
		// pauseUntil would make LiveRuntime call PauseVolatility on every tick and
		// push the deadline forward forever (now+VolatilityPause each time).
		return false, "volatility_pause", time.Time{}
	}
	if features.Stale {
		pause := g.cfg.StaleBookVolatilityPause
		if pause <= 0 {
			pause = g.cfg.VolatilityPause
		}
		return false, "stale_book", now.Add(pause)
	}
	if features.WideSpread {
		return false, "spread_guard", now.Add(g.cfg.VolatilityPause)
	}
	if features.Chaos {
		return false, "update_rate_guard", now.Add(g.cfg.VolatilityPause)
	}
	return true, "", time.Time{}
}

func (g *RiskGuard) ShouldFlatten(now time.Time, features Features, ladder *LadderContext) (bool, string) {
	if ladder == nil || !ladder.HasInventory() {
		return false, ""
	}
	if g.cfg.KillSwitch {
		return true, "kill_switch"
	}
	if features.Stale {
		return true, "stale_book_flatten"
	}
	if features.WideSpread {
		return true, "spread_expansion_flatten"
	}
	if features.Chaos {
		return true, "churn_flatten"
	}
	if !g.cfg.ExitUsesExchangeBracket() {
		if !ladder.EntryStartedAt.IsZero() && now.Sub(ladder.EntryStartedAt) >= g.cfg.TimeStop {
			return true, "time_stop"
		}
		markPx := exitReferencePrice(features.Snapshot, ladder.Side)
		if markPx > 0 {
			pnl := pnlTicks(ladder.AvgEntryPrice, markPx, g.cfg.TickSize, ladder.Side)
			if -pnl >= float64(g.cfg.StopLossTicks) {
				return true, "stop_loss_ticks"
			}
		}
	}
	return false, ""
}

func exitReferencePrice(snapshot Snapshot, side Side) float64 {
	switch side {
	case SideLong:
		return snapshot.BestBidPx
	case SideShort:
		return snapshot.BestAskPx
	default:
		return 0
	}
}
