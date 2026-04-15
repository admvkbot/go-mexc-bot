package chstore

import (
	"context"
	"fmt"
	"time"
)

const (
	scalperSignalTable    = "scalper_signal_event"
	scalperOrderTable     = "scalper_order_event"
	scalperRoundTripTable = "scalper_position_roundtrip"
	scalperReplayTable    = "scalper_replay_candidate"
)

type ScalperSignalEventRow struct {
	SessionID      string
	LadderID       string
	Mode           string
	Symbol         string
	EventAt        time.Time
	Action         string
	Side           string
	Score          float64
	Reason         string
	AllowEntry     bool
	DenyReason     string
	BestBidPx      float64
	BestAskPx      float64
	Spread         float64
	BidVol5        float64
	AskVol5        float64
	Imbalance5     float64
	BidPulseTicks  int32
	AskPulseTicks  int32
	PressureDelta  float64
	UpdateRate     float64
	MicroPriceDiff float64
	ConfirmCount   int32
	ConfirmMS      int64
	MaxSpreadTicks float64
}

type ScalperOrderEventRow struct {
	SessionID   string
	LadderID    string
	Symbol      string
	EventAt     time.Time
	EventType   string
	OrderClass  string
	Side        string
	ExternalOID string
	OrderID     string
	Price       float64
	Quantity    float64
	FilledQty   float64
	AvgFillPx   float64
	StateCode   int32
	Reason      string
	RawJSON     string
}

type ScalperRoundTripRow struct {
	SessionID           string
	LadderID            string
	Symbol              string
	Side                string
	EntryStartedAt      time.Time
	EntryFilledAt       time.Time
	ExitStartedAt       time.Time
	ExitFilledAt        time.Time
	HoldingMS           int64
	EntryAvgPx          float64
	ExitAvgPx           float64
	FilledQty           float64
	GrossPnL            float64
	PNLTicks            float64
	MaxAdverseTicks     float64
	MaxFavorableTicks   float64
	ExitReason          string
	FlattenReason       string
	RepricesCount       int32
	LadderStepsUsed     int32
	WasEmergencyFlatten uint8
}

type ScalperReplayCandidateRow struct {
	SessionID     string
	Symbol        string
	SignalAt      time.Time
	Side          string
	Score         float64
	Reason        string
	EntryPx       float64
	TargetPx      float64
	StopPx        float64
	ExpireAt      time.Time
	StepCount     int32
	BestBidPx     float64
	BestAskPx     float64
	Imbalance5    float64
	PressureDelta float64
}

