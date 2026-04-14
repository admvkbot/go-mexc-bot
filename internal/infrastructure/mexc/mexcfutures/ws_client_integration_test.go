package mexcfutures

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// TestContractWSDepthTAO receives at least one push.depth for TAO_USDT over the contract edge WS
// (same endpoint and sub.depth shape as mexc-futures-sdk MexcFuturesWebSocket).
func TestContractWSDepthTAO(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ws, err := NewContractWS(WSConfig{PingInterval: 12 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()

	if err := ws.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ws.SubscribeDepth(linearSwapTAOUSDT, false); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(18 * time.Second)
	for time.Now().Before(deadline) {
		if err := ws.SetReadDeadline(time.Now().Add(8 * time.Second)); err != nil {
			t.Fatal(err)
		}
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var frame WSFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			continue
		}
		if frame.Channel == "push.depth" && frame.Symbol == linearSwapTAOUSDT {
			if len(frame.Data) == 0 {
				t.Fatal("empty depth data")
			}
			_ = ws.UnsubscribeDepth(linearSwapTAOUSDT)
			return
		}
	}
	t.Fatal("timeout waiting for push.depth")
}

// TestContractWSReadTenPacketsThenClose reads ten inbound WebSocket frames after subscribing
// to depth, sends unsub.depth, then closes the connection (defer).
func TestContractWSReadTenPacketsThenClose(t *testing.T) {
	const wantPackets = 10
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	ws, err := NewContractWS(WSConfig{
		// Long ping so most frames in this short test are subscription / depth pushes, not pong.
		PingInterval: 60 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()

	if err := ws.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ws.SubscribeDepth(linearSwapTAOUSDT, false); err != nil {
		t.Fatal(err)
	}

	for n := 0; n < wantPackets; n++ {
		if err := ws.SetReadDeadline(time.Now().Add(12 * time.Second)); err != nil {
			t.Fatalf("packet %d: set deadline: %v", n+1, err)
		}
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("packet %d: read: %v", n+1, err)
		}
		if len(data) == 0 {
			t.Fatalf("packet %d: empty frame", n+1)
		}
		var frame WSFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			t.Fatalf("packet %d: not JSON: %v", n+1, err)
		}
		_ = frame // payload shape varies (pong, rs.sub.depth, push.depth, …)
	}

	if err := ws.UnsubscribeDepth(linearSwapTAOUSDT); err != nil {
		t.Fatal(err)
	}
}
