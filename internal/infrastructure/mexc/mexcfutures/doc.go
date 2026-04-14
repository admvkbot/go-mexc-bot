// Package mexcfutures is a Go client for MEXC Futures "WEB session" REST APIs
// and the contract-host WebSocket edge (see ContractWS, same URL and methods
// as mexc-futures-sdk MexcFuturesWebSocket). It merges the surface of the Python
// mexc_client helpers and the TypeScript mexc-futures-sdk REST client.
// Configure the browser WEB token via MEXC_SOURCE_WEB_KEY (see NewClientFromEnv).
//
// Module path: import "github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
package mexcfutures
