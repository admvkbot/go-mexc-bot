package chstore

import (
	"os"
	"strings"
)

// Config holds native ClickHouse client settings (TCP 9000 by default).
type Config struct {
	Addr     string
	User     string
	Password string
	Database string
}

// ConfigFromEnv reads CLICKHOUSE_ADDR or CLICKHOUSE_HOST + CLICKHOUSE_PORT,
// CLICKHOUSE_USER (default default), CLICKHOUSE_PASSWORD, CLICKHOUSE_DATABASE (default mexc_bot).
// If no address parts are set, Addr defaults to 127.0.0.1:9000 for local runs with published ports.
func ConfigFromEnv() Config {
	addr := strings.TrimSpace(os.Getenv("CLICKHOUSE_ADDR"))
	if addr == "" {
		h := strings.TrimSpace(os.Getenv("CLICKHOUSE_HOST"))
		p := strings.TrimSpace(os.Getenv("CLICKHOUSE_PORT"))
		if p == "" {
			p = "9000"
		}
		if h != "" {
			addr = h + ":" + p
		}
	}
	if addr == "" {
		addr = "127.0.0.1:9000"
	}
	db := strings.TrimSpace(os.Getenv("CLICKHOUSE_DATABASE"))
	if db == "" {
		db = "mexc_bot"
	}
	user := strings.TrimSpace(os.Getenv("CLICKHOUSE_USER"))
	if user == "" {
		user = "default"
	}
	return Config{
		Addr:     addr,
		User:     user,
		Password: os.Getenv("CLICKHOUSE_PASSWORD"),
		Database: db,
	}
}
