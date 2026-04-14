package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// WebKeyEnv is the environment variable holding the MEXC browser WEB session string.
const WebKeyEnv = "MEXC_SOURCE_WEB_KEY"

// WSSymbolEnv is a single-symbol fallback for public WebSocket market capture (deprecated in favour of MEXC_WS_SYMBOLS).
const WSSymbolEnv = "MEXC_WS_SYMBOL"

// MEXCWSymbolsEnv is a comma-separated list of futures contract symbols for WS market capture (order book + deals).
const MEXCWSymbolsEnv = "MEXC_WS_SYMBOLS"

// Bot holds runtime settings for the trading bot process.
type Bot struct {
	WebKey    string
	WSSymbols []string
}

// Load reads optional .env from the working directory and returns Bot configuration.
func Load() (Bot, error) {
	_ = godotenv.Load()
	k := strings.TrimSpace(os.Getenv(WebKeyEnv))
	if k == "" {
		return Bot{}, fmt.Errorf("config: %s is not set", WebKeyEnv)
	}
	return Bot{WebKey: k, WSSymbols: ParseWSSymbols()}, nil
}

// ParseWSSymbols reads MEXC_WS_SYMBOLS (comma-separated), then legacy MEXC_WS_SYMBOL, defaulting to TAO_USDT.
func ParseWSSymbols() []string {
	raw := strings.TrimSpace(os.Getenv(MEXCWSymbolsEnv))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv(WSSymbolEnv))
	}
	if raw == "" {
		return []string{"TAO_USDT"}
	}
	seen := make(map[string]struct{})
	var out []string
	for _, part := range strings.Split(raw, ",") {
		s := strings.TrimSpace(part)
		if s == "" {
			continue
		}
		key := strings.ToUpper(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{"TAO_USDT"}
	}
	return out
}
