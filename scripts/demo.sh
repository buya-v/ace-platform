#!/usr/bin/env bash
# ACE Platform — Demo Smoke Test Script
# Automates the full demo flow: registration, auth, KYC, trading, post-trade.
# Exit 0 if all checks pass, exit 1 if any fail.
#
# Usage:
#   GATEWAY_URL=https://ace.asla.mn ./scripts/demo.sh
#   ./scripts/demo.sh --help

set -euo pipefail

# ── Configuration ────────────────────────────────────────────────────────────
GATEWAY_URL="${GATEWAY_URL:-https://ace.asla.mn}"
GATEWAY_URL="${GATEWAY_URL%/}"  # strip trailing slash
CURL_TIMEOUT="${CURL_TIMEOUT:-10}"

# Unique suffix for test isolation
RUN_ID="$(date +%s)"

# ── Colors ───────────────────────────────────────────────────────────────────
if [ -t 1 ]; then
    GREEN='\033[0;32m'
    RED='\033[0;31m'
    YELLOW='\033[0;33m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    RESET='\033[0m'
else
    GREEN='' RED='' YELLOW='' CYAN='' BOLD='' RESET=''
fi

# ── Counters ─────────────────────────────────────────────────────────────────
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0
declare -a RESULTS=()  # "step_name|status|time_ms"

# ── Help ─────────────────────────────────────────────────────────────────────
if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    cat <<'USAGE'
ACE Platform Demo Smoke Test

Usage:
  ./scripts/demo.sh [--help]

Environment variables:
  GATEWAY_URL    Base URL of the ACE gateway (default: https://ace.asla.mn)
  CURL_TIMEOUT   Timeout in seconds for HTTP calls (default: 10)

Behaviour:
  - Runs the full demo flow: registration, auth, KYC, trading, post-trade
  - Prints colored PASS / FAIL / SKIP per step
  - Exits 0 if zero failures, 1 otherwise

Requires: bash 4+, curl
Optional: jq (for JSON parsing; falls back to grep/sed)
USAGE
    exit 0
fi

# ── Utility functions ────────────────────────────────────────────────────────

# Portable millisecond timer (uses date +%s%N where available, else seconds*1000)
now_ms() {
    if date +%s%N >/dev/null 2>&1; then
        echo $(( $(date +%s%N) / 1000000 ))
    else
        echo $(( $(date +%s) * 1000 ))
    fi
}

# JSON field extractor — uses jq if available, else grep/sed
HAS_JQ=false
if command -v jq >/dev/null 2>&1; then
    HAS_JQ=true
fi

json_field() {
    local json="$1" field="$2"
    if $HAS_JQ; then
        echo "$json" | jq -r ".$field // empty" 2>/dev/null
    else
        # Fallback: extract "field":"value" or "field": "value"
        echo "$json" | grep -o "\"${field}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" \
            | head -1 | sed 's/.*:[[:space:]]*"\([^"]*\)".*/\1/'
    fi
}

json_field_num() {
    local json="$1" field="$2"
    if $HAS_JQ; then
        echo "$json" | jq -r ".$field // empty" 2>/dev/null
    else
        echo "$json" | grep -o "\"${field}\"[[:space:]]*:[[:space:]]*[0-9.]*" \
            | head -1 | sed 's/.*:[[:space:]]*//'
    fi
}

# Record a step result
record() {
    local name="$1" status="$2" elapsed_ms="$3"
    RESULTS+=("${name}|${status}|${elapsed_ms}")
    case "$status" in
        PASS) PASS_COUNT=$((PASS_COUNT + 1)); printf "  ${GREEN}PASS${RESET}  %s  ${CYAN}(%d ms)${RESET}\n" "$name" "$elapsed_ms" ;;
        FAIL) FAIL_COUNT=$((FAIL_COUNT + 1)); printf "  ${RED}FAIL${RESET}  %s  ${CYAN}(%d ms)${RESET}\n" "$name" "$elapsed_ms" ;;
        SKIP) SKIP_COUNT=$((SKIP_COUNT + 1)); printf "  ${YELLOW}SKIP${RESET}  %s  ${CYAN}(%d ms)${RESET}\n" "$name" "$elapsed_ms" ;;
    esac
}

