# Документация: работа с MEXC через `mexcfutures`

Пакет `github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures` — HTTP-клиент для части REST API **MEXC Futures** и контрактного WebSocket edge, ориентированный на сценарий **браузерной сессии**: в запросах используется WEB-токен (значение cookie / строка авторизации), а не пара API Key + секрет с HMAC, как в официальной биржевой документации для ключей.

## Структура репозитория

| Путь | Назначение |
|------|------------|
| `cmd/mexc-bot/` | Точка входа процесса бота |
| `internal/app/` | Сборка зависимостей и жизненный цикл приложения |
| `internal/config/` | Загрузка конфигурации процесса из окружения |
| `internal/ports/` | Интерфейсы (порты), от которых зависит `app` |
| `internal/infrastructure/mexc/mexcfutures/` | Реализация клиента MEXC (REST, подпись, WS) |

## Оглавление

| Файл | Содержание |
|------|------------|
| [client-configuration.md](client-configuration.md) | Создание клиента, `Config`, переменные окружения, базовые URL, заголовки и подпись запросов |
| [rest-orders.md](rest-orders.md) | Размещение и отмена ордеров, история, сделки по ордерам, получение ордера |
| [rest-account-positions.md](rest-account-positions.md) | Лимиты риска, комиссии, активы, открытые позиции, история позиций |
| [rest-market.md](rest-market.md) | Публичные тикер, детали контракта, стакан, проверка соединения |
| [contract-host-compat.md](contract-host-compat.md) | Совместимость с Python `mexc_client`: `contract.mexc.com`, парсинг деталей контракта |

## Два базовых хоста

По умолчанию клиент ходит на:

- **`https://futures.mexc.com/api/v1`** — основной набор приватных и публичных путей (как в TS SDK).
- **`https://contract.mexc.com/api/v1`** — отдельные вызовы для совместимости с Python-клиентом (подписанный `GET` позиций и публичный `GET` деталей контракта).

Их можно переопределить полями `FuturesBaseURL` и `ContractBaseURL` в `Config`.

## Быстрый старт

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mexc-bot/go-mexc-bot/internal/infrastructure/mexc/mexcfutures"
)

func main() {
	ctx := context.Background()
	c, err := mexcfutures.NewClientFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if err := c.TestConnection(ctx); err != nil {
		log.Fatal(err)
	}
	body, err := c.Ticker(ctx, "BTC_USDT")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(body))
}
```

Перед запуском бота задайте в `.env`: `MEXC_SOURCE_WEB_KEY` для режима **capture**, `MEXC_WEB_KEY` для **scalper** (при `capture,scalper` — оба). См. [client-configuration.md](client-configuration.md).

## Ответы API

Большинство методов возвращает **`[]byte`** — сырое тело JSON ответа при HTTP 2xx. Для разбора используйте `encoding/json` или хелпер `mexcfutures.DecodeJSON`. Исключение по смыслу — `ParseContractDetailSummary`, который возвращает структурированное подмножество полей (см. [contract-host-compat.md](contract-host-compat.md)).

## Ограничения и оговорки

- Клиент не реализует WebSocket и не покрывает весь спектр MEXC API.
- Использование WEB-токена должно соответствовать правилам биржи и вашей модели безопасности; токен даёт доступ к приватным операциям — храните его как секрет.
- Константы `side`, `type`, `openType` и т.д. — числовые/строковые коды биржи; этот репозиторий не дублирует полный справочник значений (смотрите актуальную документацию MEXC / примеры SDK).
