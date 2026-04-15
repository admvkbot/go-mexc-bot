package scalper

import (
	"math"
	"testing"
)

func TestBracketSLTPQuantized_long(t *testing.T) {
	sl, tp, err := bracketSLTPQuantized(SideLong, 100.0, 0.01, 2, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(sl-99.99) > 1e-9 || math.Abs(tp-100.02) > 1e-9 {
		t.Fatalf("want sl=99.99 tp=100.02 got sl=%v tp=%v", sl, tp)
	}
}

func TestBracketSLTPQuantized_short(t *testing.T) {
	sl, tp, err := bracketSLTPQuantized(SideShort, 242.19, 0.01, 2, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(sl-242.20) > 1e-9 || math.Abs(tp-242.17) > 1e-9 {
		t.Fatalf("want sl=242.20 tp=242.17 got sl=%v tp=%v", sl, tp)
	}
}

func TestInferBracketExitReason_long(t *testing.T) {
	if g := inferBracketExitReason(SideLong, 100, 100.02, 0.01, 2, 1); g != "take_profit_ticks" {
		t.Fatalf("want take_profit_ticks got %q", g)
	}
	if g := inferBracketExitReason(SideLong, 100, 99.98, 0.01, 2, 1); g != "stop_loss_ticks" {
		t.Fatalf("want stop_loss_ticks got %q", g)
	}
}

func TestInferBracketExitReason_short(t *testing.T) {
	if g := inferBracketExitReason(SideShort, 242.19, 242.17, 0.01, 2, 1); g != "take_profit_ticks" {
		t.Fatalf("want take_profit_ticks got %q", g)
	}
	if g := inferBracketExitReason(SideShort, 242.19, 242.21, 0.01, 2, 1); g != "stop_loss_ticks" {
		t.Fatalf("want stop_loss_ticks got %q", g)
	}
}
