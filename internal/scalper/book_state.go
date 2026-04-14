package scalper

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/chstore"
)

type bookSnapshotEvent struct {
	at       time.Time
	snapshot Snapshot
}

// BookState keeps an in-memory view of the top-of-book and short-term market microstructure.
type BookState struct {
	cfg config.Scalper

	mu            sync.RWMutex
	symbol        string
	bids          map[float64]Level
	asks          map[float64]Level
	lastUpdate    time.Time
	lastExchange  int64
	updateSeq     int64
	recent        []bookSnapshotEvent
	volPauseUntil time.Time
}

func NewBookState(cfg config.Scalper) *BookState {
	return &BookState{
		cfg:    cfg,
		symbol: cfg.Symbol,
		bids:   make(map[float64]Level),
		asks:   make(map[float64]Level),
	}
}

// ApplyMessage updates the in-memory book from a raw WS payload.
func (b *BookState) ApplyMessage(messageJSON, symbol, channel string, ingestedAt time.Time) bool {
	levels := chstore.ParseBookLevelsFromMessageJSON(messageJSON, symbol, channel, ingestedAt, 20)
	depthTop, ok := chstore.ParseDepthTopFromMessageJSON(messageJSON, symbol, channel, ingestedAt)
	if len(levels) == 0 && !ok {
		return false
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if ok && strings.TrimSpace(depthTop.Symbol) != "" {
		b.symbol = depthTop.Symbol
		b.lastExchange = depthTop.ExchangeTSMs
	}

	if strings.HasPrefix(channel, "push.depth.full") {
		b.bids = make(map[float64]Level)
		b.asks = make(map[float64]Level)
	}

	for _, row := range levels {
		level := Level{Price: row.Price, Volume: row.Vol, Orders: row.Orders}
		priceKey := normalizePrice(level.Price, b.cfg.TickSize)
		switch row.Side {
		case "bid":
			if row.IsDelete == 1 || level.Volume <= 0 {
				delete(b.bids, priceKey)
				continue
			}
			b.bids[priceKey] = level
		case "ask":
			if row.IsDelete == 1 || level.Volume <= 0 {
				delete(b.asks, priceKey)
				continue
			}
			b.asks[priceKey] = level
		}
	}

	b.lastUpdate = ingestedAt.UTC()
	b.updateSeq++
	b.captureSnapshotLocked()
	return true
}

func (b *BookState) Snapshot() Snapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.snapshotLocked()
}

func (b *BookState) Features(now time.Time) Features {
	b.mu.RLock()
	defer b.mu.RUnlock()

	current := b.snapshotLocked()
	current.At = b.lastUpdate
	features := Features{
		Snapshot:        current,
		BookAge:         now.UTC().Sub(b.lastUpdate),
		Window:          b.cfg.FeatureLookback,
		SpreadTicks:     current.Spread / maxFloat(b.cfg.TickSize, 1e-9),
		MicroPriceDelta: microPriceDelta(current),
	}
	if !b.lastUpdate.IsZero() {
		features.Stale = features.BookAge > b.cfg.MaxBookAge
	}
	if b.cfg.MaxSpreadTicks > 0 {
		features.WideSpread = features.SpreadTicks > b.cfg.MaxSpreadTicks
	}
	if len(b.recent) > 0 && b.cfg.FeatureLookback > 0 {
		cutoff := now.Add(-b.cfg.FeatureLookback)
		prev := b.recent[0].snapshot
		count := 0
		for _, item := range b.recent {
			if item.at.Before(cutoff) {
				continue
			}
			if !features.HasLookback {
				prev = item.snapshot
				features.HasLookback = true
			}
			count++
		}
		if features.HasLookback {
			features.Previous = prev
			features.BidPulseTicks = priceDeltaTicks(prev.BestBidPx, current.BestBidPx, b.cfg.TickSize)
			features.AskPulseTicks = priceDeltaTicks(current.BestAskPx, prev.BestAskPx, b.cfg.TickSize)
			features.PressureDelta = current.Imbalance5 - prev.Imbalance5
		}
		if count > 0 && b.cfg.FeatureLookback > 0 {
			features.UpdateRate = float64(count) / b.cfg.FeatureLookback.Seconds()
		}
	}
	features.Chaos = b.cfg.MaxUpdateRate > 0 && features.UpdateRate > b.cfg.MaxUpdateRate
	features.VolatilityPause = now.Before(b.volPauseUntil)
	features.SignalImbalance = current.Imbalance5
	features.SignalSide = dominantSide(current.Imbalance5, features.PressureDelta)
	features.SignalConfidence = signalConfidence(features)
	return features
}

