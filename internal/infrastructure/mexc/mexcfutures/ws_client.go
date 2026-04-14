package mexcfutures

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSFrame is a minimal decode of inbound JSON (MEXC contract WS pushes use channel/data/symbol/ts).
type WSFrame struct {
	Channel string          `json:"channel"`
	Method  string          `json:"method"`
	Data    json.RawMessage `json:"data"`
	Param   json.RawMessage `json:"param"`
	Symbol  string          `json:"symbol"`
	TS      int64           `json:"ts"`
}

// ContractWS is the MEXC Futures **contract** WebSocket client for wss://contract.mexc.com/edge,
// modeled after mexc-futures-sdk/src/websocket.ts (MexcFuturesWebSocket).
//
// Concurrency: at most one goroutine should call ReadMessage; WriteJSON is serialized internally.
type ContractWS struct {
	cfg WSConfig

	connMu sync.RWMutex
	conn   *websocket.Conn

	writeMu sync.Mutex

	pingStop chan struct{}
	pingWG   sync.WaitGroup

	closeOnce sync.Once
}

// NewContractWS validates config and returns an unconnected client.
func NewContractWS(cfg WSConfig) (*ContractWS, error) {
	return &ContractWS{cfg: cfg}, nil
}

// Connect dials the WebSocket and starts the periodic ping loop (method "ping").
func (c *ContractWS) Connect(ctx context.Context) error {
	d := c.cfg.dialer()
	hdr := http.Header{}
	hdr.Set("Origin", "https://www.mexc.com")

	conn, _, err := d.DialContext(ctx, c.cfg.wsURL(), hdr)
	if err != nil {
		return fmt.Errorf("mexcfutures ws: dial: %w", err)
	}

	c.connMu.Lock()
	if c.conn != nil {
		c.connMu.Unlock()
		_ = conn.Close()
		return fmt.Errorf("mexcfutures ws: already connected")
	}
	c.conn = conn
	c.pingStop = make(chan struct{})
	c.connMu.Unlock()

	c.pingWG.Add(1)
	go c.pingLoop()

	return nil
}

func (c *ContractWS) pingLoop() {
	defer c.pingWG.Done()
	t := time.NewTicker(c.cfg.pingInterval())
	defer t.Stop()
	for {
		select {
		case <-c.pingStop:
			return
		case <-t.C:
			if err := c.writeJSONLocked(map[string]string{"method": "ping"}); err != nil {
				return
			}
		}
	}
}

// Close stops ping and closes the connection (idempotent).
func (c *ContractWS) Close() error {
	var err error
	c.closeOnce.Do(func() {
		if c.pingStop != nil {
			close(c.pingStop)
		}
		c.pingWG.Wait()

		c.connMu.Lock()
		if c.conn != nil {
			err = c.conn.Close()
			c.conn = nil
		}
		c.connMu.Unlock()
	})
	return err
}

