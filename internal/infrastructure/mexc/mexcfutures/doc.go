// Package mexcfutures is a Go client for MEXC Futures "WEB session" REST APIs
// and the contract-host WebSocket edge (see ContractWS, same URL and methods
// as mexc-futures-sdk MexcFuturesWebSocket). It merges the surface of the Python
// mexc_client helpers and the TypeScript mexc-futures-sdk REST client.
// Configure trading REST with MEXC_WEB_KEY; market capture uses MEXC_SOURCE_WEB_KEY at the app layer (see internal/config).
//
// Module path: import "github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
package mexcfutures