func (c *Client) InitScalperSchema(ctx context.Context) error {
	c.schemaMu.Lock()
	defer c.schemaMu.Unlock()
	db := quoteIdent(c.cfg.Database)
	if err := c.conn.Exec(ctx, "CREATE DATABASE IF NOT EXISTS "+db); err != nil {
		return fmt.Errorf("chstore: create database: %w", err)
	}
	queries := []string{
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s
(
    event_at DateTime64(3),
    session_id String,
    ladder_id String,
    mode LowCardinality(String),
    symbol LowCardinality(String),
    action LowCardinality(String),
    side LowCardinality(String),
    score Float64,
    reason String,
    allow_entry UInt8,
    deny_reason String,
    best_bid_px Float64,
    best_ask_px Float64,
    spread Float64,
    bid_vol5 Float64,
    ask_vol5 Float64,
    imbalance5 Float64,
    bid_pulse_ticks Int32,
    ask_pulse_ticks Int32,
    pressure_delta Float64,
    update_rate Float64,
    microprice_delta Float64,
    confirm_count Int32,
    confirm_ms Int64,
    max_spread_ticks Float64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(event_at)
ORDER BY (symbol, event_at, session_id)`, db, quoteIdent(scalperSignalTable)),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s
(
    event_at DateTime64(3),
    session_id String,
    ladder_id String,
    symbol LowCardinality(String),
    event_type LowCardinality(String),
    order_class LowCardinality(String),
    side LowCardinality(String),
    external_oid String,
    order_id String,
    price Float64,
    quantity Float64,
    filled_qty Float64,
    avg_fill_px Float64,
    state_code Int32,
    reason String,
    raw_json String
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(event_at)
ORDER BY (symbol, session_id, ladder_id, event_at, external_oid)`, db, quoteIdent(scalperOrderTable)),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s
(
    exit_filled_at DateTime64(3),
    session_id String,
    ladder_id String,
    symbol LowCardinality(String),
    side LowCardinality(String),
    entry_started_at DateTime64(3),
    entry_filled_at DateTime64(3),
    exit_started_at DateTime64(3),
    holding_ms Int64,
    entry_avg_px Float64,
    exit_avg_px Float64,
    filled_qty Float64,
    gross_pnl Float64,
    pnl_ticks Float64,
    max_adverse_ticks Float64,
    max_favorable_ticks Float64,
    exit_reason LowCardinality(String),
    flatten_reason LowCardinality(String),
    reprices_count Int32,
    ladder_steps_used Int32,
    was_emergency_flatten UInt8
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(exit_filled_at)
ORDER BY (symbol, exit_filled_at, session_id, ladder_id)`, db, quoteIdent(scalperRoundTripTable)),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s
(
    signal_at DateTime64(3),
    session_id String,
    symbol LowCardinality(String),
    side LowCardinality(String),
    score Float64,
    reason String,
    entry_px Float64,
    target_px Float64,
    stop_px Float64,
    expire_at DateTime64(3),
    step_count Int32,
    best_bid_px Float64,
    best_ask_px Float64,
    imbalance5 Float64,
    pressure_delta Float64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(signal_at)
ORDER BY (symbol, signal_at, session_id)`, db, quoteIdent(scalperReplayTable)),
	}
	for _, q := range queries {
		if err := c.conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("chstore: init scalper schema: %w", err)
		}
	}
	alterQueries := []string{
		fmt.Sprintf("ALTER TABLE %s.%s ADD COLUMN IF NOT EXISTS ladder_id String", db, quoteIdent(scalperSignalTable)),
		fmt.Sprintf("ALTER TABLE %s.%s ADD COLUMN IF NOT EXISTS allow_entry UInt8", db, quoteIdent(scalperSignalTable)),
		fmt.Sprintf("ALTER TABLE %s.%s ADD COLUMN IF NOT EXISTS deny_reason String", db, quoteIdent(scalperSignalTable)),
		fmt.Sprintf("ALTER TABLE %s.%s ADD COLUMN IF NOT EXISTS confirm_count Int32", db, quoteIdent(scalperSignalTable)),
		fmt.Sprintf("ALTER TABLE %s.%s ADD COLUMN IF NOT EXISTS confirm_ms Int64", db, quoteIdent(scalperSignalTable)),
		fmt.Sprintf("ALTER TABLE %s.%s ADD COLUMN IF NOT EXISTS max_spread_ticks Float64", db, quoteIdent(scalperSignalTable)),
	}
	for _, q := range alterQueries {
		if err := c.conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("chstore: alter scalper schema: %w", err)
		}
	}
	return nil
}

func (c *Client) InsertScalperSignalEventRows(ctx context.Context, rows []ScalperSignalEventRow) error {
	if len(rows) == 0 {
		return nil
	}
	fq := fmt.Sprintf("%s.%s", quoteIdent(c.cfg.Database), quoteIdent(scalperSignalTable))
	batch, err := c.conn.PrepareBatch(ctx, `INSERT INTO `+fq+` (
event_at, session_id, ladder_id, mode, symbol, action, side, score, reason, allow_entry, deny_reason,
best_bid_px, best_ask_px, spread, bid_vol5, ask_vol5, imbalance5,
bid_pulse_ticks, ask_pulse_ticks, pressure_delta, update_rate, microprice_delta, confirm_count, confirm_ms, max_spread_ticks)`)
	if err != nil {
		return fmt.Errorf("chstore: scalper signal batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.EventAt, r.SessionID, r.LadderID, r.Mode, r.Symbol, r.Action, r.Side, r.Score, r.Reason, boolToUInt8(r.AllowEntry), r.DenyReason,
			r.BestBidPx, r.BestAskPx, r.Spread, r.BidVol5, r.AskVol5, r.Imbalance5,
			r.BidPulseTicks, r.AskPulseTicks, r.PressureDelta, r.UpdateRate, r.MicroPriceDiff, r.ConfirmCount, r.ConfirmMS, r.MaxSpreadTicks,
		); err != nil {
			return fmt.Errorf("chstore: scalper signal append: %w", err)
		}
	}
	return batch.Send()
}

func boolToUInt8(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}

