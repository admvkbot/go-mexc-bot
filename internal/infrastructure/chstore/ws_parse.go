package chstore

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DepthTopRow is one normalized row per push.depth / push.depth.full* message (best bid/ask snapshot).
type DepthTopRow struct {
	IngestedAt   time.Time
	ExchangeTSMs int64
	Symbol       string
	Channel      string
	Version      int64
	Begin        int64
	End          int64
	BestBidPx    float64
	BestBidVol   float64
	BestAskPx    float64
	BestAskVol   float64
	Mid          float64
	Spread       float64
	BidVol5      float64
	AskVol5      float64
}

// DealTickRow is one public trade tick from push.deal.
type DealTickRow struct {
	IngestedAt   time.Time
	ExchangeTSMs int64
	Symbol       string
	Price        float64
	Vol          float64
	SideCode     int32
	TradeTSMs    int64
}

type depthData struct {
	Bids    [][]any `json:"bids"`
	Asks    [][]any `json:"asks"`
	Begin   int64   `json:"begin"`
	End     int64   `json:"end"`
	Version int64   `json:"version"`
}

type wsEnvelope struct {
	Symbol  string          `json:"symbol"`
	Channel string          `json:"channel"`
	TS      int64           `json:"ts"`
	Data    json.RawMessage `json:"data"`
}

// ParseDepthTopFromMessageJSON parses a full WS frame JSON for depth channels.
func ParseDepthTopFromMessageJSON(messageJSON, symbol, channel string, ingestedAt time.Time) (DepthTopRow, bool) {
	ch := strings.TrimSpace(channel)
	if ch != "push.depth" && !strings.HasPrefix(ch, "push.depth.full") {
		return DepthTopRow{}, false
	}
	var env wsEnvelope
	if err := json.Unmarshal([]byte(messageJSON), &env); err != nil {
		return DepthTopRow{}, false
	}
	var dd depthData
	if err := json.Unmarshal(env.Data, &dd); err != nil {
		return DepthTopRow{}, false
	}
	sym := strings.TrimSpace(env.Symbol)
	if sym == "" {
		sym = symbol
	}
	bbp, bbv, okb := bestLevel(dd.Bids, true)
	bap, bav, oka := bestLevel(dd.Asks, false)
	if !okb && !oka {
		return DepthTopRow{}, false
	}
	mid := 0.0
	sp := 0.0
	if okb && oka {
		mid = (bbp + bap) / 2
		sp = bap - bbp
	}
	return DepthTopRow{
		IngestedAt:   ingestedAt.UTC(),
		ExchangeTSMs: env.TS,
		Symbol:       sym,
		Channel:      ch,
		Version:      dd.Version,
		Begin:        dd.Begin,
		End:          dd.End,
		BestBidPx:    bbp,
		BestBidVol:   bbv,
		BestAskPx:    bap,
		BestAskVol:   bav,
		Mid:          mid,
		Spread:       sp,
		BidVol5:      sumVolTopN(dd.Bids, 5, true),
		AskVol5:      sumVolTopN(dd.Asks, 5, false),
	}, true
}

func bestLevel(levels [][]any, bids bool) (px, vol float64, ok bool) {
	if len(levels) == 0 {
		return 0, 0, false
	}
	type idxPx struct {
		i int
		p float64
	}
	var cand []idxPx
	for i := range levels {
		p, v, okL := levelTuple(levels[i])
		if !okL || v <= 0 {
			continue
		}
		cand = append(cand, idxPx{i: i, p: p})
	}
	if len(cand) == 0 {
		return 0, 0, false
	}
	if bids {
		sort.Slice(cand, func(a, b int) bool { return cand[a].p > cand[b].p })
	} else {
		sort.Slice(cand, func(a, b int) bool { return cand[a].p < cand[b].p })
	}
	return levelTuple(levels[cand[0].i])
}

type priceVol struct {
	p float64
	v float64
}

func sumVolTopN(levels [][]any, n int, bids bool) float64 {
	var xs []priceVol
	for _, lv := range levels {
		p, v, ok := levelTuple(lv)
		if !ok || v <= 0 {
			continue
		}
		xs = append(xs, priceVol{p: p, v: v})
	}
	if len(xs) == 0 {
		return 0
	}
	if bids {
		sort.Slice(xs, func(a, b int) bool { return xs[a].p > xs[b].p })
	} else {
		sort.Slice(xs, func(a, b int) bool { return xs[a].p < xs[b].p })
	}
	s := 0.0
	for i := 0; i < n && i < len(xs); i++ {
		s += xs[i].v
	}
	return s
}

