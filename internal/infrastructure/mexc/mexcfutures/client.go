package mexcfutures

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client performs signed WEB-token REST calls to MEXC Futures endpoints.
type Client struct {
	webKey       string
	futuresBase  string
	contractBase string
	userAgent    string
	httpClient   *http.Client
}

// NewClient validates config and returns a ready Client.
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.WebKey) == "" {
		return nil, fmt.Errorf("mexcfutures: WebKey is empty")
	}
	return &Client{
		webKey:       cfg.WebKey,
		futuresBase:  strings.TrimRight(cfg.futuresBase(), "/"),
		contractBase: strings.TrimRight(cfg.contractBase(), "/"),
		userAgent:    cfg.userAgent(),
		httpClient:   cfg.httpClient(),
	}, nil
}

// NewClientFromEnv loads .env (if present), reads MEXC_WEB_KEY (trading), and constructs a Client.
func NewClientFromEnv() (*Client, error) {
	k, err := WebKeyFromEnv(true)
	if err != nil {
		return nil, err
	}
	return NewClient(Config{WebKey: k})
}

func (c *Client) futuresURL(path string) string {
	return c.futuresBase + path
}

func (c *Client) contractURL(path string) string {
	return c.contractBase + path
}

// DoJSON issues an HTTP request and returns the raw response body on 2xx.
func (c *Client) DoJSON(ctx context.Context, req *http.Request) ([]byte, error) {
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("mexcfutures: HTTP %d: %s", resp.StatusCode, truncate(string(body), 512))
	}
	return body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (c *Client) postSignedFutures(ctx context.Context, path string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.futuresURL(path), bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c.applyDefaultHeaders(req.Header)
	c.applyAuthHeader(req.Header)
	c.applySignedPOST(req.Header, raw)
	return c.DoJSON(ctx, req)
}

func (c *Client) getAuthFutures(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := c.futuresURL(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.applyDefaultHeaders(req.Header)
	c.applyAuthHeader(req.Header)
	return c.DoJSON(ctx, req)
}

func (c *Client) getPublicFutures(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := c.futuresURL(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.applyDefaultHeaders(req.Header)
	return c.DoJSON(ctx, req)
}

func (c *Client) getSignedContract(ctx context.Context, path string, query url.Values, signBody any) ([]byte, error) {
	raw, err := json.Marshal(signBody)
	if err != nil {
		return nil, err
	}
	u := c.contractURL(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.applyDefaultHeaders(req.Header)
	c.applyAuthHeader(req.Header)
	c.applySignedPOST(req.Header, raw)
	return c.DoJSON(ctx, req)
}

func (c *Client) getPublicContract(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := c.contractURL(path)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.applyDefaultHeaders(req.Header)
	return c.DoJSON(ctx, req)
}
