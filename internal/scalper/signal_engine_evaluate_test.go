package scalper

import (
	"testing"
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/config"
)

// Regression: CooldownUntil zero-value must not make Evaluate always return DecisionEnter,
// otherwise DecisionAddLadder / manage exit are unreachable during an active ladder.
func TestEvaluate_addLadderWhenCooldownUntilUnset(t *testing.T) {
	cfg := config.Scalper{
		TickSize:          0.01,
		MinSignalScore:    0.2,
		MinImbalance:      0.01,
		MinPressureDelta:  -1,
		MinImbalanceDelta: -1,
		MinPulseTicks:     0,
		MaxLadderSteps:    4,
		MinStepInterval:   time.Millisecond,
		FeatureLookback:   time.Second,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: 0.5, BestBidPx: 100, BestAskPx: 100.02,
			BidVol5: 60, AskVol5: 40,
		},
		HasLookback: true,
		Previous:    Snapshot{Imbalance5: 0.3},
	}
	last := time.Now().Add(-time.Hour)
	ladder := &LadderContext{
		Phase:         PhaseInventoryOpen,
		Side:          SideLong,
		StepCount:     1,
		LastStepAt:    last,
		CooldownUntil: time.Time{},
		MaxSteps:      cfg.MaxLadderSteps,
	}
	dec := e.Evaluate(time.Now(), f, ladder)
	if dec.Action != DecisionAddLadder {
		t.Fatalf("want DecisionAddLadder, got %s (reason=%s)", dec.Action, dec.Reason)
	}
}

func TestEvaluate_freshEnterAfterCooldownElapsed(t *testing.T) {
	cfg := config.Scalper{MinSignalScore: 0.2, MinImbalance: 0.01, MinPressureDelta: -1, MinImbalanceDelta: -1, MinPulseTicks: 0, MaxLadderSteps: 4, MinStepInterval: time.Millisecond, FeatureLookback: time.Second, TickSize: 0.01}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot:    Snapshot{HasBook: true, Imbalance5: 0.5, BestBidPx: 100, BestAskPx: 100.02, BidVol5: 60, AskVol5: 40},
		HasLookback: true,
		Previous:    Snapshot{Imbalance5: 0.3},
	}
	past := time.Now().Add(-time.Minute)
	ladder := &LadderContext{
		Phase:         PhaseCooldown,
		Side:          SideLong,
		CooldownUntil: past,
		MaxSteps:      4,
	}
	dec := e.Evaluate(time.Now(), f, ladder)
	if dec.Action != DecisionEnter {
		t.Fatalf("want DecisionEnter after cooldown, got %s", dec.Action)
	}
}

func TestEvaluate_addLadder_whenInvertExecution_matchesExecutedSide(t *testing.T) {
	cfg := config.Scalper{
		TickSize:               0.01,
		MinSignalScore:         0.2,
		MinImbalance:           0.01,
		MinPressureDelta:       -1,
		MinImbalanceDelta:      -1,
		MinPulseTicks:          0,
		MaxLadderSteps:         4,
		MinStepInterval:        time.Millisecond,
		FeatureLookback:        time.Second,
		InvertExecution:        true,
		SignalConfirmMinTicks:  0,
		MinMicroPriceTicks:     0,
		MaxSpreadTicksInWindow: 0,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: 0.5, BestBidPx: 100, BestAskPx: 100.02,
			BidVol5: 60, AskVol5: 40,
		},
		HasLookback: true,
		Previous:    Snapshot{Imbalance5: 0.3},
	}
	last := time.Now().Add(-time.Hour)
	// Signal is long; with invert we hold short — ladder side must match executed side.
	ladder := &LadderContext{
		Phase:         PhaseInventoryOpen,
		Side:          SideShort,
		StepCount:     1,
		LastStepAt:    last,
		CooldownUntil: time.Time{},
		MaxSteps:      cfg.MaxLadderSteps,
	}
	dec := e.Evaluate(time.Now(), f, ladder)
	if dec.Action != DecisionAddLadder {
		t.Fatalf("want DecisionAddLadder, got %s (reason=%s)", dec.Action, dec.Reason)
	}
	if dec.Side != SideLong {
		t.Fatalf("want signal side long, got %s", dec.Side)
	}
}

