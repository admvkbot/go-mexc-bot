# System Patterns

## Layering

- `cmd/mexc-bot`: process entrypoint
- `internal/app`: orchestration, runtime selection, lifecycle
- `internal/config`: env-driven runtime settings
- `internal/ports`: application-facing trading interface
- `internal/infrastructure/mexc/mexcfutures`: MEXC REST and WebSocket client
- `internal/infrastructure/chstore`: ClickHouse schema and writers
- `internal/infrastructure/chscalper`: adapter from scalper events to ClickHouse rows
- `internal/scalper`: strategy, book state, risk guard, state machine, order manager, replay/live runtime

## Runtime Separation

The repository now follows a runtime-split pattern:
- `capture` mode writes public market data to ClickHouse
- `scalper` mode consumes live order-book updates and trades
- `replay` mode replays historical ClickHouse market rows through the same signal engine

This separation keeps the data ingestion path independent from execution logic.

## Scalper Flow

1. MEXC WS pushes `push.depth` and `push.depth.full`
2. `BookState` maintains an in-memory top-of-book snapshot
3. `SignalEngine` calculates book-only momentum/imbalance decisions
4. `SignalEngine` now also applies entry-quality gates from recent book history: signal persistence, spread stability, side consistency, microprice alignment, and a rolling price-corridor gate based on mean `mid` price plus separate upper/lower percentile deviations
5. `RiskGuard` blocks entry or escalates flatten conditions
6. `OrderManager` submits, reprices, cancels, and emergency-flattens orders; in `bracket` mode it reconciles local state against `open_positions` deltas instead of only waiting for full position disappearance
7. `chscalper.Writer` journals signals, order events, replay candidates, and roundtrips; signal rows now include `ladder_id`, allow/deny outcome, and an explicit `entry_submit` marker for actual entry correlation

## State Pattern

The scalper uses a single active ladder context per symbol:
- `idle`
- `entry_pending`
- `partially_filled`
- `inventory_open`
- `exit_pending`
- `emergency_flatten`
- `cooldown`

The intended V1 behavior is one ladder context at a time with multiple controlled entry steps.
