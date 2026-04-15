package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// SourceWebKeyEnv — WEB-ключ для захвата рынка (REST проверка + WS → ClickHouse). Связка: MEXC_BOT_MODE=capture, MEXC_WS_SYMBOLS.
const SourceWebKeyEnv = "MEXC_SOURCE_WEB_KEY"

// TradeWebKeyEnv — WEB-ключ для торговли (приватный REST: ордера, позиции). Связка: MEXC_BOT_MODE=scalper; может отличаться от Source.
const TradeWebKeyEnv = "MEXC_WEB_KEY"

// WSSymbolEnv — один символ для WS-захвата (устарело, приоритет у MEXC_WS_SYMBOLS).
const WSSymbolEnv = "MEXC_WS_SYMBOL"

// MEXCWSymbolsEnv — список контрактов через запятую для WS depth+deal в capture. Связка: MEXC_SCALPER_SYMBOL обычно один из них.
const MEXCWSymbolsEnv = "MEXC_WS_SYMBOLS"

// BotModeEnv — какие подсистемы запускать: capture | scalper | replay, можно через запятую (например capture,scalper).
const BotModeEnv = "MEXC_BOT_MODE"

const (
	defaultSymbol       = "TAO_USDT"
	defaultBotMode      = "capture"
	defaultPositionMode = 1
	defaultOpenType     = 1
	defaultLeverage     = 2
	defaultScalperExit  = "bracket"
)

// RuntimeMode — флаги режимов процесса (включаются из MEXC_BOT_MODE).
type RuntimeMode struct {
	Capture bool // писать публичный WS в ClickHouse (ключ MEXC_SOURCE_WEB_KEY)
	Scalper bool // живая торговля/сигналы (ключ MEXC_WEB_KEY)
	Replay  bool // прогон истории из CH без live WS
}

