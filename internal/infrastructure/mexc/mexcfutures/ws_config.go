package mexcfutures

import (
	"time"

	"github.com/gorilla/websocket"
)

// DefaultContractWSURL is the native MEXC contract WebSocket endpoint (same as
// mexc-futures-sdk MexcFuturesWebSocket).
const DefaultContractWSURL = "wss://contract.mexc.com/edge"

const defaultWSPingInterval = 15 * time.Second

// WSConfig configures ContractWS (contract host edge socket).
// APIKey and SecretKey are only required for Login; public channels work without them.
type WSConfig struct {
	URL string

	// APIKey and SecretKey are MEXC API management credentials (HMAC login on WS).
	APIKey    string
	SecretKey string

	// PingInterval defaults to 15s if zero (MEXC recommends ping every 10–20s).
	PingInterval time.Duration

	Dialer *websocket.Dialer
}

func (c WSConfig) wsURL() string {
	if c.URL != "" {
		return c.URL
	}
	return DefaultContractWSURL
}

func (c WSConfig) pingInterval() time.Duration {
	if c.PingInterval > 0 {
		return c.PingInterval
	}
	return defaultWSPingInterval
}

func (c WSConfig) dialer() *websocket.Dialer {
	if c.Dialer != nil {
		return c.Dialer
	}
	return websocket.DefaultDialer
}
