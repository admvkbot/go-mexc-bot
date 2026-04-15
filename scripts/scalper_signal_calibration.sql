-- Offline helpers: approximate bot score drivers from ClickHouse (TAO_USDT example).
-- Adjust symbol / time window. Requires futures_depth_top + optional v_futures_signal_1s.

-- 1) Per-row imbalance5 and microprice_delta (same formulas as BookState / v_futures_signal_1s)
WITH d AS (
    SELECT
        ingested_at,
        symbol,
        best_bid_px,
        best_ask_px,
        mid,
        spread,
        bid_vol5,
        ask_vol5,
        if((bid_vol5 + ask_vol5) = 0, 0.0, (bid_vol5 - ask_vol5) / (bid_vol5 + ask_vol5)) AS imb5,
        if((bid_vol5 + ask_vol5) = 0, 0.0,
           ((best_ask_px * bid_vol5) + (best_bid_px * ask_vol5)) / (bid_vol5 + ask_vol5) - mid
        ) AS microprice_delta
    FROM mexc_bot.futures_depth_top
    WHERE symbol = 'TAO_USDT'
      AND ingested_at > now() - INTERVAL 1 HOUR
      AND best_bid_px > 0 AND best_ask_px > 0
)
SELECT
    quantile(0.5)(abs(imb5)) AS q50_abs_imb5,
    quantile(0.9)(abs(imb5)) AS q90_abs_imb5,
    quantile(0.5)(abs(microprice_delta)) AS q50_abs_micro,
    quantile(0.9)(abs(microprice_delta)) AS q90_abs_micro
FROM d;

-- 2) Event-to-event Δimb5 (lag one row by time)
SELECT
    quantile(0.5)(abs(imb5 - prev_imb5)) AS q50_abs_dimb,
    quantile(0.9)(abs(imb5 - prev_imb5)) AS q90_abs_dimb
FROM (
    SELECT
        imb5,
        lagInFrame(imb5) OVER (ORDER BY ingested_at) AS prev_imb5
    FROM (
        SELECT
            ingested_at,
            if((bid_vol5 + ask_vol5) = 0, 0.0, (bid_vol5 - ask_vol5) / (bid_vol5 + ask_vol5)) AS imb5
        FROM mexc_bot.futures_depth_top
        WHERE symbol = 'TAO_USDT'
          AND ingested_at > now() - INTERVAL 1 HOUR
          AND best_bid_px > 0 AND best_ask_px > 0
    )
)
WHERE prev_imb5 IS NOT NULL;

-- 3) Join 1s book + tape (replay calibration): compare deal_vol_delta sign to imb5_avg
SELECT
    toStartOfMinute(ts_1s) AS m,
    countIf(sign(imbalance5_avg) = sign(deal_vol_delta) AND deal_vol_delta != 0) AS agree_rows,
    countIf(deal_vol_delta != 0) AS nonzero_delta_rows,
    count() AS rows
FROM mexc_bot.v_futures_signal_1s
WHERE symbol = 'TAO_USDT'
  AND ts_1s > now() - INTERVAL 6 HOUR
GROUP BY m
ORDER BY m DESC
LIMIT 120;

-- 4) Коридор цены: сколько снимков depth попадает в 15s (как окно MEXC_SCALPER_PRICE_CORRIDOR_WINDOW) и плотность по времени
--    → MEXC_SCALPER_PRICE_CORRIDOR_MIN_SAMPLES (держать существенно ниже q05..q10, иначе частые price_range_unavailable)
--    → MEXC_SCALPER_PRICE_CORRIDOR_MEAN_HALF_LIFE (порядок ~10–25× p50_dt_ms даёт умеренный упор на свежие тики внутри окна)
SELECT
    round(quantile(0.05)(cnt), 1) AS q05_rows_per_15s,
    round(quantile(0.1)(cnt), 1) AS q10_rows_per_15s,
    round(quantile(0.5)(cnt), 1) AS q50_rows_per_15s,
    round(quantile(0.9)(cnt), 1) AS q90_rows_per_15s,
    min(cnt) AS min_rows_per_15s,
    count() AS buckets
FROM (
    SELECT
        toStartOfInterval(ingested_at, INTERVAL 15 SECOND) AS t15,
        count() AS cnt
    FROM mexc_bot.futures_depth_top
    WHERE symbol = 'TAO_USDT'
      AND ingested_at > now() - INTERVAL 24 HOUR
    GROUP BY t15
);
SELECT
    quantile(0.5)(dt_ms) AS p50_ms_between_depth_rows,
    quantile(0.9)(dt_ms) AS p90_ms_between_depth_rows
FROM (
    SELECT dateDiff('millisecond', lagInFrame(ingested_at) OVER (ORDER BY ingested_at), ingested_at) AS dt_ms
    FROM mexc_bot.futures_depth_top
    WHERE symbol = 'TAO_USDT'
      AND ingested_at > now() - INTERVAL 24 HOUR
)
WHERE dt_ms > 0 AND dt_ms < 120000;
