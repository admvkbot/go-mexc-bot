# Product Context

## Why This Exists

The project started as a market-data collector and browser-session-authenticated MEXC Futures client.

The current product direction is to turn that foundation into a practical micro-scalping system for a zero-fee venue, where positive expectancy depends on:
- maker-first entries
- fast cancellation and repricing
- strict slippage controls
- detailed post-trade analytics

## User Intent

The user wants a bot that trades purely from the order book:
- detect micro-impulses from top-of-book and top-level liquidity changes
- enter with limit orders
- extend with a controlled ladder
- exit with limit-first logic
- fall back to emergency market flatten only when the scenario degrades

## Operational Expectations

- The bot must keep market capture working
- The bot must write its own trading telemetry to ClickHouse
- Strategy thresholds must be tunable from environment variables
- Historical replay must reuse the same book-only signal logic for research
