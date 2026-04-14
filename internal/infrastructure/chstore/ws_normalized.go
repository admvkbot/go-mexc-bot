package chstore

import (
	"context"
	"fmt"
	"strings"
)

const (
	wsDepthTopTable   = "futures_depth_top"
	wsDealTickTable   = "futures_deal_tick"
	wsBookLevelTable  = "futures_book_level"
	wsHourlyStatsTbl  = "futures_ws_hourly_stats"
	wsHourlyStatsMV   = "mv_futures_ws_hourly_stats"
	wsDepth1sView     = "v_futures_depth_1s"
	wsDeal1sView      = "v_futures_deal_1s"
	wsSignal1sView    = "v_futures_signal_1s"
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

	bookDDL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s
(
    ingested_at DateTime64(3),
    exchange_ts_ms Int64,
    symbol LowCardinality(String),
    channel LowCardinality(String),
    version Int64,
    begin Int64,
    end Int64,
    side Enum8('bid' = 1, 'ask' = 2),
    level_rank UInt8,
    price Float64,
    vol Float64,
    orders Int32,
    is_delete UInt8
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(ingested_at)
ORDER BY (symbol, channel, side, exchange_ts_ms, level_rank, price, ingested_at)`, dbQ, quoteIdent(wsBookLevelTable))

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

	depthViewDDL := fmt.Sprintf(`
CREATE OR REPLACE VIEW %s.%s AS
SELECT
    toStartOfSecond(ingested_at) AS ts_1s,
    symbol,
    count() AS depth_updates,
    argMax(best_bid_px, exchange_ts_ms) AS best_bid_last,
    argMax(best_ask_px, exchange_ts_ms) AS best_ask_last,
    argMax(mid, exchange_ts_ms) AS mid_last,
    argMax(spread, exchange_ts_ms) AS spread_last,
    avg(spread) AS spread_avg,
    min(spread) AS spread_min,
    max(spread) AS spread_max,
    avg(bid_vol5) AS bid_vol5_avg,
    avg(ask_vol5) AS ask_vol5_avg,
    avg(if((bid_vol5 + ask_vol5) = 0, 0, (bid_vol5 - ask_vol5) / (bid_vol5 + ask_vol5))) AS imbalance5_avg
FROM %s.%s
WHERE best_bid_px > 0 AND best_ask_px > 0
GROUP BY ts_1s, symbol`, dbQ, quoteIdent(wsDepth1sView), dbQ, quoteIdent(wsDepthTopTable))

	dealViewDDL := fmt.Sprintf(`
CREATE OR REPLACE VIEW %s.%s AS
SELECT
    toStartOfSecond(ingested_at) AS ts_1s,
    symbol,
    count() AS deal_count,
    sum(vol) AS deal_vol_sum,
    avg(vol) AS deal_vol_avg,
    max(vol) AS deal_vol_max,
    sumIf(vol, side_code = 1) AS buy_vol_sum,
    sumIf(vol, side_code = 2) AS sell_vol_sum,
    sumIf(1, side_code = 1) AS buy_count,
    sumIf(1, side_code = 2) AS sell_count
FROM %s.%s
GROUP BY ts_1s, symbol`, dbQ, quoteIdent(wsDeal1sView), dbQ, quoteIdent(wsDealTickTable))

	signalViewDDL := fmt.Sprintf(`
CREATE OR REPLACE VIEW %s.%s AS
SELECT
    d.ts_1s,
    d.symbol,
    d.depth_updates,
    d.best_bid_last,
    d.best_ask_last,
    d.mid_last,
    d.spread_last,
    d.spread_avg,
    d.spread_min,
    d.spread_max,
    d.bid_vol5_avg,
    d.ask_vol5_avg,
    d.imbalance5_avg,
    if((d.bid_vol5_avg + d.ask_vol5_avg) = 0, 0, ((d.best_ask_last * d.bid_vol5_avg) + (d.best_bid_last * d.ask_vol5_avg)) / (d.bid_vol5_avg + d.ask_vol5_avg) - d.mid_last) AS microprice_delta,
    ifNull(t.deal_count, 0) AS deal_count,
    ifNull(t.deal_vol_sum, 0) AS deal_vol_sum,
    ifNull(t.deal_vol_avg, 0) AS deal_vol_avg,
    ifNull(t.deal_vol_max, 0) AS deal_vol_max,
    ifNull(t.buy_vol_sum, 0) AS buy_vol_sum,
    ifNull(t.sell_vol_sum, 0) AS sell_vol_sum,
    ifNull(t.buy_count, 0) AS buy_count,
    ifNull(t.sell_count, 0) AS sell_count,
    ifNull(t.buy_vol_sum, 0) - ifNull(t.sell_vol_sum, 0) AS deal_vol_delta,
    ifNull(t.buy_count, 0) - ifNull(t.sell_count, 0) AS deal_count_delta
FROM %s.%s AS d
LEFT JOIN %s.%s AS t
ON d.ts_1s = t.ts_1s AND d.symbol = t.symbol`, dbQ, quoteIdent(wsSignal1sView), dbQ, quoteIdent(wsDepth1sView), dbQ, quoteIdent(wsDeal1sView))

	for _, q := range []string{depthDDL, dealDDL, bookDDL, hourlyDDL, mvDDL, depthViewDDL, dealViewDDL, signalViewDDL} {
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
	var bookRows []BookLevelRow
	for _, r := range rows {
		ch := strings.TrimSpace(r.Channel)
		switch {
		case ch == "push.depth" || strings.HasPrefix(ch, "push.depth.full"):
			if d, ok := ParseDepthTopFromMessageJSON(r.MessageRaw, r.Symbol, ch, r.IngestedAt); ok {
				depthRows = append(depthRows, d)
			}
			bookRows = append(bookRows, ParseBookLevelsFromMessageJSON(r.MessageRaw, r.Symbol, ch, r.IngestedAt, 20)...)
		case ch == "push.deal":
			dealRows = append(dealRows, ParseDealTicksFromMessageJSON(r.MessageRaw, r.Symbol, r.IngestedAt)...)
		}
	}
	if err := c.insertDepthTop(ctx, depthRows); err != nil {
		return err
	}
	if err := c.insertBookLevels(ctx, bookRows); err != nil {
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

func (c *Client) insertBookLevels(ctx context.Context, rows []BookLevelRow) error {
	if len(rows) == 0 {
		return nil
	}
	fq := fmt.Sprintf("%s.%s", quoteIdent(c.cfg.Database), quoteIdent(wsBookLevelTable))
	batch, err := c.conn.PrepareBatch(ctx, `INSERT INTO `+fq+` (
ingested_at, exchange_ts_ms, symbol, channel, version, begin, end, side, level_rank, price, vol, orders, is_delete)`)
	if err != nil {
		return fmt.Errorf("chstore: book batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(
			r.IngestedAt, r.ExchangeTSMs, r.Symbol, r.Channel, r.Version, r.Begin, r.End, r.Side, r.LevelRank, r.Price, r.Vol, r.Orders, r.IsDelete,
		); err != nil {
			return fmt.Errorf("chstore: book append: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("chstore: book send: %w", err)
	}
	return nil
}
