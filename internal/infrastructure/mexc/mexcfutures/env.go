package mexcfutures

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const envSourceWebKey = "MEXC_SOURCE_WEB_KEY"
const envTradeWebKey = "MEXC_WEB_KEY"

// LoadDotEnv loads a .env file from the working directory if present. Errors
// from a missing file are ignored (same pattern as many CLIs).
func LoadDotEnv() {
	_ = godotenv.Load()
}

// SourceWebKeyFromEnv returns MEXC_SOURCE_WEB_KEY after optional godotenv.Load (data / capture plane).
func SourceWebKeyFromEnv(loadDotenv bool) (string, error) {
	if loadDotenv {
		LoadDotEnv()
	}
	k := strings.TrimSpace(os.Getenv(envSourceWebKey))
	if k == "" {
		return "", fmt.Errorf("mexcfutures: %s is not set", envSourceWebKey)
	}
	return k, nil
}

// TradeWebKeyFromEnv returns MEXC_WEB_KEY after optional godotenv.Load (trading / private REST).
func TradeWebKeyFromEnv(loadDotenv bool) (string, error) {
	if loadDotenv {
		LoadDotEnv()
	}
	k := strings.TrimSpace(os.Getenv(envTradeWebKey))
	if k == "" {
		return "", fmt.Errorf("mexcfutures: %s is not set", envTradeWebKey)
	}
	return k, nil
}

// WebKeyFromEnv returns the trading WEB key (MEXC_WEB_KEY). Same as TradeWebKeyFromEnv.
func WebKeyFromEnv(loadDotenv bool) (string, error) {
	return TradeWebKeyFromEnv(loadDotenv)
}
