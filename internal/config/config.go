package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// WebKeyEnv is the environment variable holding the MEXC browser WEB session string.
const WebKeyEnv = "MEXC_SOURCE_WEB_KEY"

// WSSymbolEnv is a single-symbol fallback for public WebSocket market capture (deprecated in favour of MEXC_WS_SYMBOLS).
const WSSymbolEnv = "MEXC_WS_SYMBOL"

// MEXCWSymbolsEnv is a comma-separated list of futures contract symbols for WS market capture (order book + deals).
const MEXCWSymbolsEnv = "MEXC_WS_SYMBOLS"

// BotModeEnv controls which runtime blocks are enabled.
// Supported values: capture, scalper, replay, capture,scalper.
const BotModeEnv = "MEXC_BOT_MODE"

const (
	defaultSymbol       = "TAO_USDT"
	defaultBotMode      = "capture"
	defaultPositionMode = 1
	defaultOpenType     = 1
	defaultLeverage     = 1
)

// RuntimeMode describes which bot loops are enabled.
type RuntimeMode struct {
	Capture bool
	Scalper bool
	Replay  bool
}

// Scalper holds runtime settings for the book-only scalper.
type Scalper struct {
	Symbol               string
	TickSize             float64
	StepVolume           float64
	MaxLadderSteps       int
	MinStepInterval      time.Duration
	EntryTTL             time.Duration
	ExitTTL              time.Duration
	TimeStop             time.Duration
	Cooldown             time.Duration
	PollInterval         time.Duration
	RepriceInterval      time.Duration
	MaxReprices          int
	ProfitTargetTicks    int
	StopLossTicks        int
	MaxSpreadTicks       float64
	MaxBookAge           time.Duration
	MaxUpdateRate        float64
	VolatilityPause      time.Duration
	FeatureLookback      time.Duration
	MinImbalance         float64
	MinImbalanceDelta    float64
	MinPressureDelta     float64
	MinSignalScore       float64
	MinPulseTicks        int
	MaxInventoryNotional float64
	AllowEmergencyMarket bool
	KillSwitch           bool
	OpenType             int
	PositionMode         int
	Leverage             int
	ReplayStart          time.Time
	ReplayEnd            time.Time
	ReplayLimit          int
	ReplayTargetTicks    int
	ReplayStopTicks      int
	ReplayTimeStop       time.Duration
}

// Bot holds runtime settings for the trading bot process.
type Bot struct {
	WebKey    string
	WSSymbols []string
	Mode      RuntimeMode
	Scalper   Scalper
}

// Load reads optional .env from the working directory and returns Bot configuration.
func Load() (Bot, error) {
	_ = godotenv.Load()
	k := strings.TrimSpace(os.Getenv(WebKeyEnv))
	if k == "" {
		return Bot{}, fmt.Errorf("config: %s is not set", WebKeyEnv)
	}
	return Bot{
		WebKey:    k,
		WSSymbols: ParseWSSymbols(),
		Mode:      ParseRuntimeMode(),
		Scalper:   ScalperFromEnv(),
	}, nil
}

// ParseWSSymbols reads MEXC_WS_SYMBOLS (comma-separated), then legacy MEXC_WS_SYMBOL, defaulting to TAO_USDT.
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

// ParseRuntimeMode reads MEXC_BOT_MODE and enables requested runtime blocks.
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

