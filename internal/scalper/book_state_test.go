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

func TestBookState_priceCorridorMinSamples(t *testing.T) {
	now := time.Now().UTC()
	b := &BookState{
		cfg: config.Scalper{
			TickSize:                0.01,
			PriceCorridorWindow:     5 * time.Second,
			PriceCorridorPercentile: 0.8,
			PriceCorridorMinSamples: 6,
		},
		recent: []bookSnapshotEvent{
			{at: now.Add(-4 * time.Second), snapshot: Snapshot{Mid: 100}},
			{at: now.Add(-3 * time.Second), snapshot: Snapshot{Mid: 101}},
			{at: now.Add(-2 * time.Second), snapshot: Snapshot{Mid: 99}},
			{at: now.Add(-1 * time.Second), snapshot: Snapshot{Mid: 102}},
			{at: now, snapshot: Snapshot{Mid: 98}},
		},
	}
	_, _, _, samples, ok := b.priceCorridorLocked(now)
	if ok {
		t.Fatal("want corridor unavailable when samples below minimum")
	}
	if samples != 5 {
		t.Fatalf("want 5 samples, got %d", samples)
	}
}

func TestBookState_priceCorridorUnilateralMirrorsBand(t *testing.T) {
	now := time.Now().UTC()
	b := &BookState{
		cfg: config.Scalper{
			TickSize:                  0.01,
			PriceCorridorWindow:       15 * time.Second,
			PriceCorridorPercentile:   0.8,
			PriceCorridorMinSamples:   2,
			PriceCorridorMeanHalfLife: 100 * time.Millisecond,
		},
		recent: []bookSnapshotEvent{
			{at: now.Add(-4 * time.Second), snapshot: Snapshot{Mid: 100}},
			{at: now.Add(-3 * time.Second), snapshot: Snapshot{Mid: 100}},
			{at: now.Add(-2 * time.Second), snapshot: Snapshot{Mid: 100}},
			{at: now, snapshot: Snapshot{Mid: 105}},
		},
	}
	mean, upDev, lowDev, samples, ok := b.priceCorridorLocked(now)
	if !ok {
		t.Fatal("want corridor when only one-sided deviations from weighted mean")
	}
	if samples != 4 {
		t.Fatalf("want 4 samples, got %d", samples)
	}
	if math.Abs(mean-105) > 0.02 {
		t.Fatalf("want mean≈105, got %.6f", mean)
	}
	if math.Abs(upDev-lowDev) > 1e-6 {
		t.Fatalf("want mirrored deviations, upper=%.6f lower=%.6f", upDev, lowDev)
	}
	if upDev <= 0 {
		t.Fatalf("want positive deviation, got upper=%.6f lower=%.6f", upDev, lowDev)
	}
}
