# Project history

## Origin

The repository started as a small Go module exposing package `mexcfutures`: a REST client for MEXC Futures APIs that mimic browser session authentication (WEB cookie / `authorization` header), not official API-key HMAC signing.

The design merges behaviors seen in:

- Python helpers (`mexc_client` style), including `POST /private/order/create` and signed `GET` on `contract.mexc.com` with an empty JSON object for signing.
- TypeScript `mexc-futures-sdk` style paths such as `POST /private/order/submit` and related private order/account/position endpoints on `futures.mexc.com`.

## Tooling milestones

- **Go toolchain**: The module was raised to **Go 1.26** with an explicit `toolchain go1.26.0` directive so builds resolve the same compiler version via the Go toolchain mechanism.
- **Dependencies**: `github.com/joho/godotenv` for optional `.env` loading; `github.com/gorilla/websocket` for the contract-edge WebSocket client (`ContractWS`), mirroring `mexc-futures-sdk` `websocket.ts`.
- **Version control**: Git was initialized in the workspace when the directory was not yet a repository; the first commit captured the library, tests, ignore rules, documentation, and memory bank snapshot.

## GitHub remote

The repository `https://github.com/mexc-bot/go-mexc-bot` was not available to the token used (GraphQL “Could not resolve”). A new public repository was created as **`https://github.com/admvkbot/go-mexc-bot`** and `main` was pushed as the first upstream branch. The Go module import path in `go.mod` was left unchanged.

## Layout: bot-oriented layers

The root package `mexcfutures/` was moved under `internal/infrastructure/mexc/mexcfutures/` so the client is an implementation detail. Added `cmd/mexc-bot`, `internal/app`, `internal/config`, and `internal/ports` for a conventional Go service layout and future use-case/domain packages.