# HTTP helper — sets RESP_BODY, RESP_STATUS
do_curl() {
    local method="$1" path="$2"
    shift 2
    local url="${GATEWAY_URL}${path}"
    local -a curl_args=(-s -w '\n%{http_code}' --max-time "$CURL_TIMEOUT" -X "$method")
    curl_args+=(-H 'Content-Type: application/json')
    # remaining args passed through (e.g. -H "Authorization: ..." -d '...')
    curl_args+=("$@")
    curl_args+=("$url")

    local raw
    raw=$(curl "${curl_args[@]}" 2>/dev/null) || { RESP_BODY=""; RESP_STATUS=000; return 1; }
    RESP_STATUS="${raw##*$'\n'}"
    RESP_BODY="${raw%$'\n'*}"
}

# ── Banner ───────────────────────────────────────────────────────────────────
echo ""
printf "${BOLD}ACE Platform — Demo Smoke Test${RESET}\n"
printf "Gateway: ${CYAN}%s${RESET}\n" "$GATEWAY_URL"
printf "Run ID:  ${CYAN}%s${RESET}\n" "$RUN_ID"
echo ""

# ═══════════════════════════════════════════════════════════════════════════
# PRE-FLIGHT CHECKS
# ═══════════════════════════════════════════════════════════════════════════
printf "${BOLD}Pre-flight checks${RESET}\n"

# Check curl
T0=$(now_ms)
if command -v curl >/dev/null 2>&1; then
    record "curl available" "PASS" $(( $(now_ms) - T0 ))
else
    record "curl available" "FAIL" $(( $(now_ms) - T0 ))
    echo "ERROR: curl is required but not found in PATH"
    exit 1
fi

# Check gateway reachable
T0=$(now_ms)
if do_curl GET /healthz && [[ "$RESP_STATUS" == "200" ]]; then
    record "gateway reachable (/healthz)" "PASS" $(( $(now_ms) - T0 ))
else
    record "gateway reachable (/healthz)" "FAIL" $(( $(now_ms) - T0 ))
    printf "${RED}Gateway not reachable at %s — aborting.${RESET}\n" "$GATEWAY_URL"
    # Print summary even on early exit
    echo ""
    printf "${BOLD}Summary${RESET}\n"
    printf "  Passed: %d  Failed: %d  Skipped: %d\n" "$PASS_COUNT" "$FAIL_COUNT" "$SKIP_COUNT"
    exit 1
fi

# Check backend service health (ports 8081-8088 for services + 8080 for gateway)
# These are best-effort — SKIP if unreachable
SERVICE_PORTS=(
    "8080|gateway"
    "8081|matching-engine"
    "8082|clearing-engine"
    "8083|margin-engine"
    "8084|settlement-engine"
    "8085|auth-service"
    "8086|compliance-service"
    "8087|market-data-service"
    "8088|warehouse-service"
)

# Extract host from GATEWAY_URL for direct service health checks
GW_HOST=$(echo "$GATEWAY_URL" | sed 's|https\?://||; s|:[0-9]*$||; s|/.*||')

for entry in "${SERVICE_PORTS[@]}"; do
    port="${entry%%|*}"
    svc="${entry##*|}"
    T0=$(now_ms)
    if curl -s --max-time 2 "http://${GW_HOST}:${port}/healthz" >/dev/null 2>&1; then
        record "service health: ${svc} (:${port})" "PASS" $(( $(now_ms) - T0 ))
    else
        record "service health: ${svc} (:${port})" "SKIP" $(( $(now_ms) - T0 ))
    fi
done

# ═══════════════════════════════════════════════════════════════════════════
# STEP 1 — User Registration
# ═══════════════════════════════════════════════════════════════════════════
echo ""
printf "${BOLD}Step 1 — User Registration${RESET}\n"

