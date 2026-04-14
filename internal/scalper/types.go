package scalper

import "time"

type Side string

const (
	SideNone  Side = ""
	SideLong  Side = "long"
	SideShort Side = "short"
)

type DecisionAction string

const (
	DecisionHold       DecisionAction = "hold"
	DecisionEnter      DecisionAction = "enter"
	DecisionAddLadder  DecisionAction = "add_ladder"
	DecisionManageExit DecisionAction = "manage_exit"
	DecisionFlatten    DecisionAction = "flatten"
)

type TradePhase string

const (
	PhaseIdle             TradePhase = "idle"
	PhaseEntryPending     TradePhase = "entry_pending"
	PhasePartiallyFilled  TradePhase = "partially_filled"
	PhaseInventoryOpen    TradePhase = "inventory_open"
	PhaseExitPending      TradePhase = "exit_pending"
	PhaseEmergencyFlatten TradePhase = "emergency_flatten"
	PhaseCooldown         TradePhase = "cooldown"
)

type OrderClass string

const (
	OrderClassEntry     OrderClass = "entry"
	OrderClassExit      OrderClass = "exit"
	OrderClassEmergency OrderClass = "emergency"
)

type Level struct {
	Price  float64
	Volume float64
	Orders int32
}

type Snapshot struct {
	Symbol       string
	At           time.Time
	BestBidPx    float64
	BestBidVol   float64
	BestAskPx    float64
	BestAskVol   float64
	Mid          float64
	Spread       float64
	BidVol5      float64
	AskVol5      float64
	BidVol10     float64
	AskVol10     float64
	Imbalance5   float64
	HasBook      bool
	TopBids      []Level
	TopAsks      []Level
	UpdateSeq    int64
	ExchangeTSMs int64
}

type Features struct {
	Snapshot         Snapshot
	Previous         Snapshot
	BookAge          time.Duration
	Window           time.Duration
	UpdateRate       float64
	BidPulseTicks    int
	AskPulseTicks    int
	PressureDelta    float64
	SpreadTicks      float64
	MicroPriceDelta  float64
	VolatilityPause  bool
	HasLookback      bool
	Stale            bool
	WideSpread       bool
	Chaos            bool
	SignalImbalance  float64
	SignalSide       Side
	SignalConfidence float64
}

type Decision struct {
	Action      DecisionAction
	Side        Side
	Score       float64
	Reason      string
	TargetTicks int
	StopTicks   int
	Features    Features
}

type ManagedOrder struct {
	Class       OrderClass
	Side        Side
	ExternalOID string
	OrderID     string
	Price       float64
	Quantity    float64
	FilledQty   float64
	AvgFillPx   float64
	StateCode   int
	SubmittedAt time.Time
	LastUpdate  time.Time
	Reprices    int
	ReduceOnly  bool
	Reason      string
}

type LadderContext struct {
	SessionID         string
	LadderID          string
	Symbol            string
	Side              Side
	Phase             TradePhase
	EntryOrders       []*ManagedOrder
	ExitOrder         *ManagedOrder
	EmergencyOrder    *ManagedOrder
	StepCount         int
	MaxSteps          int
	NetQuantity       float64
	AvgEntryPrice     float64
	RealizedPnL       float64
	EntryStartedAt    time.Time
	EntryFilledAt     time.Time
	ExitStartedAt     time.Time
	ExitFilledAt      time.Time
	LastStepAt        time.Time
	LastDecisionAt    time.Time
	CooldownUntil     time.Time
	MaxFavorableTicks float64
	MaxAdverseTicks   float64
	RepricesCount     int
	ExitReason        string
	FlattenReason     string
	WasEmergencyExit  bool
	LastSignalReason  string
	RoundTripWritten  bool
}

func (c *LadderContext) HasInventory() bool {
	return c != nil && c.NetQuantity > 0
}

func (c *LadderContext) ActiveOrders() []*ManagedOrder {
	if c == nil {
		return nil
	}
	out := make([]*ManagedOrder, 0, len(c.EntryOrders)+2)
	for _, ord := range c.EntryOrders {
		if ord != nil {
			out = append(out, ord)
		}
	}
	if c.ExitOrder != nil {
		out = append(out, c.ExitOrder)
	}
	if c.EmergencyOrder != nil {
		out = append(out, c.EmergencyOrder)
	}
	return out
}

type SignalEvent struct {
	SessionID      string
	Mode           string
	Symbol         string
	EventAt        time.Time
	Action         string
	Side           string
	Score          float64
	Reason         string
	BestBidPx      float64
	BestAskPx      float64
	Spread         float64
	BidVol5        float64
	AskVol5        float64
	Imbalance5     float64
	BidPulseTicks  int
	AskPulseTicks  int
	PressureDelta  float64
	UpdateRate     float64
	MicroPriceDiff float64
}

type OrderEvent struct {
	SessionID   string
	LadderID    string
	Symbol      string
	EventAt     time.Time
	EventType   string
	OrderClass  string
	Side        string
	ExternalOID string
	OrderID     string
	Price       float64
	Quantity    float64
	FilledQty   float64
	AvgFillPx   float64
	StateCode   int
	Reason      string
	RawJSON     string
}

type RoundTrip struct {
	SessionID           string
	LadderID            string
	Symbol              string
	Side                string
	EntryStartedAt      time.Time
	EntryFilledAt       time.Time
	ExitStartedAt       time.Time
	ExitFilledAt        time.Time
	HoldingMS           int64
	EntryAvgPx          float64
	ExitAvgPx           float64
	FilledQty           float64
	GrossPnL            float64
	PNLTicks            float64
	MaxAdverseTicks     float64
	MaxFavorableTicks   float64
	ExitReason          string
	FlattenReason       string
	RepricesCount       int
	LadderStepsUsed     int
	WasEmergencyFlatten bool
}

type ReplayCandidate struct {
	SessionID     string
	Symbol        string
	SignalAt      time.Time
	Side          string
	Score         float64
	Reason        string
	EntryPx       float64
	TargetPx      float64
	StopPx        float64
	ExpireAt      time.Time
	StepCount     int
	BestBidPx     float64
	BestAskPx     float64
	Imbalance5    float64
	PressureDelta float64
}
