# Аккаунт и позиции (futures.mexc.com)

Методы ниже используют **`getAuthFutures`**: `GET` на хост **`FuturesBaseURL`** с заголовком **`Authorization`** (WEB-ключ). Пути приватные.

## Лимиты и комиссии

### `RiskLimit` — `GET /private/account/risk_limit`

```go
body, err := c.RiskLimit(ctx)
```

### `FeeRate` — `GET /private/account/contract/fee_rate`

```go
body, err := c.FeeRate(ctx)
```

## Активы

### `AccountAsset` — `GET /private/account/asset/{currency}`

```go
body, err := c.AccountAsset(ctx, "USDT")
```

Сегмент `currency` проходит через `url.PathEscape`.

## Позиции

### `OpenPositionsFutures` — `GET /private/position/open_positions`

Вызов на **futures**-хосте (как в TS SDK).

```go
// Все открытые позиции (query без symbol)
body, err := c.OpenPositionsFutures(ctx, nil)
```

```go
// Фильтр по символу
sym := "BTC_USDT"
body, err := c.OpenPositionsFutures(ctx, &sym)
```

Если указан ненулевой указатель, но строка пустая, параметр `symbol` в query **не** добавляется.

### `PositionHistory` — `GET /private/position/list/history_positions`

```go
body, err := c.PositionHistory(ctx, mexcfutures.PositionHistoryParams{
	Symbol:   "BTC_USDT", // можно ""
	Type:     0,          // 0 = параметр type не отправляется
	PageNum:  1,
	PageSize: 20,
})
```

В query попадают `page_num`, `page_size` всегда; `symbol` и `type` — только если заданы (для `type` — если не 0).

## Связь с `contract.mexc.com`

Открытые позиции через **другой** хост и другой способ подписи описаны в [contract-host-compat.md](contract-host-compat.md) (`GetOpenPositionsContract`).