// Scalper — параметры книжного скальпера (все MEXC_SCALPER_*).
type Scalper struct {
	// Контракт для скальпера (REST/WS торговли). Должен быть согласован с рынком (часто = первый символ из MEXC_WS_SYMBOLS).
	Symbol string
	// Размер тика цены для тиков спреда/TP-SL/квантования; связан с ProfitTargetTicks, StopLossTicks, MaxSpreadTicks.
	TickSize float64
	// Точность цены в JSON ордера (nil → из GET /contract/detail при старте). Перебивает авто; связка с TickSize при ошибке REST.
	OrderPriceScale *int
	// Точность объёма в JSON ордера (nil → из contract/detail). Связка: StepVolume, EffectiveStepVolume.
	OrderVolScale *int
	// Базовый объём шага лестницы (контракты), без плеча; фактический submit = EffectiveStepVolume() = StepVolume×Leverage.
	StepVolume float64
	// Сколько шагов лестницы максимум на один цикл. Связка: MinStepInterval между шагами.
	MaxLadderSteps int
	// Минимум времени между шагами лестницы. Связка: MaxLadderSteps, сигнал DecisionAddLadder.
	MinStepInterval time.Duration
	// TTL лимитного выхода (режим limit). В bracket почти не используется.
	ExitTTL time.Duration
	// Тайм-стоп удержания (для не-bracket выходов/логики риска). Связка: ExitMode=limit vs bracket.
	TimeStop time.Duration
	// Пауза после сброса цикла перед новым входом (фаза cooldown). Связка: стагнация сигналов после сделки.
	Cooldown time.Duration
	// Период опроса позиций/ордеров REST. Связка: bracket-синх, fill tracking.
	PollInterval time.Duration
	// Сколько держать несработавшие лимитные входы по коридору, затем отмена. Связка: коридорный вход.
	EntryLimitPendingTTL time.Duration
	// Минимальный интервал между перевыставлениями лимитного выхода (EnsureExit, не bracket).
	RepriceInterval time.Duration
	// Take-profit в тиках от входа (×TickSize). Связка: StopLossTicks, ExitMode=bracket → takeProfitPrice на бирже.
	ProfitTargetTicks int
	// Stop-loss в тиках от входа. Связка: ProfitTargetTicks, bracket → stopLossPrice.
	StopLossTicks int
	// Макс. ширина спреда в тиках «сейчас»; иначе сигнал/риск режет вход. Связка: TickSize, SpreadStabilityWindow.
	MaxSpreadTicks float64
	// Окно для MaxRecentSpreadTicks (волатильность спреда). Связка: MaxSpreadTicksInWindow.
	SpreadStabilityWindow time.Duration
	// Порог: если в окне спред был шире — отказ (spread_unstable). Связка: SpreadStabilityWindow, TickSize.
	MaxSpreadTicksInWindow float64
	// Книга старше этого возраста → stale / пауза. Связка: частота WS, FeatureLookback.
	MaxBookAge time.Duration
	// Верхняя граница смен снимков в окне FeatureLookback (анти-«хаос»). Связка: FeatureLookback.
	MaxUpdateRate float64
	// Длительность «заморозки» после риск-события (stale/spread/chaos). Связка: MaxBookAge, RiskGuard.
	VolatilityPause time.Duration
	// Окно микроструктуры (импульсы, update rate, предыдущий снимок). Связка: MaxUpdateRate, SignalConfirm*.
	FeatureLookback time.Duration
	// Окно подтверждения направления сигнала (счёт совпадающих тиков). Связка: SignalConfirmMinTicks, MinAge.
	SignalConfirmWindow time.Duration
	// Сколько подряд тиков с тем же доминирующим направлением нужно. Связка: SignalConfirmWindow, MinAge.
	SignalConfirmMinTicks int
	// Минимальный «возраст» подтверждения (время первого совпадения). Связка: SignalConfirmWindow.
	SignalConfirmMinAge time.Duration
	// Окно для коридора цены (квантиль mid). 0 = фильтр коридора выкл. Связка: Percentile, MaxMultiplier.
	PriceCorridorWindow time.Duration
	// Квантиль отклонений mid для границ коридора. Связка: PriceCorridorWindow.
	PriceCorridorPercentile float64
	// Насколько далеко от границы ещё «допустимо» для входа. Связка: Percentile, коридор в SignalEngine.
	PriceCorridorMaxMultiplier float64
	// Минимум отсчётов mid в окне коридора; меньше 2 приводится к 2.
	PriceCorridorMinSamples int
	// Период полураспада веса отсчётов при средней по коридору; 0 = равные веса.
	PriceCorridorMeanHalfLife time.Duration
	// Мин. |имбаланс5| для вклада в скоринг. Связка: MinImbalanceDelta, MinSignalScore.
	MinImbalance float64
	// Мин. изменение имбаланса между последним и текущим снимком (LastSnapshot). Связка: MinSignalScore.
	MinImbalanceDelta float64
	// Мин. давление (дельта имбаланса) для скоринга. Связка: MinImbalance, MinSignalScore.
	MinPressureDelta float64
	// Порог суммарного скора для входа long/short. Связка: все Min*, Pulse, microprice.
	MinSignalScore float64
	// Мин. импульс лучшей цены в тиках за lookback. Связка: FeatureLookback, TickSize.
	MinPulseTicks int
	// Выравнивание microprice с сигналом (в тиках). Связка: MaxMicroPriceTicks, TickSize.
	MinMicroPriceTicks float64
	// Верхняя граница microprice (не перегруженный вход). Связка: MinMicroPriceTicks.
	MaxMicroPriceTicks float64
	// Потолок вклада microprice в скор (в тиках); 0 = без ограничения. Связка: TickSize, longScore/shortScore.
	MaxMicroPriceScoreTicks float64
	// Пауза после deny stale_book (короче VolatilityPause, чтобы редкие дыры WS не блокировали вход надолго). 0 = брать VolatilityPause.
	StaleBookVolatilityPause time.Duration
	// Фильтр входа по агрессору сделок за окно (live/replay при наличии push.deal в потоке).
	EntryDealFilterEnabled bool
	// Окно суммирования signed deal volume (buy − sell). Связка: EntryDealFilterEnabled.
	EntryDealWindow time.Duration
	// Мин. |net vol| для признания направления ленты (0 = только знак >0 / <0).
	EntryDealMinSignedVol float64
	// Разрешить рыночный emergency flatten. Связка: риск ShouldFlatten.
	AllowEmergencyMarket bool
	// Полный запрет новых входов (всегда deny). Связка: диагностика, аварийный стоп.
	KillSwitch bool
	// Подробный лог скальпера раз в ~30с. Связка: только логирование.
	DiagLog bool
	// Сигнал long/short в журнале без изменений, а на бирже открывается противоположная сторона.
	InvertExecution bool
	// Выход: bracket — SL/TP на бирже при входе; limit — лимитный выход в коде. Связка: ProfitTargetTicks, StopLossTicks, PollInterval.
	ExitMode string
	// MEXC openType при сабмите. Связка: контракт/аккаунт.
	OpenType int
	// Режим позиций MEXC (one-way / hedge). Связка: ReduceOnly на выходах.
	PositionMode int
	// Плечо в JSON ордера; объём шага умножается на него в EffectiveStepVolume(). Связка: StepVolume.
	Leverage int
	// Начало окна replay (RFC3339). Связка: ReplayEnd, MEXC_BOT_MODE=replay.
	ReplayStart time.Time
	// Конец окна replay. Связка: ReplayStart.
	ReplayEnd time.Time
	// Макс. строк рынка из CH за прогон. Связка: replay-производительность.
	ReplayLimit int
	// Порог тиков для «цели» в replay-метриках. Связка: TickSize.
	ReplayTargetTicks int
	// Порог стоп-тиков в replay. Связка: TickSize.
	ReplayStopTicks int
	// Лимит времени удержания в replay-симуляции. Связка: логика выхода в replay.
	ReplayTimeStop time.Duration
}

