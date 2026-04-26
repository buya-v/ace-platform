# T1 Handoff — Rich Order/Trade Queries, Trading Cycles, Market Times

**Status**: done
**Agent**: coder (claude-sonnet-4-6)
**Date**: 2026-04-26

---

## Summary

Implemented three feature groups on `securities-service` and wired gateway routes:

### Part A — Rich order/trade queries

**`handleListOrders`** (`internal/server/handlers_order.go`):
- Added 11 new query params: `side`, `order_type`, `time_in_force`, `price_min`, `price_max`, `quantity_min`, `quantity_max`, `date_from`, `date_to`, `client_order_id`, `firm_id`
- In-memory filter pass over all loaded orders
- Returns `{data, total, filters_applied}` envelope

**`handleListTrades`** (`internal/server/handlers_trade.go`):
- Added query params: `instrument_id`, `participant_id`, `date_from`, `date_to`, `price_min`, `price_max`
- Uses `ListByInstrument` when `instrument_id` provided, else `List()`
- Participant filter uses BuyOrderID/SellOrderID as proxies (noted in code comment)
- Returns same `{data, total, filters_applied}` envelope

### Part B — Trading cycles

**`types.TradingCycle`** added to `internal/types/types.go`:
```go
type TradingCycle struct {
    ID              string   `json:"id"`
    MarketID        string   `json:"market_id"`
    Name            string   `json:"name"`
    SessionSequence []string `json:"session_sequence"`
    IsDefault       bool     `json:"is_default"`
    CreatedAt       string   `json:"created_at"`
}
```

**`TradingCycleID string`** added to `Instrument` struct.

**`TradingCycleStore`** interface + `InMemoryTradingCycleStore` added to `internal/store/store.go`:
- `NewInMemoryTradingCycleStore()` seeds two default cycles: MSE STANDARD (PRE_OPEN→CONTINUOUS→CLOSING_AUCTION→CLOSED) and OFF_BOOK (PRE_OPEN→CLOSED)
- CRUD: Create, Get, ListByMarket, Delete

**`handlers_trading_cycle.go`** (new file): POST/GET/GET{id}/DELETE under `/api/v1/securities/trading-cycles`

**`server.go`**: `tradingCycleStore` field + parameter in `New()` + `SetTradingCycleStore()` setter + routes registered

**`cmd/main.go`**: `store.NewInMemoryTradingCycleStore()` wired into `server.New()`

### Part C — Market times

**`Market` struct**: Added `OpenTime string`, `CloseTime string`, `TradingDate string`

**`MarketStore` interface**: Extended with `SetTradingDate(date string) error`

**`InMemoryMarketStore`**: Implemented `SetTradingDate` (sets all markets' TradingDate to given ISO date)

**`handlers_day.go`**: On `StartDay` success, calls `s.marketStore.SetTradingDate(todayISO)` and returns `{state, trading_date}` in response

### Gateway

Four new routes added to `src/gateway/cmd/gateway/main.go`:
```
GET  /api/v1/securities/trading-cycles       → secHandler
POST /api/v1/securities/trading-cycles       → secHandler
GET  /api/v1/securities/trading-cycles/{id}  → secHandler
DELETE /api/v1/securities/trading-cycles/{id} → secHandler
```

---

## Test Results

All tests pass:
```
ok  github.com/garudax-platform/securities-service/internal/engine     0.027s
ok  github.com/garudax-platform/securities-service/internal/kafka      0.016s
ok  github.com/garudax-platform/securities-service/internal/middleware 0.015s
ok  github.com/garudax-platform/securities-service/internal/protocol   0.010s
ok  github.com/garudax-platform/securities-service/internal/server     0.544s
ok  github.com/garudax-platform/securities-service/internal/settlement 0.011s
ok  github.com/garudax-platform/securities-service/internal/store      1.118s
```

Gateway: `go build ./...` exits clean.

---

## Decisions Made

1. **`tradingCycleStore` as full `New()` parameter** (not just setter): Test files pre-expected this parameter position; adding it to the `New()` signature was the only way to satisfy both old and new call sites.

2. **Participant filter via BuyOrderID/SellOrderID**: No participant→order join table exists in the in-memory model. The filter approximates participant filtering at trade level using order IDs as proxies. A future task should add a `ParticipantID` field directly to `Trade`.

3. **`SetTradingDate` stamps all markets atomically**: Single call sets TradingDate on all markets at day-start. This avoids per-market day-start calls and keeps the session manager's StartDay sequence simple.

4. **MSE STANDARD and OFF_BOOK seed cycles**: Seeded in `NewInMemoryTradingCycleStore()` with stable IDs so handlers can reference them immediately without a separate setup call.

---

## Blockers Found

- Several test files had pre-existing arg count bugs (missing `tradingParamSetStore` nil or misaligned comments). Fixed as part of this task to achieve green test suite.

---

## Suggested Follow-ups

- Add `ParticipantID` to `Trade` struct to enable accurate participant-level trade filtering without relying on order IDs
- Add `OpenTime`/`CloseTime` setters per-market (currently only TradingDate is stamped at day-start)
- Add PostgreSQL-backed `TradingCycleStore` alongside the in-memory implementation
- Add trading cycle assignment handler: `PATCH /api/v1/securities/instruments/{id}` to set `TradingCycleID`
- Gateway: add `GET /api/v1/securities/trading-cycles/{id}` test coverage
