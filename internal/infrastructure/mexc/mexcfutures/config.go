package mexcfutures

import (
	"net/http"
	"time"
)

const (
	defaultFuturesBaseURL  = "https://futures.mexc.com/api/v1"
	defaultContractBaseURL = "https://contract.mexc.com/api/v1"
)

// Config holds client options. WebKey is the MEXC browser cookie value (WEB…).
type Config struct {
	WebKey string

	// FuturesBaseURL is the REST prefix for futures.mexc.com private endpoints.
	FuturesBaseURL string
	// ContractBaseURL is the REST prefix for contract.mexc.com (Python client compat).
	ContractBaseURL string

	UserAgent string
	HTTPClient *http.Client
}

func (c Config) futuresBase() string {
	if c.FuturesBaseURL != "" {
		return c.FuturesBaseURL
	}
	return defaultFuturesBaseURL
}

func (c Config) contractBase() string {
	if c.ContractBaseURL != "" {
		return c.ContractBaseURL
	}
	return defaultContractBaseURL
}

func (c Config) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
}

func (c Config) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}
