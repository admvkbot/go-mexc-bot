package chstore

import (
	"context"
	"fmt"
	"strings"
)

const (
	wsDepthTopTable   = "futures_depth_top"
	wsDealTickTable   = "futures_deal_tick"
	wsHourlyStatsTbl  = "futures_ws_hourly_stats"
	wsHourlyStatsMV   = "mv_futures_ws_hourly_stats"
)

func (c *Client) createNormalizedTables(ctx context.Context) error {
	dbQ := quoteIdent(c.cfg.Database)

	depthDDL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s
(
    ingested_at DateTime64(3),
    exchange_ts_ms Int64,
    symbol LowCardinality(String),
    channel LowCardinality(String),
    version Int64,
    begin Int64,
    end Int64,
    best_bid_px Float64,
    best_bid_vol Float64,
    best_ask_px Float64,
    best_ask_vol Float64,
    mid Float64,
    spread Float64,
    bid_vol5 Float64,
    ask_vol5 Float64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(ingested_at)
ORDER BY (symbol, channel, exchange_ts_ms, ingested_at)`, dbQ, quoteIdent(wsDepthTopTable))

	dealDDL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s
(
    ingested_at DateTime64(3),
    exchange_ts_ms Int64,
    symbol LowCardinality(String),
    price Float64,
    vol Float64,
    side_code Int32,
    trade_ts_ms Int64
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(ingested_at)
ORDER BY (symbol, exchange_ts_ms, trade_ts_ms, ingested_at)`, dbQ, quoteIdent(wsDealTickTable))

	hourlyDDL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s
(
    hour DateTime,
    symbol LowCardinality(String),
    channel LowCardinality(String),
    cnt UInt64
)
ENGINE = SummingMergeTree
PARTITION BY toYYYYMM(hour)
ORDER BY (symbol, channel, hour)`, dbQ, quoteIdent(wsHourlyStatsTbl))

	mvDDL := fmt.Sprintf(`
CREATE MATERIALIZED VIEW IF NOT EXISTS %s.%s TO %s.%s AS
SELECT
    toStartOfHour(ingested_at) AS hour,
    symbol,
    channel,
    toUInt64(1) AS cnt
FROM %s.%s`, dbQ, quoteIdent(wsHourlyStatsMV), dbQ, quoteIdent(wsHourlyStatsTbl), dbQ, quoteIdent(wsMarketTable))

	for _, q := range []string{depthDDL, dealDDL, hourlyDDL, mvDDL} {
		if err := c.conn.Exec(ctx, q); err != nil {
			return fmt.Errorf("chstore: ddl normalized: %w", err)
		}
	}
	return nil
}

// insertNormalizedFromWSRows parses raw WS frames and inserts into futures_depth_top / futures_deal_tick.
func (c *Client) insertNormalizedFromWSRows(ctx context.Context, rows []WSMarketRow) error {
	var depthRows []DepthTopRow
	var dealRows []DealTickRow
	for _, r := range rows {
		ch := strings.TrimSpace(r.Channel)
		switch {
		case ch == "push.depth" || strings.HasPrefix(ch, "push.depth.full"):
			if d, ok := ParseDepthTopFromMessageJSON(r.MessageRaw, r.Symbol, ch, r.IngestedAt); ok {
				depthRows = append(depthRows, d)
			}
		case ch == "push.deal":
			dealRows = append(dealRows, ParseDealTicksFromMessageJSON(r.MessageRaw, r.Symbol, r.IngestedAt)...)
		}
	}
	if err := c.insertDepthTop(ctx, depthRows); err != nil {
		return err
	}
	return c.insertDealTicks(ctx, dealRows)
}

func (c *Client) insertDepthTop(ctx context.Context, rows []DepthTopRow) error {
	if len(rows) == 0 {
		return nil
	}
	fq := fmt.Sprintf("%s.%s", quoteIdent(c.cfg.Database), quoteIdent(wsDepthTopTable))
	batch, err := c.conn.PrepareBatch(ctx, `INSERT INTO `+fq+` (
ingested_at, exchange_ts_ms, symbol, channel, version, begin, end,
best_bid_px, best_bid_vol, best_ask_px, best_ask_vol, mid, spread, bid_vol5, ask_vol5)`)
	if err != nil {
		return fmt.Errorf("chstore: depth batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.IngestedAt, r.ExchangeTSMs, r.Symbol, r.Channel, r.Version, r.Begin, r.End,
			r.BestBidPx, r.BestBidVol, r.BestAskPx, r.BestAskVol, r.Mid, r.Spread, r.BidVol5, r.AskVol5,
		); err != nil {
			return fmt.Errorf("chstore: depth append: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("chstore: depth send: %w", err)
	}
	return nil
}

func (c *Client) insertDealTicks(ctx context.Context, rows []DealTickRow) error {
	if len(rows) == 0 {
		return nil
	}
	fq := fmt.Sprintf("%s.%s", quoteIdent(c.cfg.Database), quoteIdent(wsDealTickTable))
	batch, err := c.conn.PrepareBatch(ctx, `INSERT INTO `+fq+` (
ingested_at, exchange_ts_ms, symbol, price, vol, side_code, trade_ts_ms)`)
	if err != nil {
		return fmt.Errorf("chstore: deal batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.IngestedAt, r.ExchangeTSMs, r.Symbol, r.Price, r.Vol, r.SideCode, r.TradeTSMs,
		); err != nil {
			return fmt.Errorf("chstore: deal append: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("chstore: deal send: %w", err)
	}
	return nil
}
