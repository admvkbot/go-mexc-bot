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
