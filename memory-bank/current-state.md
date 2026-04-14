# Current state

## What this repository is

A Go module (`github.com/mexc-bot/go-mexc-bot`) with one importable package:

- **`mexcfutures`**: HTTP client for selected MEXC Futures REST endpoints using the **WEB token** from the browser (same class of credentials as the Python/TS references in code comments). Requests set browser-like headers and, where required, `x-mxc-nonce` / `x-mxc-sign` derived from an MD5 chain over the JSON body and the WEB key.

## Layout

| Path | Role |
|------|------|
| `mexcfutures/` | Client, config, signing, env helpers, request types, market/order/account APIs, Python-compat helpers |
| `mexcfutures/compat_test.go` | Unit test for `ParseContractDetailSummary` |
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
- **Compatibility**: `GetOpenPositionsContract`, `GetContractDetailContractPublic`, `ParseContractDetailSummary` align with the older Python client flows.

## Build and test

- Run tests with Go 1.26 (e.g. `GOTOOLCHAIN=go1.26.0 go test ./...` if the local `go` binary is older).

## Git remote (as of last setup)

After `git init` and the first push, the default **`origin`** points to the repository created under the authenticated GitHub account (not `mexc-bot/go-mexc-bot`, which did not exist or was not visible to the token used). Update this file if the remote is changed to match the `go.mod` module path.
