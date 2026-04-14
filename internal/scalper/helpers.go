package scalper

import (
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"
)

var sessionCounter atomic.Uint64

func newSessionID(prefix string) string {
	n := sessionCounter.Add(1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UTC().UnixMilli(), n)
}

func normalizePrice(price, tick float64) float64 {
	if tick <= 0 {
		return price
	}
	return math.Round(price/tick) * tick
}

func priceDeltaTicks(from, to, tick float64) int {
	if tick <= 0 || from == 0 || to == 0 {
		return 0
	}
	return int(math.Round((to - from) / tick))
}

func pnlTicks(entryPx, exitPx, tick float64, side Side) float64 {
	if tick <= 0 || entryPx == 0 || exitPx == 0 {
		return 0
	}
	switch side {
	case SideLong:
		return (exitPx - entryPx) / tick
	case SideShort:
		return (entryPx - exitPx) / tick
	default:
		return 0
	}
}

func grossPnL(entryPx, exitPx, qty float64, side Side) float64 {
	switch side {
	case SideLong:
		return (exitPx - entryPx) * qty
	case SideShort:
		return (entryPx - exitPx) * qty
	default:
		return 0
	}
}

func trimRecent[T any](xs []T, keep int) []T {
	if keep <= 0 || len(xs) <= keep {
		return xs
	}
	cp := make([]T, keep)
	copy(cp, xs[len(xs)-keep:])
	return cp
}

func sideToString(side Side) string {
	return string(side)
}

func reasonJoin(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "; ")
}