func (c *Client) InsertScalperOrderEventRows(ctx context.Context, rows []ScalperOrderEventRow) error {
	if len(rows) == 0 {
		return nil
	}
	fq := fmt.Sprintf("%s.%s", quoteIdent(c.cfg.Database), quoteIdent(scalperOrderTable))
	batch, err := c.conn.PrepareBatch(ctx, `INSERT INTO `+fq+` (
event_at, session_id, ladder_id, symbol, event_type, order_class, side,
external_oid, order_id, price, quantity, filled_qty, avg_fill_px, state_code, reason, raw_json)`)
	if err != nil {
		return fmt.Errorf("chstore: scalper order batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.EventAt, r.SessionID, r.LadderID, r.Symbol, r.EventType, r.OrderClass, r.Side,
			r.ExternalOID, r.OrderID, r.Price, r.Quantity, r.FilledQty, r.AvgFillPx, r.StateCode, r.Reason, r.RawJSON,
		); err != nil {
			return fmt.Errorf("chstore: scalper order append: %w", err)
		}
	}
	return batch.Send()
}

func (c *Client) InsertScalperRoundTripRows(ctx context.Context, rows []ScalperRoundTripRow) error {
	if len(rows) == 0 {
		return nil
	}
	fq := fmt.Sprintf("%s.%s", quoteIdent(c.cfg.Database), quoteIdent(scalperRoundTripTable))
	batch, err := c.conn.PrepareBatch(ctx, `INSERT INTO `+fq+` (
exit_filled_at, session_id, ladder_id, symbol, side, entry_started_at, entry_filled_at,
exit_started_at, holding_ms, entry_avg_px, exit_avg_px, filled_qty, gross_pnl, pnl_ticks,
max_adverse_ticks, max_favorable_ticks, exit_reason, flatten_reason, reprices_count,
ladder_steps_used, was_emergency_flatten)`)
	if err != nil {
		return fmt.Errorf("chstore: scalper roundtrip batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.ExitFilledAt, r.SessionID, r.LadderID, r.Symbol, r.Side, r.EntryStartedAt, r.EntryFilledAt,
			r.ExitStartedAt, r.HoldingMS, r.EntryAvgPx, r.ExitAvgPx, r.FilledQty, r.GrossPnL, r.PNLTicks,
			r.MaxAdverseTicks, r.MaxFavorableTicks, r.ExitReason, r.FlattenReason, r.RepricesCount,
			r.LadderStepsUsed, r.WasEmergencyFlatten,
		); err != nil {
			return fmt.Errorf("chstore: scalper roundtrip append: %w", err)
		}
	}
	return batch.Send()
}

func (c *Client) InsertScalperReplayCandidateRows(ctx context.Context, rows []ScalperReplayCandidateRow) error {
	if len(rows) == 0 {
		return nil
	}
	fq := fmt.Sprintf("%s.%s", quoteIdent(c.cfg.Database), quoteIdent(scalperReplayTable))
	batch, err := c.conn.PrepareBatch(ctx, `INSERT INTO `+fq+` (
signal_at, session_id, symbol, side, score, reason, entry_px, target_px, stop_px,
expire_at, step_count, best_bid_px, best_ask_px, imbalance5, pressure_delta)`)
	if err != nil {
		return fmt.Errorf("chstore: scalper replay batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.SignalAt, r.SessionID, r.Symbol, r.Side, r.Score, r.Reason, r.EntryPx, r.TargetPx, r.StopPx,
			r.ExpireAt, r.StepCount, r.BestBidPx, r.BestAskPx, r.Imbalance5, r.PressureDelta,
		); err != nil {
			return fmt.Errorf("chstore: scalper replay append: %w", err)
		}
	}
	return batch.Send()
}

func (c *Client) QueryWSMarketRows(ctx context.Context, symbol string, start, end time.Time, limit int) ([]WSMarketRow, error) {
	if limit <= 0 {
		limit = 50000
	}
	fq := fmt.Sprintf("%s.%s", quoteIdent(c.cfg.Database), quoteIdent(wsMarketTable))
	query := `SELECT ingested_at, exchange_ts, symbol, channel, message_json FROM ` + fq + ` WHERE symbol = ?`
	args := []any{symbol}
	if !start.IsZero() {
		query += ` AND ingested_at >= ?`
		args = append(args, start.UTC())
	}
	if !end.IsZero() {
		query += ` AND ingested_at <= ?`
		args = append(args, end.UTC())
	}
	query += ` ORDER BY ingested_at ASC LIMIT ?`
	args = append(args, limit)
	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chstore: query ws market: %w", err)
	}
	defer rows.Close()
	var out []WSMarketRow
	for rows.Next() {
		var row WSMarketRow
		if err := rows.Scan(&row.IngestedAt, &row.ExchangeTS, &row.Symbol, &row.Channel, &row.MessageRaw); err != nil {
			return nil, fmt.Errorf("chstore: scan ws market: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chstore: ws market rows: %w", err)
	}
	return out, nil
}
