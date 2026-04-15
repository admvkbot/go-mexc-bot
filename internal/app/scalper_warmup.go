package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
	"github.com/mexc-bot/go-mexc-bot/internal/scalper"
)

// scalperWarmupDuration returns 0 to skip warmup, or duration from MEXC_SCALPER_WARMUP (e.g. 20s, 45s). Empty → 20s.
func scalperWarmupDuration() time.Duration {
	v := strings.TrimSpace(os.Getenv("MEXC_SCALPER_WARMUP"))
	if v == "0" || v == "false" || v == "off" {
		return 0
	}
	if v == "" {
		return 20 * time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 20 * time.Second
	}
	return d
}

// scalperWarmupProgressInterval returns 0 to disable interim warmup logs, or parse MEXC_SCALPER_WARMUP_PROGRESS (e.g. 1s, 3s). Empty → 2s.
func scalperWarmupProgressInterval() time.Duration {
	v := strings.TrimSpace(os.Getenv("MEXC_SCALPER_WARMUP_PROGRESS"))
	if v == "0" || v == "false" || v == "off" {
		return 0
	}
	if v == "" {
		return 2 * time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 2 * time.Second
	}
	return d
}

// RunScalperWarmupMetrics opens the same contract WS as the scalper, feeds BookState for sampleDuration,
// logs spread / churn / scores vs thresholds (no orders). Call before go runLiveScalper.
func (b *Bot) RunScalperWarmupMetrics(ctx context.Context, sampleDuration time.Duration) error {
	if b == nil || !b.cfg.Mode.Scalper || sampleDuration <= 0 {
		return nil
	}
	sym := b.cfg.Scalper.Symbol
	log.Printf("mexc-bot: warmup — метрики стакана %s, %v (ордера не ставятся; отключить: MEXC_SCALPER_WARMUP=0; промежуточные строки: MEXC_SCALPER_WARMUP_PROGRESS, по умолчанию 2s, 0=выкл)", sym, sampleDuration)

	ws, err := mexcfutures.NewContractWS(mexcfutures.WSConfig{PingInterval: 15 * time.Second})
	if err != nil {
		return fmt.Errorf("warmup ws: %w", err)
	}
	defer func() { _ = ws.Close() }()

	if err := ws.Connect(ctx); err != nil {
		return fmt.Errorf("warmup connect: %w", err)
	}
	if err := ws.SubscribeDepth(sym, false); err != nil {
		return fmt.Errorf("warmup sub.depth: %w", err)
	}
	if err := ws.SubscribeFullDepth(sym, 20); err != nil {
		return fmt.Errorf("warmup sub.depth.full: %w", err)
	}

	cfg := b.cfg.Scalper
	book := scalper.NewBookState(cfg)
	sig := scalper.NewSignalEngine(cfg)
	risk := scalper.NewRiskGuard(cfg)

	started := time.Now()
	until := started.Add(sampleDuration)
	progressEvery := scalperWarmupProgressInterval()
	nextProgress := started.Add(progressEvery)
	var (
		lastF  scalper.Features
		lastD  scalper.Decision
		hasObs bool // at least one book tick with HasBook
	)
	var (
		nBook                            int
		sumSpreadTicks                   float64
		minSp, maxSp                     = math.MaxFloat64, 0.0
		sumUpd, maxUpd                   float64
		nUpd                             int
		chaosN, staleN, wideN, volPauseN int
		scoreBelowN, enterOrAddN         int
		allowEntryN, denyKillN           int
		enterDeniedN                     int
		maxLong, maxShort                float64
		sumAbsImb                        float64
		rangeReadyN                      int
		rangeInsideN                     int
	)

	for time.Now().Before(until) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		nowLoop := time.Now()
		if progressEvery > 0 && !nowLoop.Before(nextProgress) {
			elapsed := nowLoop.Sub(started)
			if !hasObs {
				log.Printf("mexc-bot: warmup_progress elapsed=%.1fs/%v book_ticks=%d — ждём полный снимок стакана (depth.full + валидные уровни)",
					elapsed.Seconds(), sampleDuration, nBook)
			} else {
				allowed, deny, _ := risk.AllowEntry(nowLoop, lastF, nil)
				s := lastF.Snapshot
				log.Printf("mexc-bot: warmup_progress elapsed=%.1fs/%v book_ticks=%d bid=%.4f ask=%.4f mid=%.4f spr_ticks=%.2f upd/s=%.0f imb5=%.3f pressΔ=%.3f microΔ=%.5f corridor_ready=%v corridor=[%.4f..%.4f] mean=%.4f chaos=%v stale=%v wide=%v vol_pause=%v | signal=%s action=%s side=%s score=%.3f allow_entry=%v deny=%q | cfg min_score=%.2f max_upd=%.0f max_spr_ticks=%.1f",
					elapsed.Seconds(), sampleDuration, nBook,
					s.BestBidPx, s.BestAskPx, s.Mid, lastF.SpreadTicks, lastF.UpdateRate, s.Imbalance5, lastF.PressureDelta, lastF.MicroPriceDelta,
					lastF.HasPriceCorridor, lastF.PriceLowerBound, lastF.PriceUpperBound, lastF.PriceMean,
					lastF.Chaos, lastF.Stale, lastF.WideSpread, lastF.VolatilityPause,
					lastD.Reason, lastD.Action, lastD.Side, lastD.Score, allowed, deny,
					cfg.MinSignalScore, cfg.MaxUpdateRate, cfg.MaxSpreadTicks)
			}
			nextProgress = nowLoop.Add(progressEvery)
		}
		if err := ws.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			return err
		}
		mt, raw, err := ws.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			return err
		}
		messageJSON, ok := decodeWSJSON(mt, raw)
		if !ok {
			continue
		}
		var frame mexcfutures.WSFrame
		if err := json.Unmarshal(messageJSON, &frame); err != nil {
			continue
		}
		if frame.Channel != "push.depth" && !strings.HasPrefix(frame.Channel, "push.depth.full") {
			continue
		}
		now := time.Now().UTC()
		if !book.ApplyMessage(string(messageJSON), sym, frame.Channel, now) {
			continue
		}
		f := book.Features(now)
		if !f.Snapshot.HasBook {
			continue
		}
		nBook++
		st := f.SpreadTicks
		sumSpreadTicks += st
		minSp = math.Min(minSp, st)
		maxSp = math.Max(maxSp, st)
		if f.Chaos {
			chaosN++
		}
		if f.Stale {
			staleN++
		}
		if f.WideSpread {
			wideN++
		}
		if f.VolatilityPause {
			volPauseN++
		}
		if f.UpdateRate > 0 {
			sumUpd += f.UpdateRate
			nUpd++
			maxUpd = math.Max(maxUpd, f.UpdateRate)
		}
		sumAbsImb += math.Abs(f.Snapshot.Imbalance5)
		if f.HasPriceCorridor {
			rangeReadyN++
			if f.Snapshot.Mid > f.PriceLowerBound && f.Snapshot.Mid < f.PriceUpperBound {
				rangeInsideN++
			}
		}

		dec := sig.Evaluate(now, f, nil)
		hasObs = true
		lastF = f
		lastD = dec
		lg, sh := sig.WarmupScores(f)
		maxLong = math.Max(maxLong, lg)
		maxShort = math.Max(maxShort, sh)

		if dec.Reason == "score_below_threshold" {
			scoreBelowN++
		}
		if dec.Action == scalper.DecisionEnter || dec.Action == scalper.DecisionAddLadder {
			enterOrAddN++
		}
		allowed, deny, _ := risk.AllowEntry(now, f, nil)
		if allowed {
			allowEntryN++
		}
		if deny == "kill_switch" {
			denyKillN++
		}
		if (dec.Action == scalper.DecisionEnter || dec.Action == scalper.DecisionAddLadder) && !allowed {
			enterDeniedN++
		}
	}

	if nBook == 0 {
		log.Printf("mexc-bot: warmup: нет ни одного полного снимка стакана — проверь символ и WS")
		return nil
	}

	avgSp := sumSpreadTicks / float64(nBook)
	avgUpd := 0.0
	if nUpd > 0 {
		avgUpd = sumUpd / float64(nUpd)
	}
	avgAbsImb := sumAbsImb / float64(nBook)
	pChaos := 100 * float64(chaosN) / float64(nBook)
	pStale := 100 * float64(staleN) / float64(nBook)
	pWide := 100 * float64(wideN) / float64(nBook)
	pScoreBelow := 100 * float64(scoreBelowN) / float64(nBook)
	pEnter := 100 * float64(enterOrAddN) / float64(nBook)

	log.Printf("mexc-bot: warmup: samples=%d символ=%s", nBook, sym)
	log.Printf("mexc-bot: warmup: spread_ticks avg=%.3f min=%.3f max=%.3f (cfg MaxSpreadTicks=%.1f)", avgSp, minSp, maxSp, cfg.MaxSpreadTicks)
	log.Printf("mexc-bot: warmup: update_rate upd/s avg=%.0f max=%.0f (cfg MaxUpdateRate=%.0f)", avgUpd, maxUpd, cfg.MaxUpdateRate)
	log.Printf("mexc-bot: warmup: chaos=%.1f%% stale=%.1f%% wide_spread=%.1f%% vol_pause_ticks=%d", pChaos, pStale, pWide, volPauseN)
	log.Printf("mexc-bot: warmup: |imbalance5| avg=%.4f (cfg MinImbalance=%.3f MinImbalanceDelta=%.3f MinPressureDelta=%.3f)", avgAbsImb, cfg.MinImbalance, cfg.MinImbalanceDelta, cfg.MinPressureDelta)
	if cfg.PriceCorridorWindow > 0 {
		rangeReadyPct := 100 * float64(rangeReadyN) / float64(nBook)
		rangeInsidePct := 0.0
		if rangeReadyN > 0 {
			rangeInsidePct = 100 * float64(rangeInsideN) / float64(rangeReadyN)
		}
		log.Printf("mexc-bot: warmup: price_corridor ready=%.1f%% inside_range=%.1f%% (cfg Window=%v Percentile=%.2f MaxMultiplier=%.2f)",
			rangeReadyPct, rangeInsidePct, cfg.PriceCorridorWindow, cfg.PriceCorridorPercentile, cfg.PriceCorridorMaxMultiplier)
	}
	log.Printf("mexc-bot: warmup: score max_long=%.3f max_short=%.3f (cfg MinSignalScore=%.3f) ticks_score_below=%.1f%% ticks_enter_signal=%.1f%%", maxLong, maxShort, cfg.MinSignalScore, pScoreBelow, pEnter)
	log.Printf("mexc-bot: warmup: risk AllowEntry ok on %.1f%% ticks (kill_switch_denies=%d)", 100*float64(allowEntryN)/float64(nBook), denyKillN)

	// Короткие подсказки по конфигу (не жёсткие правила, ориентиры).
	if maxLong < cfg.MinSignalScore && maxShort < cfg.MinSignalScore {
		log.Printf("mexc-bot: warmup: подсказка — за окно ни разу не набрали MinSignalScore: попробуйте снизить MEXC_SCALPER_MIN_SIGNAL_SCORE или ослабить MEXC_SCALPER_MIN_IMBALANCE / MIN_PRESSURE_DELTA / MIN_IMBALANCE_DELTA")
	}
	if pChaos > 30 && maxUpd > cfg.MaxUpdateRate*0.9 {
		log.Printf("mexc-bot: warmup: подсказка — часто chaos по update_rate: поднимите MEXC_SCALPER_MAX_UPDATE_RATE (сейчас %.0f) или смотрите MEXC_SCALPER_DIAG=1", cfg.MaxUpdateRate)
	}
	if pStale > 15 {
		log.Printf("mexc-bot: warmup: подсказка — часто stale book: увеличьте MEXC_SCALPER_MAX_BOOK_AGE или проверьте задержку WS")
	}
	if pWide > 15 {
		log.Printf("mexc-bot: warmup: подсказка — часто wide spread: увеличьте MEXC_SCALPER_MAX_SPREAD_TICKS (сейчас %.1f) или смените время/символ", cfg.MaxSpreadTicks)
	}
	if enterOrAddN > 0 && enterDeniedN == enterOrAddN {
		log.Printf("mexc-bot: warmup: подсказка — сигнал на вход был, но Risk.AllowEntry всегда false (в т.ч. kill_switch=%d): см. MEXC_SCALPER_KILL_SWITCH и лимиты риска", denyKillN)
	}
	return nil
}