func levelTuple(a []any) (px float64, vol float64, ok bool) {
	if len(a) < 2 {
		return 0, 0, false
	}
	px, ok = toFloat(a[0])
	if !ok {
		return 0, 0, false
	}
	vol, ok = toFloat(a[1])
	return px, vol, ok
}

func toFloat(x any) (float64, bool) {
	switch t := x.(type) {
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return 0, false
		}
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// ParseDealTicksFromMessageJSON extracts trade rows from push.deal frame JSON.
func ParseDealTicksFromMessageJSON(messageJSON, symbol string, ingestedAt time.Time) []DealTickRow {
	var env wsEnvelope
	if err := json.Unmarshal([]byte(messageJSON), &env); err != nil || env.Channel != "push.deal" {
		return nil
	}
	sym := strings.TrimSpace(env.Symbol)
	if sym == "" {
		sym = symbol
	}
	root := env.Data
	var asMatrix [][]any
	if json.Unmarshal(root, &asMatrix) == nil && len(asMatrix) > 0 && len(asMatrix[0]) >= 2 {
		if dr := mapDealMatrix(asMatrix, sym, env.TS, ingestedAt); len(dr) > 0 {
			return dr
		}
	}
	var asArray []map[string]any
	if json.Unmarshal(root, &asArray) == nil && len(asArray) > 0 {
		return mapDealMaps(asArray, sym, env.TS, ingestedAt)
	}
	var asObj struct {
		Deals []map[string]any `json:"deals"`
	}
	if json.Unmarshal(root, &asObj) == nil && len(asObj.Deals) > 0 {
		return mapDealMaps(asObj.Deals, sym, env.TS, ingestedAt)
	}
	var one map[string]any
	if json.Unmarshal(root, &one) == nil && len(one) > 0 {
		return mapDealMaps([]map[string]any{one}, sym, env.TS, ingestedAt)
	}
	return nil
}

func mapDealMatrix(rows [][]any, sym string, frameTS int64, ingested time.Time) []DealTickRow {
	var out []DealTickRow
	for _, r := range rows {
		if len(r) < 2 {
			continue
		}
		p, ok1 := toFloat(r[0])
		v, ok2 := toFloat(r[1])
		if !ok1 || !ok2 || (p == 0 && v == 0) {
			continue
		}
		var side int32
		var tradeMS int64
		if len(r) >= 3 {
			if s, ok := toFloat(r[2]); ok {
				side = int32(s)
			}
		}
		if len(r) >= 4 {
			if t, ok := toFloat(r[3]); ok {
				tradeMS = int64(t)
			}
		}
		out = append(out, DealTickRow{
			IngestedAt:   ingested.UTC(),
			ExchangeTSMs: frameTS,
			Symbol:       sym,
			Price:        p,
			Vol:          v,
			SideCode:     side,
			TradeTSMs:    tradeMS,
		})
	}
	return out
}

func mapDealMaps(deals []map[string]any, sym string, frameTS int64, ingested time.Time) []DealTickRow {
	var out []DealTickRow
	for _, d := range deals {
		p := pickFloat(d, "p", "price", "P")
		v := pickFloat(d, "v", "vol", "V", "volume")
		if p == 0 && v == 0 {
			continue
		}
		tradeMS := int64(pickFloat(d, "t", "ts"))
		side := int32(pickInt(d, "S", "side"))
		if side == 0 {
			if tv := pickInt(d, "T"); tv >= 1 && tv <= 4 {
				side = int32(tv)
			}
		}
		out = append(out, DealTickRow{
			IngestedAt:   ingested.UTC(),
			ExchangeTSMs: frameTS,
			Symbol:       sym,
			Price:        p,
			Vol:          v,
			SideCode:     side,
			TradeTSMs:    tradeMS,
		})
	}
	return out
}

func pickInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if x, ok := m[k]; ok {
			switch t := x.(type) {
			case float64:
				return int(t)
			case json.Number:
				i, _ := t.Int64()
				return int(i)
			case string:
				v, _ := strconv.Atoi(strings.TrimSpace(t))
				return v
			}
		}
	}
	return 0
}

func pickFloat(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if x, ok := m[k]; ok {
			f, ok := toFloat(x)
			if ok {
				return f
			}
		}
	}
	return 0
}