func TestEvaluate_rejectsUnconfirmedSignal(t *testing.T) {
	cfg := config.Scalper{
		TickSize:              0.01,
		MinSignalScore:        0.2,
		MinImbalance:          0.01,
		MinPressureDelta:      -1,
		MinImbalanceDelta:     -1,
		MinPulseTicks:         0,
		SignalConfirmMinTicks: 2,
		MinMicroPriceTicks:    0,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: 0.5, BestBidPx: 100, BestAskPx: 100.02,
			BidVol5: 60, AskVol5: 40,
		},
		HasLookback:        true,
		Previous:           Snapshot{Imbalance5: 0.3},
		PressureDelta:      0.2,
		MicroPriceDelta:    0.001,
		SignalConfirmCount: 1,
	}
	dec := e.Evaluate(time.Now(), f, nil)
	if dec.Action != DecisionHold {
		t.Fatalf("want DecisionHold, got %s", dec.Action)
	}
	if dec.Reason != "signal_not_confirmed" {
		t.Fatalf("want signal_not_confirmed, got %s", dec.Reason)
	}
}

func TestEvaluate_rejectsUnstableSpread(t *testing.T) {
	cfg := config.Scalper{
		TickSize:               0.01,
		MinSignalScore:         0.2,
		MinImbalance:           0.01,
		MinPressureDelta:       -1,
		MinImbalanceDelta:      -1,
		MinPulseTicks:          0,
		MaxSpreadTicksInWindow: 1.5,
		MinMicroPriceTicks:     0,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: 0.5, BestBidPx: 100, BestAskPx: 100.02,
			BidVol5: 60, AskVol5: 40,
		},
		HasLookback:          true,
		Previous:             Snapshot{Imbalance5: 0.3},
		PressureDelta:        0.2,
		MicroPriceDelta:      0.001,
		SignalConfirmCount:   2,
		SignalConfirmAge:     200 * time.Millisecond,
		MaxRecentSpreadTicks: 2.1,
	}
	dec := e.Evaluate(time.Now(), f, nil)
	if dec.Action != DecisionHold {
		t.Fatalf("want DecisionHold, got %s", dec.Action)
	}
	if dec.Reason != "spread_unstable" {
		t.Fatalf("want spread_unstable, got %s", dec.Reason)
	}
}

func TestEvaluate_rejectsPriceInsideRange(t *testing.T) {
	cfg := config.Scalper{
		TickSize:                   0.01,
		MinSignalScore:             0.2,
		MinImbalance:               0.01,
		MinPressureDelta:           -1,
		MinImbalanceDelta:          -1,
		MinPulseTicks:              0,
		PriceCorridorWindow:        15 * time.Second,
		PriceCorridorMaxMultiplier: 2,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: 0.5, BestBidPx: 100, BestAskPx: 100.02, Mid: 100.00,
			BidVol5: 60, AskVol5: 40,
		},
		HasLookback:        true,
		Previous:           Snapshot{Imbalance5: 0.3},
		PressureDelta:      0.2,
		MicroPriceDelta:    0.001,
		SignalConfirmCount: 2,
		SignalConfirmAge:   300 * time.Millisecond,
		HasPriceCorridor:   true,
		PriceMean:          100,
		PriceLowerBound:    99.90,
		PriceUpperBound:    100.10,
		PriceMaxLowerBound: 99.80,
		PriceMaxUpperBound: 100.20,
	}
	dec := e.Evaluate(time.Now(), f, nil)
	if dec.Action != DecisionHold {
		t.Fatalf("want DecisionHold, got %s", dec.Action)
	}
	if dec.Reason != "price_inside_range" {
		t.Fatalf("want price_inside_range, got %s", dec.Reason)
	}
}