// ExitUsesExchangeBracket — true если SL/TP уходят на биржу при входе (не режим limit).
func (s Scalper) ExitUsesExchangeBracket() bool {
	v := strings.ToLower(strings.TrimSpace(s.ExitMode))
	return v != "limit"
}

// EffectiveStepVolume — объём в заявку: StepVolume × Leverage (если Leverage<1, считается 1).
func (s Scalper) EffectiveStepVolume() float64 {
	lev := s.Leverage
	if lev < 1 {
		lev = 1
	}
	return s.StepVolume * float64(lev)
}

// Bot — ключи и режимы процесса (часть из .env, часть из MEXC_BOT_MODE / MEXC_WS_SYMBOLS).
type Bot struct {
	SourceWebKey string   // MEXC_SOURCE_WEB_KEY — только capture/данные
	TradeWebKey  string   // MEXC_WEB_KEY — только scalper/торговля
	WSSymbols    []string // MEXC_WS_SYMBOLS (+ legacy MEXC_WS_SYMBOL)
	Mode         RuntimeMode
	Scalper      Scalper // все MEXC_SCALPER_*
}

// Load — читает .env из cwd и собирает Bot; проверяет обязательные ключи под выбранный Mode.
func Load() (Bot, error) {
	_ = godotenv.Load()
	mode := ParseRuntimeMode()
	source := strings.TrimSpace(os.Getenv(SourceWebKeyEnv))
	trade := strings.TrimSpace(os.Getenv(TradeWebKeyEnv))
	if mode.Capture && source == "" {
		return Bot{}, fmt.Errorf("config: %s is required when capture is enabled", SourceWebKeyEnv)
	}
	if mode.Scalper && trade == "" {
		return Bot{}, fmt.Errorf("config: %s is required when scalper is enabled", TradeWebKeyEnv)
	}
	return Bot{
		SourceWebKey: source,
		TradeWebKey:  trade,
		WSSymbols:    ParseWSSymbols(),
		Mode:         mode,
		Scalper:      ScalperFromEnv(),
	}, nil
}

// ParseWSSymbols — список символов для capture WS (MEXC_WS_SYMBOLS или MEXC_WS_SYMBOL).
func ParseWSSymbols() []string {
	raw := strings.TrimSpace(os.Getenv(MEXCWSymbolsEnv))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv(WSSymbolEnv))
	}
	if raw == "" {
		return []string{"TAO_USDT"}
	}
	seen := make(map[string]struct{})
	var out []string
	for _, part := range strings.Split(raw, ",") {
		s := strings.TrimSpace(part)
		if s == "" {
			continue
		}
		key := strings.ToUpper(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return []string{"TAO_USDT"}
	}
	return out
}

// ParseRuntimeMode — парсит MEXC_BOT_MODE (через запятую); если пусто — только capture.
func ParseRuntimeMode() RuntimeMode {
	raw := strings.TrimSpace(os.Getenv(BotModeEnv))
	if raw == "" {
		raw = defaultBotMode
	}
	mode := RuntimeMode{}
	for _, part := range strings.Split(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "capture":
			mode.Capture = true
		case "scalper":
			mode.Scalper = true
		case "replay":
			mode.Replay = true
		}
	}
	if !mode.Capture && !mode.Scalper && !mode.Replay {
		mode.Capture = true
	}
	return mode
}

