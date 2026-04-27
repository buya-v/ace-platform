# Admin UI UAT Fixes

**Date:** 2026-04-27
**Scope:** 7 bug fixes across 6 page components

## Changes Made

### CRITICAL 1: Circuit Breakers page crash (`CircuitBreakers.tsx`)
- **Root cause:** `row.daily_volume.toLocaleString()` crashes when `daily_volume` is undefined/null
- **Fix:** Added null guard: `(row.daily_volume ?? 0).toLocaleString()`

### CRITICAL 2: System Health NaN values (`SystemMonitoring.tsx`)
- **Root cause:** `uptime_seconds` and `latency_ms` could be undefined, string, or missing, producing NaN in arithmetic and `formatUptime()`
- **Fix:**
  - Parse numeric fields with `typeof` check + `parseFloat` fallback to 0
  - `formatUptime()` now guards against NaN input
  - `data?.services.map()` changed to `(data?.services ?? []).map()` to prevent crash when services array is missing
  - Array guard added to latency history and avgLatency computation

### HIGH 1: Dashboard empty KPI cards (`DashboardHome.tsx`)
- **Root cause:** Fallback value was `'\u2014'` (em dash) when API returned null/error
- **Fix:** Changed all three KPI fallbacks from `'\u2014'` to `0`:
  - Pending KYC: `participants.data?.pagination?.total ?? 0`
  - Active Margin Calls: `marginStats.data?.total_active ?? 0`
  - Settlement Cycles: `settlements.data?.data?.length ?? 0`

### HIGH 2: Securities Positions raw UUID (`SecuritiesPositions.tsx`)
- **Root cause:** `instrument_id` column displayed raw UUID with no ticker resolution
- **Fix:** Added `useEffect` to fetch instruments list via `fetchSecuritiesInstruments()`, build a `Map<id, ticker>`, and render `instrumentMap.get(row.instrument_id) ?? row.instrument_id` in the column

### HIGH 3: Market Phase blank instrument data (`MarketPhase.tsx`)
- **Root cause:** `normalizeInstruments()` didn't check for `raw?.data` response shape
- **Fix:** Added `raw?.data` to the fallback chain: `raw?.instruments ?? raw?.data ?? (Array.isArray(raw) ? raw : [])`

### HIGH 4: Fee Management NaN bps (`FeeManagement.tsx`)
- **Root cause:** `rate_bps` could be undefined/string/NaN, causing `rateSum += r.rate_bps` to produce NaN
- **Fix:**
  - `computeFeeSummary()` now guards each `rate_bps` with `typeof` + `isNaN` check, defaulting to 0
  - Raw rules normalized on ingestion: `parseFloat(r.rate_bps) || 0`

### HIGH 5: Margin Calls empty summary cards (`MarginCalls.tsx`)
- **Root cause:** Summary cards only rendered when `stats.data` was truthy (wrapped in `{stats.data && ...}`)
- **Fix:** Cards always render with optional chaining + fallback values:
  - `stats.data?.total_active ?? 0`
  - `stats.data?.total_shortfall ?? '0.00'`
  - `stats.data?.participants_in_call ?? 0`
  - `stats.data?.average_utilization ?? 0`

## Files Modified
- `src/admin-ui/src/pages/CircuitBreakers.tsx`
- `src/admin-ui/src/pages/SystemMonitoring.tsx`
- `src/admin-ui/src/pages/DashboardHome.tsx`
- `src/admin-ui/src/pages/SecuritiesPositions.tsx`
- `src/admin-ui/src/pages/MarketPhase.tsx`
- `src/admin-ui/src/pages/FeeManagement.tsx`
- `src/admin-ui/src/pages/MarginCalls.tsx`

## Verification
- `npm run build` -- TypeScript + Vite build passes (123 modules, 330KB gzipped JS)
- Docker image rebuilt and container restarted (`garudax-admin-ui`)
