# Progress

## Working

- Public MEXC order book and deal capture into ClickHouse
- Normalized market-data tables and analytical views
- MEXC REST order submission/cancel/query primitives
- Live book-only scalper runtime scaffold
- Replay runtime using historical ClickHouse market rows
- ClickHouse trade journal for signals, order lifecycle, and roundtrip outcomes

## Still Needs Validation

- real MEXC response compatibility for all live execution branches
- production-safe repricing behavior under fast order-book churn
- emergency flatten behavior during slippage spikes
- replay threshold tuning against historical TAO market conditions

## Known Risks

- order tracking relies on REST polling, not private order pushes
- emergency market flatten can still slip badly during violent moves
- single-symbol V1 keeps complexity controlled but limits portfolio flexibility
