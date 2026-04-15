package scalper

import (
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
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

// tickDecimalPlaces returns n where tick*10^n is (approximately) an integer; used when MEXC priceScale is unknown.
func tickDecimalPlaces(tick float64) int {
	if tick <= 0 {
		return 8
	}
	for n := 0; n <= 12; n++ {
		v := tick * math.Pow10(n)
		if math.Abs(v-math.Round(v)) < 1e-9 {
			return n
		}
	}
	return 8
}

// quantizeOrderPrice snaps to tick grid then to a fixed number of decimal places for REST JSON (MEXC code 2015).
func quantizeOrderPrice(price, tick float64, priceDecimals int) float64 {
	p := normalizePrice(price, tick)
	if priceDecimals < 0 {
		return p
	}
	f := math.Pow10(priceDecimals)
	return math.Round(p*f) / f
}

// quantizeOrderVol rounds volume to volScale decimals (volScale 0 → integer contracts). If volScale < 0, trims float noise only.
func quantizeOrderVol(vol float64, volScale int) float64 {
	if volScale < 0 {
		return math.Round(vol*1e8) / 1e8
	}
	f := math.Pow10(volScale)
	return math.Round(vol*f) / f
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

// executionSide returns the exchange position side for a given signal side.
// When InvertExecution is true, long/short are swapped for orders and ladder state only;
// signal journals still use the original decision from SignalEngine.
func executionSide(cfg config.Scalper, signal Side) Side {
	if !cfg.InvertExecution || signal == SideNone {
		return signal
	}
	switch signal {
	case SideLong:
		return SideShort
	case SideShort:
		return SideLong
	default:
		return signal
	}
}

// priceCorridorReject возвращает причину отказа по полосам коридора или пустую строку, если пропускать.
// Сравнение в целых шагах цены устраняет ошибки двоичной дроби на границе; сторона — как у сигнала книги
// (согласовано с микроценой и проверкой направления); при перевороте исполнения фильтр не меняет полосу.
func priceCorridorReject(cfg config.Scalper, signalSide Side, f Features) string {
	if signalSide == SideNone {
		return ""
	}
	tick := maxFloat(cfg.TickSize, 1e-12)
	midU := int64(math.Round(f.Snapshot.Mid / tick))
	lowU := int64(math.Round(f.PriceLowerBound / tick))
	upU := int64(math.Round(f.PriceUpperBound / tick))
	if midU > lowU && midU < upU {
		return "price_inside_range"
	}
	switch signalSide {
	case SideLong:
		if midU > lowU {
			return "price_not_at_lower_band"
		}
		if f.PriceMaxLowerBound > 0 {
			mlU := int64(math.Round(f.PriceMaxLowerBound / tick))
			if midU < mlU {
				return "price_below_range_extension"
			}
		}
	case SideShort:
		if midU < upU {
			return "price_not_at_upper_band"
		}
		if f.PriceMaxUpperBound > 0 {
			muU := int64(math.Round(f.PriceMaxUpperBound / tick))
			if midU > muU {
				return "price_above_range_extension"
			}
		}
	}
	return ""
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
