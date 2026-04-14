# Tech Context

## Stack

- Go 1.26
- MEXC Futures browser-session-authenticated REST client
- MEXC contract WebSocket client
- ClickHouse native protocol via `clickhouse-go/v2`
- Docker Compose for local bot + ClickHouse setup

## Key Runtime Inputs

- `MEXC_SOURCE_WEB_KEY`: browser WEB token for **market capture** (data plane)
- `MEXC_WEB_KEY`: browser WEB token for **trading** (private REST / scalper)
- `MEXC_WS_SYMBOLS`: symbols for market capture
- `MEXC_BOT_MODE`: `capture`, `scalper`, `replay`, or combined `capture,scalper`
- `MEXC_SCALPER_*`: execution, signal, risk, and replay settings
- `CLICKHOUSE_*`: ClickHouse connection settings

## Important Current Constraints

- Live scalper V1 is single-symbol by design
- Signal generation is book-only; public deal flow is not used for entry decisions
- Order state is tracked via REST polling rather than private order WebSocket streams
- Replay consumes stored public WS market rows from ClickHouse

## Testing Notes

- `go test ./...` can fail in this workspace because `db/clickhouse` contains permission-restricted files
- Use `GOTOOLCHAIN=go1.26.0 go test ./cmd/... ./internal/...` for code validation
