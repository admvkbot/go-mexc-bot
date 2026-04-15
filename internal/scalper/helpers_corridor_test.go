package scalper

import (
	"testing"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
)

func TestPriceCorridorReject_longOnExactLowerTick(t *testing.T) {
	cfg := config.Scalper{TickSize: 0.01}
	f := Features{
		Snapshot:           Snapshot{Mid: 99.90},
		PriceLowerBound:    99.90,
		PriceUpperBound:    100.10,
		PriceMaxLowerBound: 99.80,
		PriceMaxUpperBound: 100.20,
	}
	if r := priceCorridorReject(cfg, SideLong, f); r != "" {
		t.Fatalf("want empty at lower bound, got %q", r)
	}
}

func TestPriceCorridorReject_shortOnExactUpperTick(t *testing.T) {
	cfg := config.Scalper{TickSize: 0.01}
	f := Features{
		Snapshot:           Snapshot{Mid: 100.10},
		PriceLowerBound:    99.90,
		PriceUpperBound:    100.10,
		PriceMaxLowerBound: 99.80,
		PriceMaxUpperBound: 100.20,
	}
	if r := priceCorridorReject(cfg, SideShort, f); r != "" {
		t.Fatalf("want empty at upper bound, got %q", r)
	}
}
