package scalper

import (
	"math"
	"testing"
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
)

func TestBookState_priceCorridorLocked(t *testing.T) {
	now := time.Now().UTC()
	b := &BookState{
		cfg: config.Scalper{
			TickSize:                0.01,
			PriceCorridorWindow:     5 * time.Second,
			PriceCorridorPercentile: 0.8,
		},
		recent: []bookSnapshotEvent{
			{at: now.Add(-4 * time.Second), snapshot: Snapshot{Mid: 100}},
			{at: now.Add(-3 * time.Second), snapshot: Snapshot{Mid: 101}},
			{at: now.Add(-2 * time.Second), snapshot: Snapshot{Mid: 99}},
			{at: now.Add(-1 * time.Second), snapshot: Snapshot{Mid: 102}},
			{at: now, snapshot: Snapshot{Mid: 98}},
		},
	}

	mean, up, down, samples, ok := b.priceCorridorLocked(now)
	if !ok {
		t.Fatal("want corridor to be available")
	}
	if samples != 5 {
		t.Fatalf("want 5 samples, got %d", samples)
	}
	if math.Abs(mean-100) > 1e-9 {
		t.Fatalf("want mean=100, got %.6f", mean)
	}
	if math.Abs(up-1.8) > 1e-9 {
		t.Fatalf("want upper deviation=1.8, got %.6f", up)
	}
	if math.Abs(down-1.8) > 1e-9 {
		t.Fatalf("want lower deviation=1.8, got %.6f", down)
	}
}
