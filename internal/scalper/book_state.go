package scalper

import (
	"math"
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

type dealBatch struct {
	at        time.Time
	signedVol float64
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
	recentDeals   []dealBatch
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

// ProcessMarketMessage applies depth or (when deal filter is enabled) deal payloads; returns true if state advanced and a strategy tick should run.
func (b *BookState) ProcessMarketMessage(messageJSON, symbol, channel string, ingestedAt time.Time) bool {
	if b.ApplyMessage(messageJSON, symbol, channel, ingestedAt) {
		return true
	}
	if b.cfg.EntryDealFilterEnabled && b.ApplyDealMessage(messageJSON, symbol, channel, ingestedAt) {
		return true
	}
	return false
}

// ApplyDealMessage accumulates signed trade volume (MEXC side 1 = buy +, 2 = sell −) for optional entry alignment.
func (b *BookState) ApplyDealMessage(messageJSON, symbol, channel string, ingestedAt time.Time) bool {
	if strings.TrimSpace(channel) != "push.deal" {
		return false
	}
	rows := chstore.ParseDealTicksFromMessageJSON(messageJSON, symbol, ingestedAt)
	if len(rows) == 0 {
		return false
	}
	var signed float64
	for _, row := range rows {
		switch row.SideCode {
		case 1:
			signed += row.Vol
		case 2:
			signed -= row.Vol
		}
	}
	at := ingestedAt.UTC()
	b.mu.Lock()
	b.appendDealBatchesLocked(at, signed)
	b.mu.Unlock()
	return true
}

func (b *BookState) appendDealBatchesLocked(at time.Time, signed float64) {
	win := b.cfg.EntryDealWindow
	if win <= 0 {
		win = time.Second
	}
	b.recentDeals = append(b.recentDeals, dealBatch{at: at, signedVol: signed})
	cutoff := at.Add(-win - 250*time.Millisecond)
	start := 0
	for start < len(b.recentDeals) && b.recentDeals[start].at.Before(cutoff) {
		start++
	}
	if start > 0 {
		b.recentDeals = append([]dealBatch(nil), b.recentDeals[start:]...)
	}
}

func (b *BookState) dealTapeLocked(now time.Time) (delta float64, hasTape bool) {
	if !b.cfg.EntryDealFilterEnabled {
		return 0, false
	}
	win := b.cfg.EntryDealWindow
	if win <= 0 {
		win = time.Second
	}
	cutoff := now.Add(-win)
	for _, batch := range b.recentDeals {
		if !batch.at.Before(cutoff) {
			hasTape = true
			delta += batch.signedVol
		}
	}
	return delta, hasTape
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
		Snapshot:             current,
		BookAge:              now.UTC().Sub(b.lastUpdate),
		Window:               b.cfg.FeatureLookback,
		SpreadTicks:          current.Spread / maxFloat(b.cfg.TickSize, 1e-9),
		MaxRecentSpreadTicks: current.Spread / maxFloat(b.cfg.TickSize, 1e-9),
		MicroPriceDelta:      microPriceDelta(current),
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
		}
		if count > 0 && b.cfg.FeatureLookback > 0 {
			features.UpdateRate = float64(count) / b.cfg.FeatureLookback.Seconds()
		}
	}
	if len(b.recent) >= 2 {
		last := b.recent[len(b.recent)-2].snapshot
		if last.HasBook && current.HasBook {
			features.LastSnapshot = last
			features.HasLastSnapshot = true
			features.BidPulseTicks = priceDeltaTicks(last.BestBidPx, current.BestBidPx, b.cfg.TickSize)
			features.AskPulseTicks = priceDeltaTicks(current.BestAskPx, last.BestAskPx, b.cfg.TickSize)
			features.PressureDelta = current.Imbalance5 - last.Imbalance5
		}
	}
	features.DealVolDelta1s, features.HasDealTape1s = b.dealTapeLocked(now.UTC())
	if b.cfg.SpreadStabilityWindow > 0 {
		features.MaxRecentSpreadTicks = b.maxSpreadTicksLocked(now.Add(-b.cfg.SpreadStabilityWindow))
	}
	if b.cfg.PriceCorridorWindow > 0 {
		features.PriceMean, features.PriceUpperDeviation, features.PriceLowerDeviation, features.PriceCorridorSamples, features.HasPriceCorridor =
			b.priceCorridorLocked(now)
		if features.HasPriceCorridor {
			features.PriceUpperBound = features.PriceMean + features.PriceUpperDeviation
			features.PriceLowerBound = features.PriceMean - features.PriceLowerDeviation
			multiplier := b.cfg.PriceCorridorMaxMultiplier
			if multiplier <= 0 {
				multiplier = 2
			}
			features.PriceMaxUpperBound = features.PriceMean + features.PriceUpperDeviation*multiplier
			features.PriceMaxLowerBound = features.PriceMean - features.PriceLowerDeviation*multiplier
		}
	}
	features.Chaos = b.cfg.MaxUpdateRate > 0 && features.UpdateRate > b.cfg.MaxUpdateRate
	features.VolatilityPause = now.Before(b.volPauseUntil)
	features.SignalImbalance = current.Imbalance5
	features.SignalSide = dominantSide(current.Imbalance5, features.PressureDelta)
	if features.SignalSide != SideNone && b.cfg.SignalConfirmWindow > 0 {
		features.SignalConfirmCount, features.SignalConfirmAge = b.signalConfirmationLocked(now, features.SignalSide)
	}
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
	cutoff := b.lastUpdate.Add(-b.retentionWindowLocked())
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

func (b *BookState) maxSpreadTicksLocked(cutoff time.Time) float64 {
	maxSpread := 0.0
	for _, item := range b.recent {
		if item.at.Before(cutoff) {
			continue
		}
		spreadTicks := item.snapshot.Spread / maxFloat(b.cfg.TickSize, 1e-9)
		if spreadTicks > maxSpread {
			maxSpread = spreadTicks
		}
	}
	if maxSpread == 0 {
		return b.snapshotLocked().Spread / maxFloat(b.cfg.TickSize, 1e-9)
	}
	return maxSpread
}

func (b *BookState) signalConfirmationLocked(now time.Time, side Side) (int, time.Duration) {
	if side == SideNone || len(b.recent) < 2 {
		return 0, 0
	}
	cutoff := now.Add(-b.cfg.SignalConfirmWindow)
	count := 0
	var earliest time.Time
	for i := len(b.recent) - 1; i >= 1; i-- {
		cur := b.recent[i]
		if cur.at.Before(cutoff) {
			break
		}
		prev := b.recent[i-1]
		if signalSideFromSnapshots(prev.snapshot, cur.snapshot) != side {
			break
		}
		count++
		earliest = prev.at
	}
	if count == 0 || earliest.IsZero() {
		return 0, 0
	}
	return count, now.Sub(earliest)
}

func (b *BookState) priceCorridorLocked(now time.Time) (mean, upperDev, lowerDev float64, samples int, ok bool) {
	if b.cfg.PriceCorridorWindow <= 0 {
		return 0, 0, 0, 0, false
	}
	cutoff := now.Add(-b.cfg.PriceCorridorWindow)
	mids := make([]float64, 0, len(b.recent))
	for _, item := range b.recent {
		if item.at.Before(cutoff) || item.snapshot.Mid <= 0 {
			continue
		}
		mids = append(mids, item.snapshot.Mid)
	}
	samples = len(mids)
	if samples < 2 {
		return 0, 0, 0, samples, false
	}
	for _, mid := range mids {
		mean += mid
	}
	mean /= float64(samples)
	up := make([]float64, 0, samples)
	down := make([]float64, 0, samples)
	for _, mid := range mids {
		delta := mid - mean
		switch {
		case delta > 0:
			up = append(up, delta)
		case delta < 0:
			down = append(down, -delta)
		}
	}
	if len(up) == 0 || len(down) == 0 {
		return mean, 0, 0, samples, false
	}
	p := b.cfg.PriceCorridorPercentile
	if p <= 0 {
		p = 0.8
	}
	upperDev = percentileFloat64(up, p)
	lowerDev = percentileFloat64(down, p)
	if upperDev <= 0 || lowerDev <= 0 {
		return mean, upperDev, lowerDev, samples, false
	}
	return mean, upperDev, lowerDev, samples, true
}

func (b *BookState) retentionWindowLocked() time.Duration {
	maxWindow := 2 * time.Second
	for _, d := range []time.Duration{
		b.cfg.FeatureLookback,
		b.cfg.SpreadStabilityWindow,
		b.cfg.SignalConfirmWindow,
		b.cfg.PriceCorridorWindow,
	} {
		if d > maxWindow {
			maxWindow = d
		}
	}
	if maxWindow <= 0 {
		return 2 * time.Second
	}
	return maxWindow
}

func signalSideFromSnapshots(prev, current Snapshot) Side {
	return dominantSide(current.Imbalance5, current.Imbalance5-prev.Imbalance5)
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

func percentileFloat64(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	if p <= 0 {
		return cp[0]
	}
	if p >= 1 {
		return cp[len(cp)-1]
	}
	pos := p * float64(len(cp)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return cp[lo]
	}
	weight := pos - float64(lo)
	return cp[lo] + (cp[hi]-cp[lo])*weight
}