func (b *BookState) PauseVolatility(until time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if until.After(b.volPauseUntil) {
		b.volPauseUntil = until
	}
}

func (b *BookState) captureSnapshotLocked() {
	snap := b.snapshotLocked()
	b.recent = append(b.recent, bookSnapshotEvent{at: b.lastUpdate, snapshot: snap})
	cutoff := b.lastUpdate.Add(-2 * time.Second)
	start := 0
	for start < len(b.recent) && b.recent[start].at.Before(cutoff) {
		start++
	}
	if start > 0 {
		b.recent = append([]bookSnapshotEvent(nil), b.recent[start:]...)
	}
}

func (b *BookState) snapshotLocked() Snapshot {
	bids := sortedLevels(b.bids, true)
	asks := sortedLevels(b.asks, false)
	snap := Snapshot{
		Symbol:       b.symbol,
		At:           b.lastUpdate,
		TopBids:      trimLevels(bids, 10),
		TopAsks:      trimLevels(asks, 10),
		HasBook:      len(bids) > 0 && len(asks) > 0,
		UpdateSeq:    b.updateSeq,
		ExchangeTSMs: b.lastExchange,
	}
	if len(bids) > 0 {
		snap.BestBidPx = bids[0].Price
		snap.BestBidVol = bids[0].Volume
		snap.BidVol5 = sumLevelVolume(bids, 5)
		snap.BidVol10 = sumLevelVolume(bids, 10)
	}
	if len(asks) > 0 {
		snap.BestAskPx = asks[0].Price
		snap.BestAskVol = asks[0].Volume
		snap.AskVol5 = sumLevelVolume(asks, 5)
		snap.AskVol10 = sumLevelVolume(asks, 10)
	}
	if snap.BestBidPx > 0 && snap.BestAskPx > 0 {
		snap.Mid = (snap.BestBidPx + snap.BestAskPx) / 2
		snap.Spread = snap.BestAskPx - snap.BestBidPx
	}
	total := snap.BidVol5 + snap.AskVol5
	if total > 0 {
		snap.Imbalance5 = (snap.BidVol5 - snap.AskVol5) / total
	}
	return snap
}

func sortedLevels(src map[float64]Level, desc bool) []Level {
	out := make([]Level, 0, len(src))
	for _, level := range src {
		out = append(out, level)
	}
	sort.Slice(out, func(i, j int) bool {
		if desc {
			return out[i].Price > out[j].Price
		}
		return out[i].Price < out[j].Price
	})
	return out
}

func sumLevelVolume(levels []Level, n int) float64 {
	sum := 0.0
	for i := 0; i < len(levels) && i < n; i++ {
		sum += levels[i].Volume
	}
	return sum
}

func trimLevels(levels []Level, n int) []Level {
	if len(levels) <= n {
		cp := make([]Level, len(levels))
		copy(cp, levels)
		return cp
	}
	cp := make([]Level, n)
	copy(cp, levels[:n])
	return cp
}

func microPriceDelta(s Snapshot) float64 {
	if s.BestBidPx == 0 || s.BestAskPx == 0 {
		return 0
	}
	total := s.BidVol5 + s.AskVol5
	if total == 0 {
		return 0
	}
	micro := ((s.BestAskPx * s.BidVol5) + (s.BestBidPx * s.AskVol5)) / total
	return micro - s.Mid
}

func dominantSide(imbalance, pressure float64) Side {
	if imbalance >= 0 && pressure >= 0 {
		return SideLong
	}
	if imbalance <= 0 && pressure <= 0 {
		return SideShort
	}
	return SideNone
}

func signalConfidence(f Features) float64 {
	score := absFloat(f.Snapshot.Imbalance5) + absFloat(f.PressureDelta)
	if f.BidPulseTicks > 0 {
		score += float64(f.BidPulseTicks) * 0.5
	}
	if f.AskPulseTicks > 0 {
		score += float64(f.AskPulseTicks) * 0.5
	}
	return score
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