func (c *ContractWS) writeJSONLocked(v any) error {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()
	if conn == nil {
		return fmt.Errorf("mexcfutures ws: not connected")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.connMu.RLock()
	conn = c.conn
	c.connMu.RUnlock()
	if conn == nil {
		return fmt.Errorf("mexcfutures ws: not connected")
	}
	return conn.WriteJSON(v)
}

// WriteJSON sends one JSON message (exported for advanced use; prefer Subscribe* helpers).
func (c *ContractWS) WriteJSON(v any) error {
	return c.writeJSONLocked(v)
}

// ReadMessage reads the next frame (text/binary). Only one concurrent reader is allowed.
func (c *ContractWS) ReadMessage() (messageType int, p []byte, err error) {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()
	if conn == nil {
		return 0, nil, fmt.Errorf("mexcfutures ws: not connected")
	}
	return conn.ReadMessage()
}

// SetReadDeadline forwards to the underlying connection (optional per-read timeout).
func (c *ContractWS) SetReadDeadline(t time.Time) error {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()
	if conn == nil {
		return fmt.Errorf("mexcfutures ws: not connected")
	}
	return conn.SetReadDeadline(t)
}

// --- Public market channels (same JSON as mexc-futures-sdk) ---

// SubscribeDepth sends sub.depth (incremental order book).
func (c *ContractWS) SubscribeDepth(symbol string, compress bool) error {
	param := map[string]any{"symbol": symbol}
	if compress {
		param["compress"] = true
	}
	return c.WriteJSON(map[string]any{"method": "sub.depth", "param": param})
}

// UnsubscribeDepth sends unsub.depth.
func (c *ContractWS) UnsubscribeDepth(symbol string) error {
	return c.WriteJSON(map[string]any{
		"method": "unsub.depth",
		"param":  map[string]any{"symbol": symbol},
	})
}

// SubscribeFullDepth sends sub.depth.full (limit 5, 10, or 20).
func (c *ContractWS) SubscribeFullDepth(symbol string, limit int) error {
	if limit != 5 && limit != 10 && limit != 20 {
		return fmt.Errorf("mexcfutures ws: full depth limit must be 5, 10, or 20")
	}
	return c.WriteJSON(map[string]any{
		"method": "sub.depth.full",
		"param": map[string]any{
			"symbol": symbol,
			"limit":  limit,
		},
	})
}

// UnsubscribeFullDepth sends usub.depth.full.
func (c *ContractWS) UnsubscribeFullDepth(symbol string) error {
	return c.WriteJSON(map[string]any{
		"method": "usub.depth.full",
		"param":  map[string]any{"symbol": symbol},
	})
}

// SubscribeTicker sends sub.ticker.
func (c *ContractWS) SubscribeTicker(symbol string) error {
	return c.WriteJSON(map[string]any{
		"method": "sub.ticker",
		"param":  map[string]any{"symbol": symbol},
	})
}

// UnsubscribeTicker sends unsub.ticker.
func (c *ContractWS) UnsubscribeTicker(symbol string) error {
	return c.WriteJSON(map[string]any{
		"method": "unsub.ticker",
		"param":  map[string]any{"symbol": symbol},
	})
}

// SubscribeDeals sends sub.deal (public trades).
func (c *ContractWS) SubscribeDeals(symbol string) error {
	return c.WriteJSON(map[string]any{
		"method": "sub.deal",
		"param":  map[string]any{"symbol": symbol},
	})
}

// UnsubscribeDeals sends unsub.deal.
func (c *ContractWS) UnsubscribeDeals(symbol string) error {
	return c.WriteJSON(map[string]any{
		"method": "unsub.deal",
		"param":  map[string]any{"symbol": symbol},
	})
}

// SubscribeAllTickers sends sub.tickers.
func (c *ContractWS) SubscribeAllTickers(gzip bool) error {
	msg := map[string]any{"method": "sub.tickers", "param": map[string]any{}}
	if gzip {
		msg["gzip"] = true
	}
	return c.WriteJSON(msg)
}

// UnsubscribeAllTickers sends unsub.tickers.
func (c *ContractWS) UnsubscribeAllTickers() error {
	return c.WriteJSON(map[string]any{"method": "unsub.tickers", "param": map[string]any{}})
}

// --- Private (API key) login, same semantics as TS login() ---

// Login sends method login with HMAC-SHA256(apiKey+reqTime, secretKey). Requires APIKey and SecretKey on WSConfig.
func (c *ContractWS) Login(subscribeDefault bool) error {
	if c.cfg.APIKey == "" || c.cfg.SecretKey == "" {
		return fmt.Errorf("mexcfutures ws: Login requires WSConfig.APIKey and WSConfig.SecretKey")
	}
	reqTime := strconv.FormatInt(time.Now().UnixMilli(), 10)
	sig := hmac.New(sha256.New, []byte(c.cfg.SecretKey))
	sig.Write([]byte(c.cfg.APIKey + reqTime))
	signature := hex.EncodeToString(sig.Sum(nil))
	return c.WriteJSON(map[string]any{
		"subscribe": subscribeDefault,
		"method":    "login",
		"param": map[string]any{
			"apiKey":    c.cfg.APIKey,
			"signature": signature,
			"reqTime":   reqTime,
		},
	})
}

// WSPersonalFilterItem matches TS FilterParams filters entries.
type WSPersonalFilterItem struct {
	Filter string   `json:"filter"`
	Rules  []string `json:"rules,omitempty"`
}

// SetPersonalFilter sends personal.filter (call after successful login).
func (c *ContractWS) SetPersonalFilter(filters []WSPersonalFilterItem) error {
	return c.WriteJSON(map[string]any{
		"method": "personal.filter",
		"param": map[string]any{
			"filters": filters,
		},
	})
}
