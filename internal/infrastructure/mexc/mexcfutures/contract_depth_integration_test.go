package mexcfutures

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// linearSwapTAOUSDT is the futures REST symbol for the USDT-margined perpetual
// shown at https://www.mexc.co/en-GB/futures/TAO_USDT?type=linear_swap (pair TAO/USDT).
const linearSwapTAOUSDT = "TAO_USDT"

// TestContractDepthTAOUSDTPublicJSON fetches GET /contract/depth/{symbol} for TAO_USDT
// and prints indented JSON to stdout. Public endpoint; WebKey is only required by
// NewClient and is not sent on getPublicFutures.
func TestContractDepthTAOUSDTPublicJSON(t *testing.T) {
	cli, err := NewClient(Config{WebKey: "mexcfutures-public-depth-test"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	body, err := cli.ContractDepth(ctx, linearSwapTAOUSDT, nil)
	if err != nil {
		t.Fatal(err)
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	fmt.Println("--- REST /contract/depth ---")
	fmt.Println(pretty.String())

	// Contract edge WebSocket: same symbol, first 10 inbound JSON frames after sub.depth.
	wsCtx, wsCancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer wsCancel()

	ws, err := NewContractWS(WSConfig{PingInterval: 60 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()

	if err := ws.Connect(wsCtx); err != nil {
		t.Fatal(err)
	}
	if err := ws.SubscribeDepth(linearSwapTAOUSDT, false); err != nil {
		t.Fatal(err)
	}

	const wsPackets = 10
	for n := 0; n < wsPackets; n++ {
		if err := ws.SetReadDeadline(time.Now().Add(12 * time.Second)); err != nil {
			t.Fatalf("ws packet %d: set deadline: %v", n+1, err)
		}
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("ws packet %d: read: %v", n+1, err)
		}
		if len(data) == 0 {
			t.Fatalf("ws packet %d: empty frame", n+1)
		}
		var pktPretty bytes.Buffer
		if err := json.Indent(&pktPretty, data, "", "  "); err != nil {
			t.Fatalf("ws packet %d: not JSON: %v", n+1, err)
		}
		fmt.Printf("--- WS packet %d/%d ---\n%s\n", n+1, wsPackets, pktPretty.String())
	}

	if err := ws.UnsubscribeDepth(linearSwapTAOUSDT); err != nil {
		t.Fatal(err)
	}
}
