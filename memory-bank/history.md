# Project history

## Origin

The repository started as a small Go module exposing package `mexcfutures`: a REST client for MEXC Futures APIs that mimic browser session authentication (WEB cookie / `authorization` header), not official API-key HMAC signing.

The design merges behaviors seen in:

- Python helpers (`mexc_client` style), including `POST /private/order/create` and signed `GET` on `contract.mexc.com` with an empty JSON object for signing.
- TypeScript `mexc-futures-sdk` style paths such as `POST /private/order/submit` and related private order/account/position endpoints on `futures.mexc.com`.

## Tooling milestones

- **Go toolchain**: The module was raised to **Go 1.26** with an explicit `toolchain go1.26.0` directive so builds resolve the same compiler version via the Go toolchain mechanism.
- **Dependencies**: The only third-party dependency is `github.com/joho/godotenv` for optional `.env` loading; versions were refreshed using Context7 documentation as reference for install and usage patterns.
- **Version control**: Git was initialized in the workspace when the directory was not yet a repository; the first commit captured the library, tests, ignore rules, documentation, and memory bank snapshot.

## GitHub remote

The canonical GitHub remote used for the initial push was created under the authenticated GitHub user because the organization/name `mexc-bot/go-mexc-bot` did not resolve as an existing repository at push time. See `current-state.md` for the exact remote URL.
