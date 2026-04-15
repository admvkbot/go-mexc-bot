# Active Context

## Current Focus

The active work implemented a V1 execution framework for a book-only scalper on `TAO_USDT`.

## Recently Added

- price corridor entry gate: `BookState.Features` now computes a rolling mean `mid` price over `MEXC_SCALPER_PRICE_CORRIDOR_WINDOW`, then separate upper/lower deviation bands from `MEXC_SCALPER_PRICE_CORRIDOR_PERCENTILE` (default `0.80`); `SignalEngine.Evaluate` keeps the existing order-book signal but only allows entry when the **executed** side is at the matching corridor edge (long at/below lower band, short at/above upper band), rejects entries inside the corridor, and rejects overshoots beyond `MEXC_SCALPER_PRICE_CORRIDOR_MAX_MULTIPLIER` times the band width from the mean. Warmup logs now print `corridor_ready`, `corridor=[lower..upper]`, and `mean`.
- live TAO profile tuning from the last 10m ClickHouse window: `.env` now overrides `MEXC_SCALPER_MAX_SPREAD_TICKS=2`, `MEXC_SCALPER_MAX_SPREAD_TICKS_IN_WINDOW=1.5`, `MEXC_SCALPER_PROFIT_TARGET_TICKS=2`, `MEXC_SCALPER_MIN_SIGNAL_SCORE=2.2`, `MEXC_SCALPER_MIN_IMBALANCE=0.35`, `MEXC_SCALPER_MIN_IMBALANCE_DELTA=0.10`, `MEXC_SCALPER_SIGNAL_CONFIRM_WINDOW=600ms`, `MEXC_SCALPER_SIGNAL_CONFIRM_MIN_AGE=300ms`; rationale: the prior live window showed negative expectancy, many fast stop-outs, and effectively unreachable `TP=3`
- scalper experiment: `MEXC_SCALPER_INVERT_EXECUTION` — journals keep `SignalEngine` side/reason/score; `LiveRuntime` / `ReplayRuntime` open the opposite position and extend ladder by executed side (`SignalEngine` ladder match uses `executionSide`).
- scalper entry gating: `BookState.Features` now computes `MaxRecentSpreadTicks` and signal persistence (`SignalConfirmCount` / `SignalConfirmAge`) from recent snapshots; `SignalEngine.Evaluate` rejects entries when spread was unstable in the recent window, imbalance/pressure side disagrees, the signal is too fresh / not persistent enough, or `microprice` is not aligned / already too extended. New env knobs: `MEXC_SCALPER_SPREAD_STABILITY_WINDOW`, `MEXC_SCALPER_MAX_SPREAD_TICKS_IN_WINDOW`, `MEXC_SCALPER_SIGNAL_CONFIRM_WINDOW`, `MEXC_SCALPER_SIGNAL_CONFIRM_MIN_TICKS`, `MEXC_SCALPER_SIGNAL_CONFIRM_MIN_AGE`, `MEXC_SCALPER_MIN_MICROPRICE_TICKS`, `MEXC_SCALPER_MAX_MICROPRICE_TICKS`.
- scalper analytics: `scalper_signal_event` now stores `ladder_id`, `allow_entry`, `deny_reason`, `confirm_count`, `confirm_ms`, `max_spread_ticks`; live/replay runtimes emit normal decision rows plus explicit `entry_submit` / `ladder_submit` signal rows so roundtrips can be joined to the actual submitted entry context.
- bracket sync: `OrderManager.syncExchangeBracketClose` now mirrors partial exchange-side bracket fills by comparing `open_positions` hold volume vs local `ladder.NetQuantity`, keeps a synthetic exit order with cumulative fill qty/avg px, uses bracket target/stop prices for inferred TP/SL fills, and cancels still-open entry orders after the exchange fully closes the position.
- scalper exit: default `MEXC_SCALPER_EXIT_MODE=bracket` — entry `SubmitOrder` sends `stopLossPrice` / `takeProfitPrice` from `ProfitTargetTicks` / `StopLossTicks`; `EnsureExit` limit path skipped; `RiskGuard` no longer applies software `time_stop` / `stop_loss_ticks` when bracketing (exchange handles); position close detected via throttled `open_positions` + synthetic exit fill (`SyncLadder` now takes exit mark + time). Legacy: `MEXC_SCALPER_EXIT_MODE=limit`.
- scalper: `RiskGuard.AllowEntry` — when `features.VolatilityPause` is already true, return zero `pauseUntil` so `LiveRuntime` does not call `BookState.PauseVolatility` every tick (was pushing `volPauseUntil` forward by `VolatilityPause` forever and blocking all new entries after the first guard trip)
- scalper submit: `NewFromConfig` loads public `GET /contract/detail` into `OrderPriceScale` / `OrderVolScale`; `OrderManager` quantizes limit `price` and `vol` for REST (avoids MEXC code **2015**); env `MEXC_SCALPER_ORDER_PRICE_SCALE` / `MEXC_SCALPER_ORDER_VOL_SCALE` override
- scalper: `SignalEngine.Evaluate` fixed — `CooldownUntil` zero must not imply `DecisionEnter` (was blocking `DecisionAddLadder`); `tick` clears completed ladder (`FlushRoundTrip` + `current=nil`) after `SyncLadder` and before `Evaluate` to avoid overwriting cooldown ladder on same tick
- scalper warmup: `MEXC_SCALPER_WARMUP_PROGRESS` (default `2s`, `0` off) logs `mexc-bot: warmup_progress` lines with bid/ask/mid, spread_ticks, update_rate, imbalance5, signal action/score, `allow_entry` vs cfg thresholds while sampling
- MEXC futures submit responses: if JSON has `success:false` (e.g. code 2005 balance), log `[trade] submit REJECTED ...`, emit `submit_error` / `flatten_error` with raw body, return error from `PlaceEntry`/`PlaceExit`/`EmergencyFlatten`; `placeNewStep` and `RepriceEntries` log and continue without tearing down the WS session
- console `[trade]` logs from `OrderManager` (order lifecycle + cancel-by-external-id + cancel-all on emergency flatten)
- with scalper enabled, `Bot.Run` calls `StartupFlattenOpenPositions` after REST OK (90s timeout): cancel-all + market-close every row from `open_positions` before live scalper WS
- on SIGINT/SIGTERM after `Run` returns `context.Canceled`, `main` calls `Bot.ShutdownFlattenAll` (same flatten logic, `shutdown` log prefix)
- runtime mode parsing in config (`capture`, `scalper`, `replay`)
- extended trading port methods for order submit/cancel/query/positions
- in-memory `BookState` with short-window microstructure features
- `SignalEngine`, `RiskGuard`, `LadderContext`, and `OrderManager`
- live scalper runtime on MEXC contract WS
- replay runtime over historical ClickHouse market rows
- ClickHouse journaling for:
  - signal events
  - order events
  - roundtrip analytics
  - replay candidates

