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

1. MEXC WS pushes `push.depth` and `push.depth.full`; optional `push.deal` when deal-tape entry filter is enabled
2. `BookState` maintains an in-memory top-of-book snapshot and optional rolling signed deal volume
3. `SignalEngine` scores book pressure using **step-to-step** deltas (penultimate snapshot → current) plus microprice (score contribution optionally capped in ticks)
4. `SignalEngine` applies entry-quality gates: signal persistence, spread stability, side consistency, microprice reject band, optional deal-tape alignment, and a rolling price-corridor gate on mean `mid` with percentile bands
5. `RiskGuard` blocks entry or escalates flatten conditions (shorter volatility extension after `stale_book` than after spread/chaos when configured)
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
