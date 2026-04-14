# Active Context

## Current Focus

The active work implemented a V1 execution framework for a book-only scalper on `TAO_USDT`.

## Recently Added

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

## Immediate Next Steps

- preliminary `MEXC_SCALPER_*` defaults were tuned from the sample `TAO_USDT` position history export (typical notional ~8–9k USDT, sub‑5s holds, small tick PnL); override via env as needed
- validate MEXC order response shapes on a safe live environment
- tune thresholds for imbalance, pressure delta, repricing TTLs, and stop/target ticks
- confirm that `GetOrderByExternalID` polling is sufficient for reliable fill tracking
- run replay studies on historical TAO data before aggressive live deployment
