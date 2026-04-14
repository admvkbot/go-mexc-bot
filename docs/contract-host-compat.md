# Совместимость с Python `mexc_client` (contract.mexc.com)

Часть логики дублирует поведение Python-клиента: отдельный базовый URL **`ContractBaseURL`** (по умолчанию `https://contract.mexc.com/api/v1`) и иной способ подписи для одного из методов.

## `GetOpenPositionsContract` — подписанный `GET`

- Путь: **`GET /private/position/open_positions`** на **contract**-хосте.
- Подпись: как для POST на futures — заголовки **`x-mxc-nonce`** и **`x-mxc-sign`**, но тело для подписи — **JSON пустого объекта** `{}` (сериализуется и участвует в MD5-цепочке), запрос при этом **без тела** (классический GET).

```go
body, err := c.GetOpenPositionsContract(ctx)
```

Используйте этот метод, если вам нужно повторить именно Python `get_open_positions` на `contract.mexc.com`. Для TS-стиля на `futures.mexc.com` см. **`OpenPositionsFutures`** в [rest-account-positions.md](rest-account-positions.md).

## `GetContractDetailContractPublic` — публичный `GET`

- Путь: **`GET /contract/detail`** на **contract**-хосте.
- Без авторизации; query: **`symbol`**.

```go
body, err := c.GetContractDetailContractPublic(ctx, "BTC_USDT")
```

## `ParseContractDetailSummary` — разбор ответа деталей контракта

Парсер ориентирован на JSON вида Python `get_contract_detail`: обёртка `success` + `data`, с возможной вложенностью `data` внутри `data` при `success: true`.

Возвращает **`ContractDetailSummary`**:

- Числовые поля: `ContractSize`, `MaxLeverage`, `MaxVolume`, `MinVolume`, `VolScale`, `VolUnit` — из ключей JSON в camelCase (`contractSize`, `maxLeverage`, `maxVol`, `minVol`, `volScale`, `volUnit`).
- **`Data`** — `map[string]any` с полным набором полей узла, чтобы не терять незамапленные ключи.

Пример:

```go
body, err := c.GetContractDetailContractPublic(ctx, "BTC_USDT")
if err != nil {
	return err
}
sum, err := mexcfutures.ParseContractDetailSummary(body)
if err != nil {
	return err
}
fmt.Println(sum.MaxLeverage, sum.MinVolume)
```

Юнит-тест в репозитории проверяет разбор минимального успешного JSON без вложенного `data.data`.