// ScalperFromEnv — читает MEXC_SCALPER_*; дефолты можно переопределить .env.
func ScalperFromEnv() Scalper {
	start, _ := parseTimeEnv("MEXC_SCALPER_REPLAY_START")
	end, _ := parseTimeEnv("MEXC_SCALPER_REPLAY_END")
	return Scalper{
		Symbol:                     getenvString("MEXC_SCALPER_SYMBOL", defaultSymbol),                           // контракт скальпера
		TickSize:                   getenvFloat("MEXC_SCALPER_TICK_SIZE", 0.01),                                  // тик цены → тики спреда/TP/SL
		OrderPriceScale:            getenvIntPtr("MEXC_SCALPER_ORDER_PRICE_SCALE"),                               // nil = из REST contract/detail
		OrderVolScale:              getenvIntPtr("MEXC_SCALPER_ORDER_VOL_SCALE"),                                 // nil = из REST
		StepVolume:                 getenvFloat("MEXC_SCALPER_STEP_VOLUME", 3),                                   // базовый объём шага; ×Leverage в заявку
		MaxLadderSteps:             getenvInt("MEXC_SCALPER_MAX_LADDER_STEPS", 1),                                // глубина лестницы
		MinStepInterval:            getenvDuration("MEXC_SCALPER_MIN_STEP_INTERVAL", 150*time.Millisecond),       // пауза между шагами лестницы
		ExitTTL:                    getenvDuration("MEXC_SCALPER_EXIT_TTL", 900*time.Millisecond),                // жизнь лимита выхода (не bracket)
		TimeStop:                   getenvDuration("MEXC_SCALPER_TIME_STOP", 5*time.Second),                      // тайм-стоп удержания (не bracket SL)
		Cooldown:                   getenvDuration("MEXC_SCALPER_COOLDOWN", 300*time.Millisecond),                // пауза перед новым циклом
		PollInterval:               getenvDuration("MEXC_SCALPER_POLL_INTERVAL", 500*time.Millisecond),           // REST опрос позиций/ордеров
		EntryLimitPendingTTL:       getenvDuration("MEXC_SCALPER_ENTRY_LIMIT_TTL", 12*time.Second),                 // отмена висящих лимитов входа по коридору
		RepriceInterval:            getenvDuration("MEXC_SCALPER_REPRICE_INTERVAL", 1000*time.Millisecond),       // троттлинг перевыставления лимитного выхода
		ProfitTargetTicks:          getenvInt("MEXC_SCALPER_PROFIT_TARGET_TICKS", 5),                             // TP в тиках; с TickSize и bracket
		StopLossTicks:              getenvInt("MEXC_SCALPER_STOP_LOSS_TICKS", 2),                                 // SL в тиках; с ProfitTargetTicks
		MaxSpreadTicks:             getenvFloat("MEXC_SCALPER_MAX_SPREAD_TICKS", 2),                              // макс. спред «сейчас» (тиках)
		SpreadStabilityWindow:      getenvDuration("MEXC_SCALPER_SPREAD_STABILITY_WINDOW", 450*time.Millisecond), // окно max спреда; с MaxSpreadTicksInWindow
		MaxSpreadTicksInWindow:     getenvFloat("MEXC_SCALPER_MAX_SPREAD_TICKS_IN_WINDOW", 2.0),                  // порог «нестабильного» спреда
		MaxBookAge:                 getenvDuration("MEXC_SCALPER_MAX_BOOK_AGE", 1100*time.Millisecond),           // ~2× типичный p99 интервала depth; старение книги → stale
		MaxUpdateRate:              getenvFloat("MEXC_SCALPER_MAX_UPDATE_RATE", 1000),                            // анти-хаос: слишком много апдейтов/с
		VolatilityPause:            getenvDuration("MEXC_SCALPER_VOLATILITY_PAUSE", 5*time.Second),               // длительность vol-паузы после риска
		FeatureLookback:            getenvDuration("MEXC_SCALPER_FEATURE_LOOKBACK", 550*time.Millisecond),        // окно фич микроструктуры
		SignalConfirmWindow:        getenvDuration("MEXC_SCALPER_SIGNAL_CONFIRM_WINDOW", 700*time.Millisecond),   // окно подтверждения сигнала
		SignalConfirmMinTicks:      getenvInt("MEXC_SCALPER_SIGNAL_CONFIRM_MIN_TICKS", 2),                        // мин. совпадающих тиков в окне
		SignalConfirmMinAge:        getenvDuration("MEXC_SCALPER_SIGNAL_CONFIRM_MIN_AGE", 120*time.Millisecond),  // мин. «возраст» подтверждения
		PriceCorridorWindow:        getenvDuration("MEXC_SCALPER_PRICE_CORRIDOR_WINDOW", 15*time.Second),         // окно коридора mid; 0 = выкл.
		PriceCorridorPercentile:    getenvFloat("MEXC_SCALPER_PRICE_CORRIDOR_PERCENTILE", 0.80),                  // ширина коридора по квантилю
		PriceCorridorMaxMultiplier: getenvFloat("MEXC_SCALPER_PRICE_CORRIDOR_MAX_MULTIPLIER", 2.0),               // допуск от границы коридора
		PriceCorridorMinSamples:    getenvInt("MEXC_SCALPER_PRICE_CORRIDOR_MIN_SAMPLES", 5),                      // минимум точек mid в окне
		PriceCorridorMeanHalfLife:  getenvDuration("MEXC_SCALPER_PRICE_CORRIDOR_MEAN_HALF_LIFE", 0),              // 0 = среднее без убывания веса по времени
		MinImbalance:               getenvFloat("MEXC_SCALPER_MIN_IMBALANCE", 0.14),                              // порог имбаланса в скоре
		MinImbalanceDelta:          getenvFloat("MEXC_SCALPER_MIN_IMBALANCE_DELTA", 0.055),                       // порог Δимбаланса
		MinPressureDelta:           getenvFloat("MEXC_SCALPER_MIN_PRESSURE_DELTA", 0.09),                         // порог давления
		MinSignalScore:             getenvFloat("MEXC_SCALPER_MIN_SIGNAL_SCORE", 1.5),                            // суммарный скор для входа
		MinPulseTicks:              getenvInt("MEXC_SCALPER_MIN_PULSE_TICKS", 1),                                 // мин. импульс bid/ask в тиках
		MinMicroPriceTicks:         getenvFloat("MEXC_SCALPER_MIN_MICROPRICE_TICKS", 0.02),                       // выравнивание microprice
		MaxMicroPriceTicks:         getenvFloat("MEXC_SCALPER_MAX_MICROPRICE_TICKS", 2.0),                        // потолок microprice
		MaxMicroPriceScoreTicks:    getenvFloat("MEXC_SCALPER_MAX_MICROPRICE_SCORE_TICKS", 5),                    // вклад microprice в скор (тиках); 0 = без капа
		StaleBookVolatilityPause:   getenvDuration("MEXC_SCALPER_STALE_BOOK_PAUSE", 400*time.Millisecond),        // 0s в env → полная VolatilityPause (см. RiskGuard)
		EntryDealFilterEnabled:     getenvBool("MEXC_SCALPER_ENTRY_DEAL_FILTER", false),                          // сделки buy−sell за окно согласованы с сигналом
		EntryDealWindow:            getenvDuration("MEXC_SCALPER_ENTRY_DEAL_WINDOW", time.Second),                // окно ленты сделок
		EntryDealMinSignedVol:      getenvFloat("MEXC_SCALPER_ENTRY_DEAL_MIN_SIGNED_VOL", 0),                     // мин. объём для направления; 0 = только строгий знак
		AllowEmergencyMarket:       getenvBool("MEXC_SCALPER_EMERGENCY_MARKET", true),                            // рынок при emergency flatten
		KillSwitch:                 getenvBool("MEXC_SCALPER_KILL_SWITCH", false),                                // стоп всех новых входов
		DiagLog:                    getenvBool("MEXC_SCALPER_DIAG", false),                                       // расширенный лог тика
		InvertExecution:            getenvBool("MEXC_SCALPER_INVERT_EXECUTION", false),                           // исполнить против сигнала
		ExitMode:                   getenvString("MEXC_SCALPER_EXIT_MODE", defaultScalperExit),                   // bracket | limit
		OpenType:                   getenvInt("MEXC_SCALPER_OPEN_TYPE", defaultOpenType),                         // MEXC openType
		PositionMode:               getenvInt("MEXC_SCALPER_POSITION_MODE", defaultPositionMode),                 // one-way / hedge
		Leverage:                   getenvInt("MEXC_SCALPER_LEVERAGE", defaultLeverage),                          // плечо в JSON; множитель к StepVolume
		ReplayStart:                start,                                                                        // начало окна replay (RFC3339)
		ReplayEnd:                  end,                                                                          // конец окна replay
		ReplayLimit:                getenvInt("MEXC_SCALPER_REPLAY_LIMIT", 50000),                                // лимит строк CH за прогон
		ReplayTargetTicks:          getenvInt("MEXC_SCALPER_REPLAY_TARGET_TICKS", 2),                             // целевые тики в replay-метриках
		ReplayStopTicks:            getenvInt("MEXC_SCALPER_REPLAY_STOP_TICKS", 1),                               // стоп-тики в replay
		ReplayTimeStop:             getenvDuration("MEXC_SCALPER_REPLAY_TIME_STOP", 5*time.Second),               // тайм-стоп replay-сессии
	}
}

func getenvString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

// getenvIntPtr parses an int env var; empty or invalid returns nil (caller treats as unset).
func getenvIntPtr(key string) *int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &v
}

func getenvFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return v
}

func getenvBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return v
}

func parseTimeEnv(key string) (time.Time, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Time{}, false
	}
	v, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return v.UTC(), true
}
