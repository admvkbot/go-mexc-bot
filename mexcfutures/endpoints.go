package mexcfutures

// Futures (mexc-futures-sdk) paths relative to FuturesBaseURL.
const (
	pathSubmitOrder              = "/private/order/submit"
	pathPlaceOrderCreate         = "/private/order/create"
	pathCancelOrder              = "/private/order/cancel"
	pathCancelOrderByExternalID  = "/private/order/cancel_with_external"
	pathCancelAllOrders          = "/private/order/cancel_all"
	pathOrderHistory             = "/private/order/list/history_orders"
	pathOrderDeals               = "/private/order/list/order_deals"
	pathGetOrder                 = "/private/order/get"
	pathGetOrderByExternalPrefix = "/private/order/external"
	pathRiskLimit                = "/private/account/risk_limit"
	pathFeeRate                  = "/private/account/contract/fee_rate"
	pathAccountAsset             = "/private/account/asset"
	pathOpenPositionsFutures     = "/private/position/open_positions"
	pathPositionHistory          = "/private/position/list/history_positions"
	pathTicker                   = "/contract/ticker"
	pathContractDetail           = "/contract/detail"
	pathContractDepth            = "/contract/depth"
)

// Contract host (Python mexc_client) paths relative to ContractBaseURL.
const (
	pathOpenPositionsContract = "/private/position/open_positions"
	pathContractDetailPublic  = "/contract/detail"
)
