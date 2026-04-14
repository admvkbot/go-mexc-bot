# Рыночные данные (публичные, futures.mexc.com)

Эти методы вызывают **`getPublicFutures`**: без заголовка `Authorization`, только «браузерные» заголовки по умолчанию. База — **`FuturesBaseURL`**.

## `Ticker` — `GET /contract/ticker`

Обязательный query-параметр **`symbol`** (формат вроде `BTC_USDT`).

```go
body, err := c.Ticker(ctx, "BTC_USDT")
```

## `ContractDetailFutures` — `GET /contract/detail`

Публичные детали контракта на **futures**-хосте.

```go
// Без фильтра по символу
body, err := c.ContractDetailFutures(ctx, nil)
```

```go
sym := "BTC_USDT"
body, err := c.ContractDetailFutures(ctx, &sym)
```

## `ContractDepth` — `GET /contract/depth/{symbol}`

Путь содержит символ; опционально ограничение глубины стакана.

```go
body, err := c.ContractDepth(ctx, "BTC_USDT", nil)
```

```go
lim := 50
body, err := c.ContractDepth(ctx, "BTC_USDT", &lim)
```

Параметр `limit` в query добавляется только если указан ненулевой указатель.

## `TestConnection`

Обёртка над **`Ticker`** для символа **`BTC_USDT`**: удобно проверить доступность API и сеть.

```go
if err := c.TestConnection(ctx); err != nil {
	log.Fatal(err)
}
```

Возвращает только **`error`** (тело ответа отбрасывается).
