package mexcfutures

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const envWebKey = "MEXC_SOURCE_WEB_KEY"

// LoadDotEnv loads a .env file from the working directory if present. Errors
// from a missing file are ignored (same pattern as many CLIs).
func LoadDotEnv() {
	_ = godotenv.Load()
}

// WebKeyFromEnv returns MEXC_SOURCE_WEB_KEY after optional godotenv.Load.
func WebKeyFromEnv(loadDotenv bool) (string, error) {
	if loadDotenv {
		LoadDotEnv()
	}
	k := strings.TrimSpace(os.Getenv(envWebKey))
	if k == "" {
		return "", fmt.Errorf("mexcfutures: %s is not set", envWebKey)
	}
	return k, nil
}
