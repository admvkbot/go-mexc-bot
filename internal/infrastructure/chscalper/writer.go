package chscalper

import (
	"context"

	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/chstore"
	"github.com/mexc-bot/go-mexc-bot/internal/scalper"
)

// Writer adapts chstore.Client to the scalper journal interface.
type Writer struct {
	ch *chstore.Client
}

func NewWriter(ch *chstore.Client) *Writer {
	return &Writer{ch: ch}
}

func (w *Writer) InitScalperSchema(ctx context.Context) error {
	if w == nil || w.ch == nil {
		return nil
	}
	return w.ch.InitScalperSchema(ctx)
}

func (w *Writer) InsertScalperSignalEvents(ctx context.Context, rows []scalper.SignalEvent) error {
	if w == nil || w.ch == nil || len(rows) == 0 {
		return nil
	}
	out := make([]chstore.ScalperSignalEventRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, chstore.ScalperSignalEventRow{
			SessionID:      r.SessionID,
			Mode:           r.Mode,
			Symbol:         r.Symbol,
			EventAt:        r.EventAt,
			Action:         r.Action,
			Side:           r.Side,
			Score:          r.Score,
			Reason:         r.Reason,
			BestBidPx:      r.BestBidPx,
			BestAskPx:      r.BestAskPx,
			Spread:         r.Spread,
			BidVol5:        r.BidVol5,
			AskVol5:        r.AskVol5,
			Imbalance5:     r.Imbalance5,
			BidPulseTicks:  int32(r.BidPulseTicks),
			AskPulseTicks:  int32(r.AskPulseTicks),
			PressureDelta:  r.PressureDelta,
			UpdateRate:     r.UpdateRate,
			MicroPriceDiff: r.MicroPriceDiff,
		})
	}
	return w.ch.InsertScalperSignalEventRows(ctx, out)
}

func (w *Writer) InsertScalperOrderEvents(ctx context.Context, rows []scalper.OrderEvent) error {
	if w == nil || w.ch == nil || len(rows) == 0 {
		return nil
	}
	out := make([]chstore.ScalperOrderEventRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, chstore.ScalperOrderEventRow{
			SessionID:   r.SessionID,
			LadderID:    r.LadderID,
			Symbol:      r.Symbol,
			EventAt:     r.EventAt,
			EventType:   r.EventType,
			OrderClass:  r.OrderClass,
			Side:        r.Side,
			ExternalOID: r.ExternalOID,
			OrderID:     r.OrderID,
			Price:       r.Price,
			Quantity:    r.Quantity,
			FilledQty:   r.FilledQty,
			AvgFillPx:   r.AvgFillPx,
			StateCode:   int32(r.StateCode),
			Reason:      r.Reason,
			RawJSON:     r.RawJSON,
		})
	}
	return w.ch.InsertScalperOrderEventRows(ctx, out)
}

func (w *Writer) InsertScalperRoundTrips(ctx context.Context, rows []scalper.RoundTrip) error {
	if w == nil || w.ch == nil || len(rows) == 0 {
		return nil
	}
	out := make([]chstore.ScalperRoundTripRow, 0, len(rows))
	for _, r := range rows {
		var emergency uint8
		if r.WasEmergencyFlatten {
			emergency = 1
		}
		out = append(out, chstore.ScalperRoundTripRow{
			SessionID:           r.SessionID,
			LadderID:            r.LadderID,
			Symbol:              r.Symbol,
			Side:                r.Side,
			EntryStartedAt:      r.EntryStartedAt,
			EntryFilledAt:       r.EntryFilledAt,
			ExitStartedAt:       r.ExitStartedAt,
			ExitFilledAt:        r.ExitFilledAt,
			HoldingMS:           r.HoldingMS,
			EntryAvgPx:          r.EntryAvgPx,
			ExitAvgPx:           r.ExitAvgPx,
			FilledQty:           r.FilledQty,
			GrossPnL:            r.GrossPnL,
			PNLTicks:            r.PNLTicks,
			MaxAdverseTicks:     r.MaxAdverseTicks,
			MaxFavorableTicks:   r.MaxFavorableTicks,
			ExitReason:          r.ExitReason,
			FlattenReason:       r.FlattenReason,
			RepricesCount:       int32(r.RepricesCount),
			LadderStepsUsed:     int32(r.LadderStepsUsed),
			WasEmergencyFlatten: emergency,
		})
	}
	return w.ch.InsertScalperRoundTripRows(ctx, out)
}

func (w *Writer) InsertScalperReplayCandidates(ctx context.Context, rows []scalper.ReplayCandidate) error {
	if w == nil || w.ch == nil || len(rows) == 0 {
		return nil
	}
	out := make([]chstore.ScalperReplayCandidateRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, chstore.ScalperReplayCandidateRow{
			SessionID:     r.SessionID,
			Symbol:        r.Symbol,
			SignalAt:      r.SignalAt,
			Side:          r.Side,
			Score:         r.Score,
			Reason:        r.Reason,
			EntryPx:       r.EntryPx,
			TargetPx:      r.TargetPx,
			StopPx:        r.StopPx,
			ExpireAt:      r.ExpireAt,
			StepCount:     int32(r.StepCount),
			BestBidPx:     r.BestBidPx,
			BestAskPx:     r.BestAskPx,
			Imbalance5:    r.Imbalance5,
			PressureDelta: r.PressureDelta,
		})
	}
	return w.ch.InsertScalperReplayCandidateRows(ctx, out)
}