func TestEvaluate_allowsLongAtLowerBand(t *testing.T) {
	cfg := config.Scalper{
		TickSize:                   0.01,
		MinSignalScore:             0.2,
		MinImbalance:               0.01,
		MinPressureDelta:           -1,
		MinImbalanceDelta:          -1,
		MinPulseTicks:              0,
		PriceCorridorWindow:        15 * time.Second,
		PriceCorridorMaxMultiplier: 2,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: 0.5, BestBidPx: 99.89, BestAskPx: 99.91, Mid: 99.90,
			BidVol5: 60, AskVol5: 40,
		},
		HasLookback:        true,
		Previous:           Snapshot{Imbalance5: 0.3},
		PressureDelta:      0.2,
		MicroPriceDelta:    0.001,
		SignalConfirmCount: 2,
		SignalConfirmAge:   300 * time.Millisecond,
		HasPriceCorridor:   true,
		PriceMean:          100,
		PriceLowerBound:    99.90,
		PriceUpperBound:    100.10,
		PriceMaxLowerBound: 99.80,
		PriceMaxUpperBound: 100.20,
	}
	dec := e.Evaluate(time.Now(), f, nil)
	if dec.Action != DecisionEnter {
		t.Fatalf("want DecisionEnter, got %s (reason=%s)", dec.Action, dec.Reason)
	}
	if dec.Side != SideLong {
		t.Fatalf("want SideLong, got %s", dec.Side)
	}
}

func TestEvaluate_rejectsPriceOutsideRangeExtension(t *testing.T) {
	cfg := config.Scalper{
		TickSize:                   0.01,
		MinSignalScore:             0.2,
		MinImbalance:               0.01,
		MinPressureDelta:           -1,
		MinImbalanceDelta:          -1,
		MinPulseTicks:              0,
		PriceCorridorWindow:        15 * time.Second,
		PriceCorridorMaxMultiplier: 2,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: 0.5, BestBidPx: 99.69, BestAskPx: 99.71, Mid: 99.70,
			BidVol5: 60, AskVol5: 40,
		},
		HasLookback:        true,
		Previous:           Snapshot{Imbalance5: 0.3},
		PressureDelta:      0.2,
		MicroPriceDelta:    0.001,
		SignalConfirmCount: 2,
		SignalConfirmAge:   300 * time.Millisecond,
		HasPriceCorridor:   true,
		PriceMean:          100,
		PriceLowerBound:    99.90,
		PriceUpperBound:    100.10,
		PriceMaxLowerBound: 99.80,
		PriceMaxUpperBound: 100.20,
	}
	dec := e.Evaluate(time.Now(), f, nil)
	if dec.Action != DecisionHold {
		t.Fatalf("want DecisionHold, got %s", dec.Action)
	}
	if dec.Reason != "price_below_range_extension" {
		t.Fatalf("want price_below_range_extension, got %s", dec.Reason)
	}
}

func TestEvaluate_priceCorridorUsesSignalSideWhenInverted(t *testing.T) {
	cfg := config.Scalper{
		TickSize:                   0.01,
		MinSignalScore:             0.2,
		MinImbalance:               0.01,
		MinPressureDelta:           -1,
		MinImbalanceDelta:          -1,
		MinPulseTicks:              0,
		InvertExecution:            true,
		PriceCorridorWindow:        15 * time.Second,
		PriceCorridorMaxMultiplier: 2,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: 0.5, BestBidPx: 99.89, BestAskPx: 99.91, Mid: 99.90,
			BidVol5: 60, AskVol5: 40,
		},
		HasLookback:        true,
		Previous:           Snapshot{Imbalance5: 0.3},
		PressureDelta:      0.2,
		MicroPriceDelta:    0.001,
		SignalConfirmCount: 2,
		SignalConfirmAge:   300 * time.Millisecond,
		HasPriceCorridor:   true,
		PriceMean:          100,
		PriceLowerBound:    99.90,
		PriceUpperBound:    100.10,
		PriceMaxLowerBound: 99.80,
		PriceMaxUpperBound: 100.20,
	}
	dec := e.Evaluate(time.Now(), f, nil)
	if dec.Action != DecisionEnter {
		t.Fatalf("want DecisionEnter, got %s (reason=%s)", dec.Action, dec.Reason)
	}
	if dec.Side != SideLong {
		t.Fatalf("want SideLong (коридор по сигналу книги), got %s", dec.Side)
	}
}

