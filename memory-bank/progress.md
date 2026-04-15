# Progress

## Working

- live/replay entry selection now requires both the existing order-book signal and a matching price-corridor location: no entries inside the corridor, no entries beyond the configured `x2` overshoot, and side matching is based on executed side so `MEXC_SCALPER_INVERT_EXECUTION` stays coherent
- live `.env` profile for `TAO_USDT` was tightened from a fresh 10-minute ClickHouse sample: stricter spread / persistence / score / imbalance filters plus `ProfitTargetTicks=2`; bot restarted successfully and new thresholds are visible in warmup/live logs
- `bracket` mode now syncs partial exchange-side closes into the local ladder state and cancels stale entry orders after a full exchange-side close
- `scalper_signal_event` now records enough context to join roundtrips to the actual submitted entry (`ladder_id`, `allow_entry`, `deny_reason`, confirmation/spread metrics, explicit `entry_submit` rows)
- live/replay entry selection now includes recent-spread stability, signal persistence, side-consistency, and microprice alignment filters
- Live scalper: volatility pause no longer self-extends on every book tick after `RiskGuard.AllowEntry` fix (entries resume after configured `MEXC_SCALPER_VOLATILITY_PAUSE`)
- Public MEXC order book and deal capture into ClickHouse
- Normalized market-data tables and analytical views
- MEXC REST order submission/cancel/query primitives
- Live book-only scalper runtime scaffold
- Replay runtime using historical ClickHouse market rows
- ClickHouse trade journal for signals, order lifecycle, and roundtrip outcomes

## Still Needs Validation

- live profitability after adding the price-corridor gate; runtime/warmup already show corridor metrics, but a longer sample is still needed for expectancy
- whether the tightened `.env` profile improves expectancy after enough live samples; the first post-restart window showed zero entries during warmup / immediate startup, which confirms filtering but not profitability yet
- real MEXC response compatibility for all live execution branches
- inferred bracket fill price still uses bracket target/stop levels or current mark; it is more stable than before but still not true exchange execution history
- production-safe repricing behavior under fast order-book churn
- emergency flatten behavior during slippage spikes
- replay threshold tuning against historical TAO market conditions

## Known Risks

- order tracking relies on REST polling, not private order pushes
- emergency market flatten can still slip badly during violent moves
- single-symbol V1 keeps complexity controlled but limits portfolio flexibility
