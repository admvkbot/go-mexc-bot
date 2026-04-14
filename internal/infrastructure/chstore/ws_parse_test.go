package chstore

import (
	"math"
	"testing"
	"time"
)

func TestParseDepthTopFromMessageJSON(t *testing.T) {
	const raw = `{
  "symbol": "TAO_USDT",
  "data": {
    "asks": [[250.18, 100, 1], [250.20, 200, 1]],
    "bids": [[250.10, 50, 1], [250.05, 10, 1]],
    "begin": 1,
    "end": 2,
    "version": 99
  },
  "channel": "push.depth",
  "ts": 1776166891963
}`
	ts := time.Date(2026, 4, 14, 8, 0, 0, 0, time.UTC)
	row, ok := ParseDepthTopFromMessageJSON(raw, "TAO_USDT", "push.depth", ts)
	if !ok {
		t.Fatal("expected ok")
	}
	if math.Abs(row.BestBidPx-250.10) > 1e-9 || math.Abs(row.BestAskPx-250.18) > 1e-9 {
		t.Fatalf("best bid/ask: got %+v", row)
	}
	wantMid := (250.10 + 250.18) / 2
	wantSp := 250.18 - 250.10
	if math.Abs(row.Mid-wantMid) > 1e-6 || math.Abs(row.Spread-wantSp) > 1e-6 {
		t.Fatalf("mid/spread: got %+v want mid=%v spread=%v", row, wantMid, wantSp)
	}
	if row.Version != 99 || row.ExchangeTSMs != 1776166891963 {
		t.Fatalf("meta: %+v", row)
	}
}

func TestParseDealTicksFromMessageJSON_objects(t *testing.T) {
	const raw = `{
  "symbol": "TAO_USDT",
  "data": [
    {"p": 250.1, "v": 2, "T": 1, "t": 1700000000001},
    {"p": 250.2, "v": 3, "S": 2, "ts": 1700000000002}
  ],
  "channel": "push.deal",
  "ts": 1700000000000
}`
	ts := time.Unix(0, 0).UTC()
	rows := ParseDealTicksFromMessageJSON(raw, "TAO_USDT", ts)
	if len(rows) != 2 {
		t.Fatalf("want 2 rows got %d", len(rows))
	}
}

func TestParseDealTicksFromMessageJSON_matrix(t *testing.T) {
	const raw = `{
  "symbol": "TAO_USDT",
  "data": [[250.5, 1.5, 1, 1700000000003]],
  "channel": "push.deal",
  "ts": 1700000000000
}`
	ts := time.Unix(0, 0).UTC()
	rows := ParseDealTicksFromMessageJSON(raw, "TAO_USDT", ts)
	if len(rows) != 1 || rows[0].Price != 250.5 || rows[0].Vol != 1.5 {
		t.Fatalf("got %+v", rows)
	}
}

func TestParseBookLevelsFromMessageJSON_keepsDeletesAndRanks(t *testing.T) {
	const raw = `{
  "symbol": "TAO_USDT",
  "data": {
    "asks": [[250.18, 100, 2], [250.17, 0, 0]],
    "bids": [[250.10, 50, 1], [250.11, 5, 1], [250.09, 0, 0]],
    "begin": 10,
    "end": 11,
    "version": 12
  },
  "channel": "push.depth",
  "ts": 1776166891963
}`
	ts := time.Unix(0, 0).UTC()
	rows := ParseBookLevelsFromMessageJSON(raw, "TAO_USDT", "push.depth", ts, 2)
	if len(rows) != 4 {
		t.Fatalf("want 4 rows got %d: %+v", len(rows), rows)
	}
	if rows[0].Side != "bid" || rows[0].LevelRank != 1 || math.Abs(rows[0].Price-250.11) > 1e-9 {
		t.Fatalf("unexpected best bid row: %+v", rows[0])
	}
	if rows[1].Side != "bid" || rows[1].LevelRank != 2 || math.Abs(rows[1].Price-250.10) > 1e-9 {
		t.Fatalf("unexpected second bid row: %+v", rows[1])
	}
	if rows[2].Side != "ask" || rows[2].LevelRank != 1 || rows[2].IsDelete != 1 || math.Abs(rows[2].Price-250.17) > 1e-9 {
		t.Fatalf("unexpected best ask row: %+v", rows[2])
	}
}
