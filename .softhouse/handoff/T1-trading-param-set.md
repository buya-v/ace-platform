# Handoff: T1 — Unified TradingParameterSet

## Status
COMPLETED — all builds pass, all tests pass.

## Summary
Implemented full TradingParameterSet CRUD across securities-service and gateway.

## Changes Made

### Part A — types.go (APPENDED)
- `AuctionConfig` struct: RandomEndSeconds, SurplusHandling (PRO_RATA/TIME_PRIORITY), MinAuctionDurationSeconds.
- `TradingParameterSet` struct: ID, InstrumentID, Name, TickTableID, CircuitBreakerID, AllowedOrderTypes, AllowedTimeInForce, MinOrderSize, MaxOrderSize, MaxOrderValue, AuctionParams, STPMode, ShortSellingAllowed, CreatedAt, UpdatedAt.
- File: `src/securities-service/internal/types/types.go`

### Part B — store.go (APPENDED)
- `TradingParamSetStore` interface: Create, Get, GetByInstrument, List, Update, Delete.
- `InMemoryTradingParamSetStore`: thread-safe, dual-indexed (ID + instrumentID mapping).
- Helper `copyTradingParamSet` deep-copies slice fields.
- File: `src/securities-service/internal/store/store.go`

### Part C — handlers_trading_params.go (NEW)
- POST /api/v1/securities/trading-params (validate instrument_id required)
- GET /api/v1/securities/trading-params (list)
- GET /api/v1/securities/trading-params/{id}
- GET /api/v1/securities/trading-params/instrument/{instrument_id}
- PUT /api/v1/securities/trading-params/{id} (preserves CreatedAt)
- DELETE /api/v1/securities/trading-params/{id} (204 No Content)
- File: `src/securities-service/internal/server/handlers_trading_params.go`

### Part D — handleSubmitOrder (MODIFIED)
Added block (j) after stop_price validation: loads TradingParameterSet for the instrument and checks:
- AllowedOrderTypes (422 ORDER_TYPE_NOT_ALLOWED) — skipped if list empty
- AllowedTimeInForce (422 TIME_IN_FORCE_NOT_ALLOWED) — skipped if list empty or TIF blank
- MinOrderSize (422 ORDER_TOO_SMALL) — skipped if 0
- MaxOrderSize (422 ORDER_TOO_LARGE) — skipped if 0
- MaxOrderValue = price * quantity (422 ORDER_VALUE_EXCEEDED) — skipped if 0 or price 0
- ShortSellingAllowed for SHORT_SELL (422 SHORT_SELLING_NOT_ALLOWED) — skipped if allowed
All checks nil-safe; no-op if no param set configured for instrument.
- File: `src/securities-service/internal/server/handlers_order.go`

### Part E — server.go + main.go + gateway routes (MODIFIED)
- server.go: added `tradingParamSetStore` field, added to New() params (before cfg), registered routes.
- cmd/main.go: wired `store.NewInMemoryTradingParamSetStore()`.
- gateway/main.go: added 6 routes proxied to securities-service.
- Test files: 20+ files updated to pass nil as new param via Python script.

## Build & Test Results
```
cd src/securities-service && go build ./... → OK
cd src/securities-service && go test ./...  → all packages PASS
cd src/gateway && go build ./...            → OK
```

## Decisions Made
- tradingParamSetStore placed as second-to-last param in New() (before cfg), consistent with prior optional stores.
- Instrument sub-route /trading-params/instrument/ registered before wildcard /trading-params/ to prevent path swallowing.
- All order validation checks strictly optional (zero/nil/empty = skip).

## Suggested Follow-ups
- Add handlers_trading_params_test.go with CRUD and order validation integration tests.
- PgTradingParamSetStore for production persistence.
- Wire TickTableID from param set into tick validation path (currently unused).
- Wire CircuitBreakerID from param set into CB engine lookup.
