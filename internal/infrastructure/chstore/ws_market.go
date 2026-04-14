package chstore

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

const wsMarketTable = "futures_ws_market"

// WSMarketRow is one persisted contract-edge WebSocket frame (public market).
type WSMarketRow struct {
	IngestedAt time.Time
	ExchangeTS int64
	Symbol     string
	Channel    string
	MessageRaw string
}

// Client is a small ClickHouse writer for WS capture tables.
type Client struct {
	cfg  Config
	conn driver.Conn

	schemaMu   sync.Mutex
	schemaDone bool
}

// Dial opens a native-protocol connection and pings the server.
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	opts := &clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.User,
			Password: cfg.Password,
		},
		DialTimeout: 15 * time.Second,
	}
	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("chstore: open: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := conn.Ping(pingCtx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("chstore: ping %q: %w", cfg.Addr, err)
	}
	return &Client{cfg: cfg, conn: conn}, nil
}

// Close releases the connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// InitMarketWSSchema creates database and the market WebSocket capture table if missing (symbol is a column).
// Safe to call from several goroutines (e.g. one per tracked symbol); runs DDL at most once after success.
func (c *Client) InitMarketWSSchema(ctx context.Context) error {
	c.schemaMu.Lock()
	defer c.schemaMu.Unlock()
	if c.schemaDone {
		return nil
	}
	db := quoteIdent(c.cfg.Database)
	if err := c.conn.Exec(ctx, "CREATE DATABASE IF NOT EXISTS "+db); err != nil {
		return fmt.Errorf("chstore: create database: %w", err)
	}
	tbl := quoteIdent(wsMarketTable)
	q := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s.%s
(
    ingested_at DateTime64(3) DEFAULT now64(3),
    exchange_ts Int64,
    symbol LowCardinality(String),
    channel LowCardinality(String),
    message_json String CODEC(ZSTD(3))
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(ingested_at)
ORDER BY (symbol, channel, ingested_at, exchange_ts)`, db, tbl)
	if err := c.conn.Exec(ctx, q); err != nil {
		return fmt.Errorf("chstore: create table: %w", err)
	}
	c.schemaDone = true
	return nil
}

// InsertWSMarketRows appends rows in one batch insert.
func (c *Client) InsertWSMarketRows(ctx context.Context, rows []WSMarketRow) error {
	if len(rows) == 0 {
		return nil
	}
	fq := fmt.Sprintf("%s.%s", quoteIdent(c.cfg.Database), quoteIdent(wsMarketTable))
	batch, err := c.conn.PrepareBatch(ctx, "INSERT INTO "+fq+" (ingested_at, exchange_ts, symbol, channel, message_json)")
	if err != nil {
		return fmt.Errorf("chstore: prepare batch: %w", err)
	}
	for _, r := range rows {
		if err := batch.Append(r.IngestedAt, r.ExchangeTS, r.Symbol, r.Channel, r.MessageRaw); err != nil {
			return fmt.Errorf("chstore: append: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		return fmt.Errorf("chstore: send: %w", err)
	}
	return nil
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
