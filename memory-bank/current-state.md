# Current state

## What this repository is

A Go module (`github.com/mexc-bot/go-mexc-bot`) for a **MEXC bot** with layered layout:

- **`cmd/mexc-bot`**: process entrypoint (signal handling, config load, `app.Bot.Run`).
- **`internal/app`**: composition / lifecycle; depends on `ports` and `config`, constructs `mexcfutures.Client`.
- **`internal/config`**: `MEXC_SOURCE_WEB_KEY`, `MEXC_WS_SYMBOLS` (and legacy `MEXC_WS_SYMBOL`), ClickHouse settings via env (`chstore.ConfigFromEnv`), optional `.env`.
- **`internal/ports`**: small interfaces (`FuturesREST`) for testability and future use cases.
- **`internal/infrastructure/mexc/mexcfutures`**: HTTP client for selected MEXC Futures REST endpoints using the **WEB token** from the browser, plus **contract WebSocket** (`ContractWS` on `wss://contract.mexc.com/edge`). REST requests set browser-like headers and, where required, `x-mxc-nonce` / `x-mxc-sign` derived from an MD5 chain over the JSON body and the WEB key.

The `mexcfutures` package is **not** importable from outside this module (under `internal/`).

## Layout

| Path | Role |
|------|------|
| `cmd/mexc-bot/main.go` | Loads `internal/config`, builds `internal/app.Bot`, runs until SIGINT/SIGTERM |
| `internal/app/bot.go` | `Bot`, `New`, `NewFromConfig`, `Run` (REST check, then WS market capture → ClickHouse until shutdown) |
| `internal/app/ws_market_clickhouse.go` | Contract WS per symbol in `MEXC_WS_SYMBOLS`: `sub.depth`, `sub.depth.full` 5/10/20, `sub.deal`; persists `push.depth*`, `push.deal` JSON to ClickHouse |
| `internal/infrastructure/chstore` | ClickHouse native client: raw `futures_ws_market`; normalized `futures_depth_top`, `futures_deal_tick`, `futures_book_level`; MV `mv_futures_ws_hourly_stats` → `futures_ws_hourly_stats`; analytical views `v_futures_depth_1s`, `v_futures_deal_1s`, `v_futures_signal_1s`; dual-write on flush after parsing WS JSON |
| `internal/config/config.go` | `Bot`, `Load()`, `ParseWSSymbols()` |
| `internal/ports/futures_rest.go` | `FuturesREST` interface |
| `internal/infrastructure/mexc/mexcfutures/` | REST client, `ContractWS`, signing, env helpers, types, market/order/account, Python-compat helpers |
| `internal/infrastructure/mexc/mexcfutures/compat_test.go` | Unit test for `ParseContractDetailSummary` |
| `internal/infrastructure/mexc/mexcfutures/open_positions_integration_test.go` | Integration test with `MEXC_SOURCE_WEB_KEY` (loads `../../../../.env` toward repo root) |
| `internal/infrastructure/mexc/mexcfutures/contract_depth_integration_test.go` | Integration test: public `ContractDepth` for `TAO_USDT` |
| `internal/infrastructure/mexc/mexcfutures/ws_config.go`, `ws_client.go` | Contract WS |
| `internal/infrastructure/mexc/mexcfutures/ws_client_integration_test.go` | Integration test: `sub.depth` / `push.depth` for `TAO_USDT` |
| `docs/` | Human-oriented documentation of MEXC-related capabilities and usage |
| `memory-bank/` | Historical and situational notes (no roadmap) |

## Authentication and configuration

- Environment variable: **`MEXC_SOURCE_WEB_KEY`** — value of the MEXC browser WEB cookie string used as `Authorization`.
- Optional: `.env` in the working directory is loaded when using `WebKeyFromEnv(true)` or `NewClientFromEnv()` (errors from a missing file are ignored in `LoadDotEnv`).

## API surface (high level)

- **Hosts**: Default bases are `https://futures.mexc.com/api/v1` and `https://contract.mexc.com/api/v1`; both are overridable via `Config`.
- **Orders**: Submit/cancel/history/deals/get-by-id helpers on the futures host; some bodies mirror TS or Python shapes.
- **Account / positions**: Risk limit, fee rate, asset by currency, open positions, position history.
- **Market**: Public ticker, contract detail, depth; `TestConnection` uses `BTC_USDT` ticker.
- **WebSocket (contract edge)**: `ContractWS` — same URL and `sub.depth` / `push.depth` (etc.) as `mexc-futures-sdk` `MexcFuturesWebSocket`; optional HMAC `Login` for private streams.
- **Compatibility**: `GetOpenPositionsContract`, `GetContractDetailContractPublic`, `ParseContractDetailSummary` align with the older Python client flows.

## Build and test

- Run tests with Go 1.26 (e.g. `GOTOOLCHAIN=go1.26.0 go test ./...` if the local `go` binary is older).

## Docker

- **`Dockerfile`**: multi-stage build of `cmd/mexc-bot`, runtime image `distroless/static` (nonroot).
- **`docker-compose.yml`**: `mexc-bot` and **`clickhouse`** both use **`env_file: .env`** for ClickHouse credentials (`CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`, `CLICKHOUSE_DATABASE`) and **`MEXC_WS_SYMBOLS`**. The ClickHouse service maps **`CLICKHOUSE_*`** into the image’s **`CLICKHOUSE_DB` / `CLICKHOUSE_USER` / `CLICKHOUSE_PASSWORD`**. `mexc-bot` overrides **`CLICKHOUSE_ADDR=clickhouse:9000`** (in-cluster TCP). Host ports: HTTP **127.0.0.1:8123**, native **127.0.0.1:19000→9000** (avoids clashes with another process on host **9000**). Data: **`db/clickhouse`**. Bot appends to **`mexc_bot.futures_ws_market`**.
- **`.env.example`**: placeholder keys (empty `MEXC_SOURCE_WEB_KEY`, ClickHouse user/password, default symbol list).
- **`.gitignore`**: `db/clickhouse/` is ignored (local DB files).

## Git remote (as of last setup)

After `git init` and the first push, the default **`origin`** is:

- **HTTPS**: `https://github.com/admvkbot/go-mexc-bot.git`
- **Web**: `https://github.com/admvkbot/go-mexc-bot`

The `go.mod` module path remains `github.com/mexc-bot/go-mexc-bot` (vanity path); the GitHub organization `mexc-bot` did not resolve to an existing repository for the authenticated account at setup time. Update this file if the remote is repointed.