// ScalperFromEnv reads book-only scalper runtime settings.
func ScalperFromEnv() Scalper {
	start, _ := parseTimeEnv("MEXC_SCALPER_REPLAY_START")
	end, _ := parseTimeEnv("MEXC_SCALPER_REPLAY_END")
	return Scalper{
		Symbol:               getenvString("MEXC_SCALPER_SYMBOL", defaultSymbol),
		TickSize:             getenvFloat("MEXC_SCALPER_TICK_SIZE", 0.01),
		StepVolume:           getenvFloat("MEXC_SCALPER_STEP_VOLUME", 1),
		MaxLadderSteps:       getenvInt("MEXC_SCALPER_MAX_LADDER_STEPS", 3),
		MinStepInterval:      getenvDuration("MEXC_SCALPER_MIN_STEP_INTERVAL", 350*time.Millisecond),
		EntryTTL:             getenvDuration("MEXC_SCALPER_ENTRY_TTL", 1500*time.Millisecond),
		ExitTTL:              getenvDuration("MEXC_SCALPER_EXIT_TTL", 1200*time.Millisecond),
		TimeStop:             getenvDuration("MEXC_SCALPER_TIME_STOP", 4*time.Second),
		Cooldown:             getenvDuration("MEXC_SCALPER_COOLDOWN", 800*time.Millisecond),
		PollInterval:         getenvDuration("MEXC_SCALPER_POLL_INTERVAL", 250*time.Millisecond),
		RepriceInterval:      getenvDuration("MEXC_SCALPER_REPRICE_INTERVAL", 450*time.Millisecond),
		MaxReprices:          getenvInt("MEXC_SCALPER_MAX_REPRICES", 4),
		ProfitTargetTicks:    getenvInt("MEXC_SCALPER_PROFIT_TARGET_TICKS", 2),
		StopLossTicks:        getenvInt("MEXC_SCALPER_STOP_LOSS_TICKS", 2),
		MaxSpreadTicks:       getenvFloat("MEXC_SCALPER_MAX_SPREAD_TICKS", 2),
		MaxBookAge:           getenvDuration("MEXC_SCALPER_MAX_BOOK_AGE", 1200*time.Millisecond),
		MaxUpdateRate:        getenvFloat("MEXC_SCALPER_MAX_UPDATE_RATE", 180),
		VolatilityPause:      getenvDuration("MEXC_SCALPER_VOLATILITY_PAUSE", 6*time.Second),
		FeatureLookback:      getenvDuration("MEXC_SCALPER_FEATURE_LOOKBACK", 400*time.Millisecond),
		MinImbalance:         getenvFloat("MEXC_SCALPER_MIN_IMBALANCE", 0.18),
		MinImbalanceDelta:    getenvFloat("MEXC_SCALPER_MIN_IMBALANCE_DELTA", 0.08),
		MinPressureDelta:     getenvFloat("MEXC_SCALPER_MIN_PRESSURE_DELTA", 0.12),
		MinSignalScore:       getenvFloat("MEXC_SCALPER_MIN_SIGNAL_SCORE", 1.7),
		MinPulseTicks:        getenvInt("MEXC_SCALPER_MIN_PULSE_TICKS", 1),
		MaxInventoryNotional: getenvFloat("MEXC_SCALPER_MAX_INVENTORY_NOTIONAL", 3500),
		AllowEmergencyMarket: getenvBool("MEXC_SCALPER_EMERGENCY_MARKET", true),
		KillSwitch:           getenvBool("MEXC_SCALPER_KILL_SWITCH", false),
		OpenType:             getenvInt("MEXC_SCALPER_OPEN_TYPE", defaultOpenType),
		PositionMode:         getenvInt("MEXC_SCALPER_POSITION_MODE", defaultPositionMode),
		Leverage:             getenvInt("MEXC_SCALPER_LEVERAGE", defaultLeverage),
		ReplayStart:          start,
		ReplayEnd:            end,
		ReplayLimit:          getenvInt("MEXC_SCALPER_REPLAY_LIMIT", 50000),
		ReplayTargetTicks:    getenvInt("MEXC_SCALPER_REPLAY_TARGET_TICKS", 2),
		ReplayStopTicks:      getenvInt("MEXC_SCALPER_REPLAY_STOP_TICKS", 2),
		ReplayTimeStop:       getenvDuration("MEXC_SCALPER_REPLAY_TIME_STOP", 4*time.Second),
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
