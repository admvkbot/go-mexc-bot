# Ордера (futures.mexc.com)

Все пути ниже относительны к **`FuturesBaseURL`** (по умолчанию `https://futures.mexc.com/api/v1`). Ответы — `[]byte` (JSON), если не указано иное.

## Размещение ордеров

### `SubmitOrder` — `POST /private/order/submit`

Соответствует форме **mexc-futures-sdk** `SubmitOrderRequest`.

```go
body, err := c.SubmitOrder(ctx, mexcfutures.SubmitOrderRequest{
	Symbol:   "BTC_USDT",
	Price:    0,
	Vol:      0.001,
	Side:     1,             // код стороны биржи
	Type:     5,             // тип ордера
	OpenType: 1,             // рыночный/лимитный и т.д. — по документации MEXC
	Leverage: 10,
	// необязательные поля:
	// PositionID, ExternalOid, StopLossPrice, TakeProfitPrice, PositionMode, ReduceOnly
})
```

Поле `Type` в структуре — **`int`** (в отличие от `PlaceOrderCreate`, где тип — строка).

### `PlaceOrderCreate` — `POST /private/order/create`

Соответствует Python **`mexc_client.place_order`**.

```go
body, err := c.PlaceOrderCreate(ctx, mexcfutures.PlaceOrderCreateRequest{
	Symbol:       "BTC_USDT",
	Side:         1,
	OpenType:     1,
	Type:         "5",       // строковый код типа
	Vol:          0.001,
	Leverage:     10,
	Price:        0,
	PriceProtect: "0",
})
```

### `OpenMarketPosition` — тот же путь, что и `SubmitOrder`

Удобная обёртка: **`POST /private/order/submit`** с телом **`OpenMarketPositionRequest`** (как в Python `open_market_position`).

```go
sym := "BTC_USDT"
body, err := c.OpenMarketPosition(ctx, mexcfutures.OpenMarketPositionRequest{
	Symbol:       sym,
	Side:         1,
	OpenType:     1,
	Type:         5,
	Vol:          "0.001",   // объём строкой
	Leverage:     10,
	Price:        "0",
	PositionMode: 1,
})
```

## Отмена ордеров

### `CancelOrder` — `POST /private/order/cancel`

Тело запроса — **JSON-массив до 50 целочисленных ID** ордеров.

```go
body, err := c.CancelOrder(ctx, []int64{123456789})
```

Ошибки клиента до HTTP:

- пустой слайс `orderIDs`;
- больше 50 элементов.

### `CancelOrderByExternalID` — `POST /private/order/cancel_with_external`

```go
body, err := c.CancelOrderByExternalID(ctx, mexcfutures.CancelOrderByExternalIDRequest{
	Symbol:      "BTC_USDT",
	ExternalOid: "my-client-id-1",
})
```

### `CancelAllOrders` — `POST /private/order/cancel_all`

```go
body, err := c.CancelAllOrders(ctx, mexcfutures.CancelAllOrdersRequest{
	Symbol: "BTC_USDT", // можно оставить пустым, если API допускает отмену по всем
})
```

## История и сделки

### `OrderHistory` — `GET /private/order/list/history_orders`

Параметры запроса — структура **`OrderHistoryParams`** (имена полей в JSON-тегах отражают ожидания TS; в URL уходят `category`, `page_num`, `page_size`, `states`, `symbol`).

```go
body, err := c.OrderHistory(ctx, mexcfutures.OrderHistoryParams{
	Category: 1,
	PageNum:  1,
	PageSize: 20,
	States:   3,
	Symbol:   "BTC_USDT",
})
```

### `OrderDeals` — `GET /private/order/list/order_deals`

```go
body, err := c.OrderDeals(ctx, mexcfutures.OrderDealsParams{
	Symbol:    "BTC_USDT",
	StartTime: 0,          // 0 = не передаётся в query
	EndTime:   0,
	PageNum:   1,
	PageSize:  20,
})
```

## Получение ордера

### `GetOrder` — `GET /private/order/get/{orderId}`

```go
body, err := c.GetOrder(ctx, "123456789")
```

`orderId` экранируется через `url.PathEscape`.

### `GetOrderByExternalID` — `GET /private/order/external/{symbol}/{externalOid}`

```go
body, err := c.GetOrderByExternalID(ctx, "BTC_USDT", "my-client-id-1")
```

## Разбор JSON

```go
var resp map[string]any
if err := mexcfutures.DecodeJSON(body, &resp); err != nil {
	// ...
}
```

Используйте свои типы под фактический ответ MEXC; в репозитории заранее заданы только тела **запросов** для ордеров.