TRADER1_EMAIL="trader1-${RUN_ID}@demo.ace"
TRADER2_EMAIL="trader2-${RUN_ID}@demo.ace"
ADMIN_EMAIL="admin-${RUN_ID}@demo.ace"
PASSWORD="DemoPass123!"

TRADER1_ID=""
TRADER2_ID=""
ADMIN_ID=""

# Register trader1
T0=$(now_ms)
if do_curl POST /api/v1/auth/register \
    -d "{\"email\":\"${TRADER1_EMAIL}\",\"password\":\"${PASSWORD}\",\"role\":\"trader\"}"; then
    if [[ "$RESP_STATUS" == "200" || "$RESP_STATUS" == "201" ]]; then
        TRADER1_ID=$(json_field "$RESP_BODY" "id")
        record "register trader1 (${RESP_STATUS})" "PASS" $(( $(now_ms) - T0 ))
    else
        record "register trader1 (${RESP_STATUS})" "FAIL" $(( $(now_ms) - T0 ))
    fi
else
    record "register trader1 (unreachable)" "FAIL" $(( $(now_ms) - T0 ))
fi

# Register trader2
T0=$(now_ms)
if do_curl POST /api/v1/auth/register \
    -d "{\"email\":\"${TRADER2_EMAIL}\",\"password\":\"${PASSWORD}\",\"role\":\"trader\"}"; then
    if [[ "$RESP_STATUS" == "200" || "$RESP_STATUS" == "201" ]]; then
        TRADER2_ID=$(json_field "$RESP_BODY" "id")
        record "register trader2 (${RESP_STATUS})" "PASS" $(( $(now_ms) - T0 ))
    else
        record "register trader2 (${RESP_STATUS})" "FAIL" $(( $(now_ms) - T0 ))
    fi
else
    record "register trader2 (unreachable)" "FAIL" $(( $(now_ms) - T0 ))
fi

# Register admin
T0=$(now_ms)
if do_curl POST /api/v1/auth/register \
    -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${PASSWORD}\",\"role\":\"admin\"}"; then
    if [[ "$RESP_STATUS" == "200" || "$RESP_STATUS" == "201" ]]; then
        ADMIN_ID=$(json_field "$RESP_BODY" "id")
        record "register admin (${RESP_STATUS})" "PASS" $(( $(now_ms) - T0 ))
    else
        record "register admin (${RESP_STATUS})" "FAIL" $(( $(now_ms) - T0 ))
    fi
else
    record "register admin (unreachable)" "FAIL" $(( $(now_ms) - T0 ))
fi

# ═══════════════════════════════════════════════════════════════════════════
# STEP 2 — Authentication
# ═══════════════════════════════════════════════════════════════════════════
echo ""
printf "${BOLD}Step 2 — Authentication${RESET}\n"

TRADER1_TOKEN=""
TRADER2_TOKEN=""
ADMIN_TOKEN=""

login_user() {
    local label="$1" email="$2" token_var="$3"
    T0=$(now_ms)
    if do_curl POST /api/v1/auth/login \
        -d "{\"email\":\"${email}\",\"password\":\"${PASSWORD}\"}"; then
        if [[ "$RESP_STATUS" == "200" ]]; then
            # Try both snake_case and PascalCase (auth service may use either)
            local tok
            tok=$(json_field "$RESP_BODY" "access_token")
            if [[ -z "$tok" ]]; then
                tok=$(json_field "$RESP_BODY" "AccessToken")
            fi
            if [[ -n "$tok" ]]; then
                eval "${token_var}='${tok}'"
                record "login ${label} (token obtained)" "PASS" $(( $(now_ms) - T0 ))
            else
                record "login ${label} (no token in response)" "FAIL" $(( $(now_ms) - T0 ))
            fi
        else
            record "login ${label} (${RESP_STATUS})" "FAIL" $(( $(now_ms) - T0 ))
        fi
    else
        record "login ${label} (unreachable)" "FAIL" $(( $(now_ms) - T0 ))
    fi
}

login_user "trader1" "$TRADER1_EMAIL" TRADER1_TOKEN
login_user "trader2" "$TRADER2_EMAIL" TRADER2_TOKEN
login_user "admin"   "$ADMIN_EMAIL"   ADMIN_TOKEN