## Live zero-trade window (2026-04-15)

- `scalper_signal_event` (~15m): **no** `reason` rows starting with `book_pressure_*` → `longScore`/`shortScore` never reached `MEXC_SCALPER_MIN_SIGNAL_SCORE` (was **2.2**); dominant `reason` when `allow_entry=1` was **`score_below_threshold`**. Risk denials: mostly **`volatility_pause`**, some **`spread_guard`**.
- `.env` relaxed for flow: `MIN_SIGNAL_SCORE=1.55`, `MIN_IMBALANCE=0.22`, `MIN_IMBALANCE_DELTA=0.07`, `MAX_SPREAD_TICKS_IN_WINDOW=4`, `PRICE_CORRIDOR_MAX_MULTIPLIER=2.0`, `VOLATILITY_PAUSE=2s`, `SIGNAL_CONFIRM_MIN_AGE=180ms`.

## ClickHouse ↔ live timing (2026-04-15)

- 5-minute sample on `futures_ws_market` (TAO_USDT): ~2.6k rows; per-second rates roughly `push.depth` ~4.3/s, `push.depth.full` ~2.8/s, `push.deal` ~1.8/s.
- Merged depth stream (all `push.depth%` by `ingested_at`): median inter-arrival ~147ms (~6.8/s), p99 gap ~472ms, **max gap ~1.52s**.
- Capture uses `MEXC_SOURCE_WEB_KEY` WS → ClickHouse; scalper uses a separate contract WS (`MEXC_WEB_KEY` session). Streams are logically the same market, but **ingest timestamps and burst shapes differ**; tuning must not assume CH rows align tick-for-tick with the scalper session.
- **Desync driver**: default `MaxBookAge` (1s) is shorter than observed **combined** depth gaps (~1.5s), so the scalper often rejects work with `stale_book` while CH still shows a healthy intermittent stream (not a dead feed).
- Mitigation in `.env`: raise `MEXC_SCALPER_MAX_BOOK_AGE` above max gap, widen `FEATURE_LOOKBACK` / `SPREAD_STABILITY_WINDOW` slightly for denser microstructure windows, loosen signal persistence (`SIGNAL_CONFIRM_*`) so confirmations can form within real message spacing.

## Immediate Next Steps

- observe the tightened live profile for at least another 30-60 minutes and compare entry rate / roundtrip expectancy against the pre-tuning 10-minute sample
- decide whether `MEXC_SCALPER_INVERT_EXECUTION=true` should stay global; the last 10-minute window was mixed by signal side, so inversion was left unchanged for now
- preliminary `MEXC_SCALPER_*` defaults were tuned from the sample `TAO_USDT` position history export (typical notional ~8–9k USDT, sub‑5s holds, small tick PnL); override via env as needed
- MEXC keys are split: `MEXC_SOURCE_WEB_KEY` for capture/data, `MEXC_WEB_KEY` for trading/scalper (no cross-fallback)
- validate MEXC order response shapes on a safe live environment
- tune the new entry filters (signal persistence, recent spread stability, microprice bounds) on replay / live samples before tightening defaults further
- confirm that `GetOrderByExternalID` polling is sufficient for reliable fill tracking
- run replay studies on historical TAO data before aggressive live deployment
