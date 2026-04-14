# Project Brief

## Purpose

This repository is a Go service for MEXC Futures market data capture and execution research.

It now supports three runtime modes:
- `capture`: ingest public order book and deal streams into ClickHouse
- `scalper`: run a live book-only scalper on `TAO_USDT`
- `replay`: replay historical ClickHouse market data through the same signal engine

## Primary Goals

- Persist raw and normalized MEXC futures market data in ClickHouse
- Build a low-latency, order-book-driven scalper for zero-fee execution environments
- Journal all signals, order events, and roundtrip outcomes in ClickHouse
- Keep live trading, market capture, and replay analysis as separate runtime concerns

## Non-Goals for V1

- Multi-symbol live trading
- Trade-flow-based entries
- Complex portfolio/risk allocation
- Machine-learning signal generation