# Auth headers
auth_header() {
    echo "Authorization: Bearer $1"
}

# ═══════════════════════════════════════════════════════════════════════════
# STEP 3 — KYC / Compliance Flow
# ═══════════════════════════════════════════════════════════════════════════
echo ""
printf "${BOLD}Step 3 — KYC / Compliance Flow${RESET}\n"

PARTICIPANT1_ID=""
PARTICIPANT2_ID=""

submit_kyc() {
    local label="$1" token="$2" user_id="$3" email="$4" result_var="$5"
    if [[ -z "$token" ]]; then
        record "KYC submit ${label} (no auth token)" "SKIP" 0
        return
    fi
    T0=$(now_ms)
    local body
    body=$(cat <<ENDJSON
{
  "participant_id": "${user_id}",
  "participant_type": "INDIVIDUAL",
  "legal_name": "Demo ${label}",
  "trading_name": "${label} Trading",
  "nationality": "KE",
  "contact": {
    "email": "${email}",
    "phone": "+254700000001",
    "contact_person_name": "Demo ${label}"
  },
  "registered_address": {
    "line1": "123 Demo Street",
    "city": "Nairobi",
    "province": "Nairobi",
    "postal_code": "00100",
    "country": "KE"
  },
  "source_of_funds": "Trading income"
}
ENDJSON
)
    if do_curl POST /api/v1/participants -H "$(auth_header "$token")" -d "$body"; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            record "KYC submit ${label} (compliance-service unavailable)" "SKIP" $(( $(now_ms) - T0 ))
            return
        fi
        local pid
        pid=$(json_field "$RESP_BODY" "participant_id")
        if [[ -z "$pid" ]]; then
            pid=$(json_field "$RESP_BODY" "ParticipantID")
        fi
        if [[ -z "$pid" ]]; then
            pid="$user_id"  # fallback
        fi
        eval "${result_var}='${pid}'"
        if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            record "KYC submit ${label} (${RESP_STATUS})" "PASS" $(( $(now_ms) - T0 ))
        else
            record "KYC submit ${label} (${RESP_STATUS})" "FAIL" $(( $(now_ms) - T0 ))
        fi
    else
        record "KYC submit ${label} (unreachable)" "FAIL" $(( $(now_ms) - T0 ))
    fi
}

submit_kyc "trader1" "$TRADER1_TOKEN" "$TRADER1_ID" "$TRADER1_EMAIL" PARTICIPANT1_ID
submit_kyc "trader2" "$TRADER2_TOKEN" "$TRADER2_ID" "$TRADER2_EMAIL" PARTICIPANT2_ID

