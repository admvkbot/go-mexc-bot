# Клиент, конфигурация и аутентификация

## Суть модели доступа

`Client` выполняет подписанные и неподписанные REST-вызовы к MEXC Futures, подставляя в заголовки:

- **`Authorization`** — строка WEB-ключа (как в браузере).
- Для **POST с JSON** на хосте `futures.mexc.com` — **`x-mxc-nonce`** и **`x-mxc-sign`**, вычисляемые функцией `webSignature`: цепочка MD5 от ключа, nonce, тела JSON (совместимо с описанными в коде Python/TS клиентами).
- Для **подписанного GET на `contract.mexc.com`** тело подписи сериализуется в JSON (в коде совместимости — пустой объект `{}`).

## Переменные окружения

| Переменная | Назначение |
|------------|------------|
| `MEXC_SOURCE_WEB_KEY` | WEB-токен для **захвата рынка** (потоки/данные в приложении `mexc-bot` при режиме `capture`). |
| `MEXC_WEB_KEY` | WEB-токен для **торговли** (приватный REST, ордера, скальпер при режиме `scalper`). |

### Загрузка `.env`

- `mexcfutures.LoadDotEnv()` — вызывает `godotenv.Load()`; ошибка отсутствующего файла **игнорируется**.
- `mexcfutures.TradeWebKeyFromEnv(loadDotenv bool)` — при `true` загружает `.env`, затем читает `MEXC_WEB_KEY` (торговый ключ).
- `mexcfutures.SourceWebKeyFromEnv(loadDotenv bool)` — то же для `MEXC_SOURCE_WEB_KEY` (ключ для data/capture).
- `mexcfutures.WebKeyFromEnv` — алиас на `TradeWebKeyFromEnv` (торговый ключ).

## Создание клиента

### Из переменных окружения

```go
c, err := mexcfutures.NewClientFromEnv()
```

Эквивалентно: загрузка `.env` (если есть), чтение ключа, `NewClient(Config{WebKey: k})`.

### Явная конфигурация

```go
c, err := mexcfutures.NewClient(mexcfutures.Config{
	WebKey:          os.Getenv("MEXC_WEB_KEY"),
	FuturesBaseURL:  "", // пусто = https://futures.mexc.com/api/v1
	ContractBaseURL: "", // пусто = https://contract.mexc.com/api/v1
	UserAgent:       "", // пусто = Chrome-подобная строка по умолчанию
	HTTPClient:      nil, // nil = клиент с таймаутом 30s
})
```

Поля `FuturesBaseURL` и `ContractBaseURL` перед использованием обрезаются с конца по символу `/`.

## Низкоуровневый HTTP

`(*Client).DoJSON(ctx, req)` — выполняет запрос с учётом `ctx`, читает тело, на статусы вне 2xx возвращает ошибку с усечённым текстом ответа (до 512 символов). Остальные методы пакета собирают `*http.Request` и вызывают `DoJSON`.

## Заголовки по умолчанию

`applyDefaultHeaders` выставляет, в частности:

- `content-type: application/json` (для POST),
- `origin: https://www.mexc.com`,
- `referer: https://www.mexc.com/`,
- `user-agent` из конфига,
- `accept`, `accept-language`, `cache-control`, `pragma`, `dnt`, `x-language`.

Это имитирует запрос из браузера; при кастомном `UserAgent` можно снизить «заметность» автоматизации, но биржа может менять требования к заголовкам.

## Контекст

Все публичные RPC-методы принимают `context.Context` для отмены и дедлайнов.
