package mexcfutures

// SubmitOrderRequest mirrors mexc-futures-sdk SubmitOrderRequest (POST /private/order/submit).
type SubmitOrderRequest struct {
	Symbol          string  `json:"symbol"`
	Price           float64 `json:"price"`
	Vol             float64 `json:"vol"`
	Side            int     `json:"side"`
	Type            int     `json:"type"`
	OpenType        int     `json:"openType"`
	Leverage        int     `json:"leverage,omitempty"`
	PositionID      int64   `json:"positionId,omitempty"`
	ExternalOid     string  `json:"externalOid,omitempty"`
	StopLossPrice   float64 `json:"stopLossPrice,omitempty"`
	TakeProfitPrice float64 `json:"takeProfitPrice,omitempty"`
	PositionMode    int     `json:"positionMode,omitempty"`
	ReduceOnly      bool    `json:"reduceOnly,omitempty"`
	STPMode         int     `json:"stpMode,omitempty"`
	MarketCeiling   bool    `json:"marketCeiling,omitempty"`
	FlashClose      bool    `json:"flashClose,omitempty"`
}

// PlaceOrderCreateRequest mirrors Python place_order (POST /private/order/create).
type PlaceOrderCreateRequest struct {
	Symbol       string  `json:"symbol"`
	Side         int     `json:"side"`
	OpenType     int     `json:"openType"`
	Type         string  `json:"type"`
	Vol          float64 `json:"vol"`
	Leverage     int     `json:"leverage"`
	Price        float64 `json:"price"`
	PriceProtect string  `json:"priceProtect"`
}

// OpenMarketPositionRequest mirrors Python open_market_position payload.
type OpenMarketPositionRequest struct {
	Symbol          string `json:"symbol"`
	Side            int    `json:"side"`
	OpenType        int    `json:"openType"`
	Type            int    `json:"type"`
	Vol             string `json:"vol"`
	Leverage        int    `json:"leverage"`
	Price           string `json:"price"`
	PositionMode    int    `json:"positionMode"`
	StopLossPrice   string `json:"stopLossPrice,omitempty"`
	TakeProfitPrice string `json:"takeProfitPrice,omitempty"`
}

// CancelOrderByExternalIDRequest matches TS CancelOrderByExternalIdRequest.
type CancelOrderByExternalIDRequest struct {
	Symbol      string `json:"symbol"`
	ExternalOid string `json:"externalOid"`
}

// CancelAllOrdersRequest matches TS CancelAllOrdersRequest.
type CancelAllOrdersRequest struct {
	Symbol string `json:"symbol,omitempty"`
}

// OrderHistoryParams matches TS OrderHistoryParams (query on futures host).
type OrderHistoryParams struct {
	Category int    `json:"category"`
	PageNum  int    `json:"page_num"`
	PageSize int    `json:"page_size"`
	States   int    `json:"states"`
	Symbol   string `json:"symbol"`
}

// OrderDealsParams matches TS OrderDealsParams.
type OrderDealsParams struct {
	Symbol    string `json:"symbol"`
	StartTime int64  `json:"start_time,omitempty"`
	EndTime   int64  `json:"end_time,omitempty"`
	PageNum   int    `json:"page_num"`
	PageSize  int    `json:"page_size"`
}

// PositionHistoryParams matches TS PositionHistoryParams.
type PositionHistoryParams struct {
	Symbol   string `json:"symbol,omitempty"`
	Type     int    `json:"type,omitempty"`
	PageNum  int    `json:"page_num"`
	PageSize int    `json:"page_size"`
}

const (
	OrderSideOpenLong   = 1
	OrderSideCloseShort = 2
	OrderSideOpenShort  = 3
	OrderSideCloseLong  = 4

	OrderTypeLimit    = 1
	OrderTypePostOnly = 2
	OrderTypeIOC      = 3
	OrderTypeFOK      = 4
	OrderTypeMarket   = 5

	OpenTypeIsolated = 1
	OpenTypeCross    = 2

	PositionModeDualSide = 1
	PositionModeOneWay   = 2

	OrderStatePending  = 1
	OrderStateUnfilled = 2
	OrderStateFilled   = 3
	OrderStateCanceled = 4
	OrderStateInvalid  = 5
)