func TestEvaluate_allowsShortAtUpperBand(t *testing.T) {
	cfg := config.Scalper{
		TickSize:                   0.01,
		MinSignalScore:             0.2,
		MinImbalance:               0.01,
		MinPressureDelta:           -1,
		MinImbalanceDelta:          -1,
		MinPulseTicks:              0,
		PriceCorridorWindow:        15 * time.Second,
		PriceCorridorMaxMultiplier: 2,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: -0.5, BestBidPx: 100.09, BestAskPx: 100.11, Mid: 100.10,
			BidVol5: 40, AskVol5: 60,
		},
		HasLookback:        true,
		Previous:           Snapshot{Imbalance5: -0.3},
		PressureDelta:      -0.2,
		MicroPriceDelta:    -0.001,
		SignalConfirmCount: 2,
		SignalConfirmAge:   300 * time.Millisecond,
		HasPriceCorridor:   true,
		PriceMean:          100,
		PriceLowerBound:    99.90,
		PriceUpperBound:    100.10,
		PriceMaxLowerBound: 99.80,
		PriceMaxUpperBound: 100.20,
	}
	dec := e.Evaluate(time.Now(), f, nil)
	if dec.Action != DecisionEnter {
		t.Fatalf("want DecisionEnter, got %s (reason=%s)", dec.Action, dec.Reason)
	}
	if dec.Side != SideShort {
		t.Fatalf("want SideShort, got %s", dec.Side)
	}
}

func TestEvaluate_rejectsDealTapeInsufficient(t *testing.T) {
	cfg := config.Scalper{
		TickSize:               0.01,
		MinSignalScore:         0.2,
		MinImbalance:           0.01,
		MinPressureDelta:       -1,
		MinImbalanceDelta:      -1,
		MinPulseTicks:          0,
		SignalConfirmMinTicks:  0,
		MinMicroPriceTicks:     0,
		MaxSpreadTicksInWindow: 0,
		EntryDealFilterEnabled: true,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: 0.5, BestBidPx: 100, BestAskPx: 100.02,
			BidVol5: 60, AskVol5: 40,
		},
		HasLookback:   true,
		PressureDelta: 0.15,
		MicroPriceDelta: 0.001,
		HasDealTape1s: false,
	}
	dec := e.Evaluate(time.Now(), f, nil)
	if dec.Reason != "deal_tape_insufficient" {
		t.Fatalf("want deal_tape_insufficient, got %q", dec.Reason)
	}
}

func TestEvaluate_rejectsDealTapeMisalignedForShort(t *testing.T) {
	cfg := config.Scalper{
		TickSize:               0.01,
		MinSignalScore:         0.2,
		MinImbalance:           0.01,
		MinPressureDelta:       -1,
		MinImbalanceDelta:      -1,
		MinPulseTicks:          0,
		SignalConfirmMinTicks:  0,
		MinMicroPriceTicks:     0,
		MaxSpreadTicksInWindow: 0,
		EntryDealFilterEnabled: true,
	}
	e := NewSignalEngine(cfg)
	f := Features{
		Snapshot: Snapshot{
			HasBook: true, Imbalance5: -0.55, BestBidPx: 100, BestAskPx: 100.02,
			BidVol5: 40, AskVol5: 60,
		},
		HasLookback:     true,
		PressureDelta:   -0.12,
		MicroPriceDelta: -0.001,
		HasDealTape1s:   true,
		DealVolDelta1s:  5.0,
	}
	dec := e.Evaluate(time.Now(), f, nil)
	if dec.Reason != "deal_tape_misaligned" {
		t.Fatalf("want deal_tape_misaligned, got %q", dec.Reason)
	}
}
