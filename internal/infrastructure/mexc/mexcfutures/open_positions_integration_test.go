package mexcfutures

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
)

// TestOpenPositionsFuturesFromEnv loads .env (if present), reads MEXC_SOURCE_WEB_KEY,
// and requests GET /private/position/open_positions on the futures host.
// Skips when the key is unset so CI and local runs without credentials stay green.
//
// go test sets the working directory to this package folder, so we load
// ../../../../.env in addition to ./.env to pick up a project-root .env like
// NewClientFromEnv would when run from the repository root.
func TestOpenPositionsFuturesFromEnv(t *testing.T) {
	_ = godotenv.Load()
	_ = godotenv.Load(filepath.Join("..", "..", "..", "..", ".env"))
	if _, err := WebKeyFromEnv(false); err != nil {
		t.Skipf("skip: %v", err)
	}
	cli, err := NewClientFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body, err := cli.OpenPositionsFutures(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertFuturesPrivateJSONEnvelope(t, body)
}

func assertFuturesPrivateJSONEnvelope(t *testing.T, body []byte) {
	t.Helper()
	var outer struct {
		Success *bool           `json:"success"`
		Code    json.RawMessage `json:"code"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		t.Fatalf("response is not JSON: %v; body=%s", err, truncate(string(body), 384))
	}
	if outer.Success != nil && !*outer.Success {
		t.Fatalf("success=false; body=%s", truncate(string(body), 768))
	}
}