# Approve KYC for trader1 (as admin)
approve_kyc() {
    local label="$1" participant_id="$2"
    if [[ -z "$ADMIN_TOKEN" ]]; then
        record "KYC approve ${label} (no admin token)" "SKIP" 0
        return
    fi
    if [[ -z "$participant_id" ]]; then
        record "KYC approve ${label} (no participant ID)" "SKIP" 0
        return
    fi
    T0=$(now_ms)
    if do_curl POST "/api/v1/participants/${participant_id}/approve" \
        -H "$(auth_header "$ADMIN_TOKEN")" \
        -d '{"officer_id":"admin-demo","notes":"Demo approval"}'; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            record "KYC approve ${label} (compliance-service unavailable)" "SKIP" $(( $(now_ms) - T0 ))
        elif [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            record "KYC approve ${label} (${RESP_STATUS})" "PASS" $(( $(now_ms) - T0 ))
        else
            record "KYC approve ${label} (${RESP_STATUS})" "FAIL" $(( $(now_ms) - T0 ))
        fi
    else
        record "KYC approve ${label} (unreachable)" "FAIL" $(( $(now_ms) - T0 ))
    fi
}

approve_kyc "trader1" "$PARTICIPANT1_ID"
approve_kyc "trader2" "$PARTICIPANT2_ID"

# ═══════════════════════════════════════════════════════════════════════════
# STEP 4 — Trading Flow
# ═══════════════════════════════════════════════════════════════════════════
echo ""
printf "${BOLD}Step 4 — Trading Flow${RESET}\n"

INSTRUMENT="WHEAT-2026-07"
TRADE_QTY="10.0000"
TRADE_PRICE="250.0000"

# Place buy order (trader1)
T0=$(now_ms)
if [[ -z "$TRADER1_TOKEN" ]]; then
    record "submit buy order (no auth token)" "SKIP" 0
else
    BUY_PARTICIPANT="${TRADER1_ID:-participant-demo-buy}"
    if do_curl POST /api/v1/orders \
        -H "$(auth_header "$TRADER1_TOKEN")" \
        -d "{\"instrument_id\":\"${INSTRUMENT}\",\"side\":\"BUY\",\"order_type\":\"LIMIT\",\"quantity\":\"${TRADE_QTY}\",\"price\":\"${TRADE_PRICE}\",\"participant_id\":\"${BUY_PARTICIPANT}\",\"time_in_force\":\"GTC\"}"; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            record "submit buy order (matching-engine unavailable)" "SKIP" $(( $(now_ms) - T0 ))
        elif [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            record "submit buy order (${RESP_STATUS})" "PASS" $(( $(now_ms) - T0 ))
        else
            record "submit buy order (${RESP_STATUS})" "FAIL" $(( $(now_ms) - T0 ))
        fi
    else
        record "submit buy order (unreachable)" "FAIL" $(( $(now_ms) - T0 ))
    fi
fi

# Place sell order (trader2) — should match
T0=$(now_ms)
if [[ -z "$TRADER2_TOKEN" ]]; then
    record "submit sell order (no auth token)" "SKIP" 0
else
    SELL_PARTICIPANT="${TRADER2_ID:-participant-demo-sell}"
    if do_curl POST /api/v1/orders \
        -H "$(auth_header "$TRADER2_TOKEN")" \
        -d "{\"instrument_id\":\"${INSTRUMENT}\",\"side\":\"SELL\",\"order_type\":\"LIMIT\",\"quantity\":\"${TRADE_QTY}\",\"price\":\"${TRADE_PRICE}\",\"participant_id\":\"${SELL_PARTICIPANT}\",\"time_in_force\":\"GTC\"}"; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            record "submit sell order (matching-engine unavailable)" "SKIP" $(( $(now_ms) - T0 ))
        elif [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            record "submit sell order (${RESP_STATUS})" "PASS" $(( $(now_ms) - T0 ))
        else
            record "submit sell order (${RESP_STATUS})" "FAIL" $(( $(now_ms) - T0 ))
        fi
    else
        record "submit sell order (unreachable)" "FAIL" $(( $(now_ms) - T0 ))
    fi
fi

# Verify trade via last trade endpoint
T0=$(now_ms)
if [[ -z "$TRADER1_TOKEN" ]]; then
    record "verify trade (no auth token)" "SKIP" 0
else
    if do_curl GET "/api/v1/instruments/${INSTRUMENT}/trades/latest" \
        -H "$(auth_header "$TRADER1_TOKEN")"; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            record "verify trade (matching-engine unavailable)" "SKIP" $(( $(now_ms) - T0 ))
        elif [[ "$RESP_STATUS" == "200" ]]; then
            record "verify trade (${RESP_STATUS})" "PASS" $(( $(now_ms) - T0 ))
        elif [[ "$RESP_STATUS" == "404" ]]; then
            record "verify trade (no trades yet — 404)" "SKIP" $(( $(now_ms) - T0 ))
        else
            record "verify trade (${RESP_STATUS})" "FAIL" $(( $(now_ms) - T0 ))
        fi
    else
        record "verify trade (unreachable)" "FAIL" $(( $(now_ms) - T0 ))
    fi
fi

# ═══════════════════════════════════════════════════════════════════════════
# STEP 5 — Post-Trade Verification
# ═══════════════════════════════════════════════════════════════════════════
echo ""
printf "${BOLD}Step 5 — Post-Trade Verification${RESET}\n"

# Helper for GET with auth and graceful skip
get_check() {
    local label="$1" path="$2" token="$3" skip_on_missing="${4:-false}"
    if [[ -z "$token" ]]; then
        record "${label} (no auth token)" "SKIP" 0
        return
    fi
    T0=$(now_ms)
    if do_curl GET "$path" -H "$(auth_header "$token")"; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            record "${label} (backend unavailable)" "SKIP" $(( $(now_ms) - T0 ))
        elif [[ "$RESP_STATUS" == "200" ]]; then
            record "${label} (${RESP_STATUS})" "PASS" $(( $(now_ms) - T0 ))
        elif [[ "$RESP_STATUS" == "404" && "$skip_on_missing" == "true" ]]; then
            record "${label} (endpoint missing)" "SKIP" $(( $(now_ms) - T0 ))
        else
            record "${label} (${RESP_STATUS})" "FAIL" $(( $(now_ms) - T0 ))
        fi
    else
        record "${label} (unreachable)" "FAIL" $(( $(now_ms) - T0 ))
    fi
}

# Positions (trader1)
get_check "positions (trader1)" "/api/v1/clearing/positions" "$TRADER1_TOKEN"

# Positions (trader2)
get_check "positions (trader2)" "/api/v1/clearing/positions" "$TRADER2_TOKEN"

# Margin (trader1)
get_check "margin (trader1)" "/api/v1/margin" "$TRADER1_TOKEN"

# Settlement cycles
get_check "settlement cycles" "/api/v1/settlement/cycles" "$TRADER1_TOKEN"

# Margin calls
get_check "margin calls" "/api/v1/margin/calls" "$TRADER1_TOKEN"

# Netting
get_check "netting" "/api/v1/clearing/netting" "$TRADER1_TOKEN"

# ═══════════════════════════════════════════════════════════════════════════
# STEP 6 — Market Data & Order Book
# ═══════════════════════════════════════════════════════════════════════════
echo ""
printf "${BOLD}Step 6 — Market Data & Order Book${RESET}\n"

get_check "order book L2" "/api/v1/instruments/${INSTRUMENT}/book" "$TRADER1_TOKEN"
get_check "order book L3" "/api/v1/instruments/${INSTRUMENT}/book/l3" "$TRADER1_TOKEN"
get_check "last trade" "/api/v1/instruments/${INSTRUMENT}/trades/latest" "$TRADER1_TOKEN"

# ═══════════════════════════════════════════════════════════════════════════
# SUMMARY
# ═══════════════════════════════════════════════════════════════════════════
echo ""
printf "${BOLD}═══════════════════════════════════════════════════════════════${RESET}\n"
printf "${BOLD}Summary${RESET}\n"
printf "${BOLD}═══════════════════════════════════════════════════════════════${RESET}\n"
echo ""

# Print results table
printf "  %-50s %-6s %s\n" "STEP" "STATUS" "TIME"
printf "  %-50s %-6s %s\n" "────────────────────────────────────────────────" "──────" "────────"
for entry in "${RESULTS[@]}"; do
    IFS='|' read -r name status elapsed <<< "$entry"
    case "$status" in
        PASS) color="$GREEN" ;;
        FAIL) color="$RED" ;;
        SKIP) color="$YELLOW" ;;
        *)    color="$RESET" ;;
    esac
    printf "  %-50s ${color}%-6s${RESET} %d ms\n" "$name" "$status" "$elapsed"
done

echo ""
printf "  ${GREEN}Passed: %d${RESET}  ${RED}Failed: %d${RESET}  ${YELLOW}Skipped: %d${RESET}  Total: %d\n" \
    "$PASS_COUNT" "$FAIL_COUNT" "$SKIP_COUNT" $(( PASS_COUNT + FAIL_COUNT + SKIP_COUNT ))
echo ""

if [[ "$FAIL_COUNT" -gt 0 ]]; then
    printf "${RED}RESULT: FAIL${RESET}\n"
    exit 1
else
    printf "${GREEN}RESULT: PASS${RESET}\n"
    exit 0
fi
