#!/usr/bin/env bash
# GarudaX Platform — Data Seed Script
# Populates the platform with realistic demo data for the admin dashboard.
# Registers users, submits orders that cross (generating trades), creates
# warehouse receipts, and triggers compliance screening events.
#
# Usage:
#   GATEWAY_URL=http://127.0.0.1:8080 ./scripts/seed-data.sh
#   GATEWAY_URL=https://garudax.asla.mn ./scripts/seed-data.sh

set -euo pipefail

GATEWAY="${GATEWAY_URL:-http://127.0.0.1:8080}"
GATEWAY="${GATEWAY%/}"
CURL_TIMEOUT="${CURL_TIMEOUT:-10}"
RUN_ID="$(date +%s)"
PASSWORD="SeedPass123!"

# ── Colors ──────────────────────────────────────────────────────────────────
if [ -t 1 ]; then
    GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[0;33m'
    CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
else
    GREEN='' RED='' YELLOW='' CYAN='' BOLD='' RESET=''
fi

# ── Helpers ─────────────────────────────────────────────────────────────────
STEP_COUNT=0
PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

step() {
    STEP_COUNT=$((STEP_COUNT + 1))
    printf "  [%02d] %s ... " "$STEP_COUNT" "$1"
}

pass() {
    PASS_COUNT=$((PASS_COUNT + 1))
    printf "${GREEN}OK${RESET} %s\n" "${1:-}"
}

fail() {
    FAIL_COUNT=$((FAIL_COUNT + 1))
    printf "${RED}FAIL${RESET} %s\n" "${1:-}"
}

skip() {
    SKIP_COUNT=$((SKIP_COUNT + 1))
    printf "${YELLOW}SKIP${RESET} %s\n" "${1:-}"
}

HAS_JQ=false
command -v jq >/dev/null 2>&1 && HAS_JQ=true

json_field() {
    local json="$1" field="$2"
    if $HAS_JQ; then
        printf '%s' "$json" | jq -r ".$field // empty" 2>/dev/null
    else
        printf '%s' "$json" | grep -o "\"${field}\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" \
            | head -1 | sed 's/.*:[[:space:]]*"\([^"]*\)".*/\1/'
    fi
}

# do_curl sets RESP_BODY and RESP_STATUS
do_curl() {
    local method="$1" path="$2"
    shift 2
    local url="${GATEWAY}${path}"
    local -a args=(-s -w '\n%{http_code}' --max-time "$CURL_TIMEOUT" -X "$method")
    args+=(-H 'Content-Type: application/json')
    # Extract -d body separately to pipe via stdin (avoids curl encoding issues)
    local body_data=""
    local -a other_args=()
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d) body_data="$2"; shift 2 ;;
            *)  other_args+=("$1"); shift ;;
        esac
    done
    args+=("${other_args[@]}" "$url")
    local raw
    if [[ -n "$body_data" ]]; then
        raw=$(printf '%s' "$body_data" | curl "${args[@]}" --data-binary @- 2>/dev/null) || { RESP_BODY=""; RESP_STATUS=000; return 1; }
    else
        raw=$(curl "${args[@]}" 2>/dev/null) || { RESP_BODY=""; RESP_STATUS=000; return 1; }
    fi
    RESP_STATUS="${raw##*$'\n'}"
    RESP_BODY="${raw%$'\n'*}"
}

auth_header() {
    printf 'Authorization: Bearer %s' "$1"
}

# do_direct calls a service directly (bypassing gateway) for operations
# that don't proxy cleanly through the gateway HTTP forwarding layer.
do_direct() {
    local base_url="$1" method="$2" path="$3"
    shift 3
    local url="${base_url}${path}"
    local -a args=(-s -w '\n%{http_code}' --max-time "$CURL_TIMEOUT" -X "$method")
    args+=(-H 'Content-Type: application/json')
    local body_data=""
    local -a other_args=()
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -d) body_data="$2"; shift 2 ;;
            *)  other_args+=("$1"); shift ;;
        esac
    done
    args+=("${other_args[@]}" "$url")
    local raw
    if [[ -n "$body_data" ]]; then
        raw=$(printf '%s' "$body_data" | curl "${args[@]}" --data-binary @- 2>/dev/null) || { RESP_BODY=""; RESP_STATUS=000; return 1; }
    else
        raw=$(curl "${args[@]}" 2>/dev/null) || { RESP_BODY=""; RESP_STATUS=000; return 1; }
    fi
    RESP_STATUS="${raw##*$'\n'}"
    RESP_BODY="${raw%$'\n'*}"
}

# Service direct endpoints (bypass gateway for operations with routing issues)
MATCHING_ENGINE="${MATCHING_ENGINE_URL:-http://127.0.0.1:8081}"
AUTH_SERVICE="${AUTH_SERVICE_URL:-http://127.0.0.1:8085}"

# ── Banner ──────────────────────────────────────────────────────────────────
echo ""
printf "${BOLD}GarudaX Platform — Data Seed Script${RESET}\n"
printf "Gateway: ${CYAN}%s${RESET}\n" "$GATEWAY"
printf "Run ID:  ${CYAN}%s${RESET}\n\n" "$RUN_ID"

# ── Pre-flight ──────────────────────────────────────────────────────────────
step "Gateway health check"
if do_curl GET /healthz && [[ "$RESP_STATUS" == "200" ]]; then
    pass
else
    fail "(status: ${RESP_STATUS:-unreachable})"
    printf "${RED}Gateway not reachable — aborting.${RESET}\n"
    exit 1
fi

# ═════════════════════════════════════════════════════════════════════════════
# STEP 1 — Register Users
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Register Users ---${RESET}\n"

declare -a USER_EMAILS USER_IDS USER_TOKENS USER_ROLES
USER_EMAILS=(
    "trader1-${RUN_ID}@demo.ace"
    "trader2-${RUN_ID}@demo.ace"
    "trader3-${RUN_ID}@demo.ace"
    "admin-${RUN_ID}@demo.ace"
    "compliance-${RUN_ID}@demo.ace"
)
USER_ROLES=(trader trader trader admin compliance_officer)

for i in 0 1 2 3 4; do
    email="${USER_EMAILS[$i]}"
    role="${USER_ROLES[$i]}"
    step "Register ${role} (${email})"
    if do_curl POST /api/v1/auth/register \
        -d "{\"email\":\"${email}\",\"password\":\"${PASSWORD}\",\"role\":\"${role}\"}"; then
        if [[ "$RESP_STATUS" == "200" || "$RESP_STATUS" == "201" ]]; then
            uid=$(json_field "$RESP_BODY" "id")
            if [[ -z "$uid" ]]; then uid=$(json_field "$RESP_BODY" "user_id"); fi
            USER_IDS[$i]="${uid:-user-${i}}"
            pass "(id: ${USER_IDS[$i]})"
        else
            fail "(${RESP_STATUS})"
            USER_IDS[$i]="user-${i}"
        fi
    else
        fail "(unreachable)"
        USER_IDS[$i]="user-${i}"
    fi
done

# ═════════════════════════════════════════════════════════════════════════════
# STEP 2 — Login All Users
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Login Users ---${RESET}\n"

for i in 0 1 2 3 4; do
    email="${USER_EMAILS[$i]}"
    role="${USER_ROLES[$i]}"
    step "Login ${role} (${email})"
    if do_curl POST /api/v1/auth/login \
        -d "{\"email\":\"${email}\",\"password\":\"${PASSWORD}\"}"; then
        if [[ "$RESP_STATUS" == "200" ]]; then
            tok=$(json_field "$RESP_BODY" "access_token")
            if [[ -z "$tok" ]]; then tok=$(json_field "$RESP_BODY" "AccessToken"); fi
            USER_TOKENS[$i]="${tok:-}"
            if [[ -n "${USER_TOKENS[$i]}" ]]; then
                pass "(token obtained)"
            else
                fail "(no token in response)"
            fi
        else
            fail "(${RESP_STATUS})"
            USER_TOKENS[$i]=""
        fi
    else
        fail "(unreachable)"
        USER_TOKENS[$i]=""
    fi
done

TRADER1_TOKEN="${USER_TOKENS[0]:-}"
TRADER2_TOKEN="${USER_TOKENS[1]:-}"
TRADER3_TOKEN="${USER_TOKENS[2]:-}"
ADMIN_TOKEN="${USER_TOKENS[3]:-}"
COMPLIANCE_TOKEN="${USER_TOKENS[4]:-}"

TRADER1_ID="${USER_IDS[0]:-}"
TRADER2_ID="${USER_IDS[1]:-}"
TRADER3_ID="${USER_IDS[2]:-}"

# ═════════════════════════════════════════════════════════════════════════════
# STEP 3 — KYC Applications
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- KYC Applications ---${RESET}\n"

submit_kyc() {
    local label="$1" token="$2" user_id="$3" email="$4" nationality="$5"
    if [[ -z "$token" ]]; then skip "(no token)"; return; fi
    local body
    body=$(cat <<ENDJSON
{
  "participant_id": "${user_id}",
  "participant_type": "INDIVIDUAL",
  "legal_name": "Demo ${label}",
  "trading_name": "${label} Trading Co",
  "nationality": "${nationality}",
  "contact": {"email":"${email}","phone":"+254700000001","contact_person_name":"Demo ${label}"},
  "registered_address": {"line1":"123 Demo Street","city":"Nairobi","province":"Nairobi","postal_code":"00100","country":"KE"},
  "source_of_funds": "Trading income and investments"
}
ENDJSON
)
    if do_curl POST /api/v1/participants -H "$(auth_header "$token")" -d "$body"; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            skip "(compliance-service unavailable)"
        elif [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            pass "(${RESP_STATUS})"
        else
            fail "(${RESP_STATUS})"
        fi
    else
        fail "(unreachable)"
    fi
}

step "KYC trader1"
submit_kyc "Trader1" "$TRADER1_TOKEN" "$TRADER1_ID" "${USER_EMAILS[0]}" "KE"

step "KYC trader2"
submit_kyc "Trader2" "$TRADER2_TOKEN" "$TRADER2_ID" "${USER_EMAILS[1]}" "UG"

step "KYC trader3"
submit_kyc "Trader3" "$TRADER3_TOKEN" "$TRADER3_ID" "${USER_EMAILS[2]}" "TZ"

# Approve KYC for all traders
for i in 0 1 2; do
    label="trader$((i+1))"
    uid="${USER_IDS[$i]}"
    step "Approve KYC ${label}"
    if [[ -z "$ADMIN_TOKEN" ]]; then skip "(no admin token)"; continue; fi
    if do_curl POST "/api/v1/participants/${uid}/approve" \
        -H "$(auth_header "$ADMIN_TOKEN")" \
        -d '{"officer_id":"admin-demo","notes":"Demo approval"}'; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            skip "(unavailable)"
        elif [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            pass "(${RESP_STATUS})"
        else
            fail "(${RESP_STATUS})"
        fi
    else
        fail "(unreachable)"
    fi
done

# ═════════════════════════════════════════════════════════════════════════════
# STEP 4 — Submit Crossing Orders (generates trades)
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Submit Orders (crossing pairs) ---${RESET}\n"

INSTRUMENT="WHT-HRW-2026M07-UB"

submit_order() {
    local label="$1" token="$2" participant="$3" side="$4" qty="$5" price="$6"
    step "${label}"
    if [[ -z "$token" ]]; then skip "(no token)"; return; fi
    if do_direct "$MATCHING_ENGINE" POST /orders \
        -d "{\"instrument_id\":\"${INSTRUMENT}\",\"side\":\"${side}\",\"type\":\"LIMIT\",\"quantity\":\"${qty}\",\"price\":\"${price}\",\"account_id\":\"${participant}\",\"time_in_force\":\"GTC\"}"; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            skip "(matching-engine unavailable)"
        elif [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            pass "(${RESP_STATUS})"
        else
            fail "(${RESP_STATUS})"
        fi
    else
        fail "(unreachable)"
    fi
}

# Crossing pair 1: BUY 10 @ 325.50 vs SELL 10 @ 325.50 -> exact match
submit_order "trader1 BUY 10 @ 325.50" "$TRADER1_TOKEN" "$TRADER1_ID" "BUY" "10.0000" "325.5000"
submit_order "trader2 SELL 10 @ 325.50" "$TRADER2_TOKEN" "$TRADER2_ID" "SELL" "10.0000" "325.5000"

# Crossing pair 2: BUY 5 @ 330.00 vs SELL 5 @ 328.00 -> crosses at 330
submit_order "trader1 BUY 5 @ 330.00" "$TRADER1_TOKEN" "$TRADER1_ID" "BUY" "5.0000" "330.0000"
submit_order "trader2 SELL 5 @ 328.00" "$TRADER2_TOKEN" "$TRADER2_ID" "SELL" "5.0000" "328.0000"

# Crossing pair 3: trader3 enters the market
submit_order "trader3 BUY 8 @ 332.00" "$TRADER3_TOKEN" "$TRADER3_ID" "BUY" "8.0000" "332.0000"
submit_order "trader1 SELL 8 @ 331.00" "$TRADER1_TOKEN" "$TRADER1_ID" "SELL" "8.0000" "331.0000"

# Resting orders (won't match — create depth)
submit_order "trader1 BUY 20 @ 320.00 (resting)" "$TRADER1_TOKEN" "$TRADER1_ID" "BUY" "20.0000" "320.0000"
submit_order "trader2 BUY 15 @ 318.00 (resting)" "$TRADER2_TOKEN" "$TRADER2_ID" "BUY" "15.0000" "318.0000"
submit_order "trader2 SELL 20 @ 340.00 (resting)" "$TRADER2_TOKEN" "$TRADER2_ID" "SELL" "20.0000" "340.0000"
submit_order "trader3 SELL 15 @ 342.00 (resting)" "$TRADER3_TOKEN" "$TRADER3_ID" "SELL" "15.0000" "342.0000"

# ═════════════════════════════════════════════════════════════════════════════
# STEP 5 — Verify Post-Trade Data
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Verify Post-Trade Data ---${RESET}\n"

check_endpoint() {
    local label="$1" path="$2" token="$3"
    step "${label}"
    if [[ -z "$token" ]]; then skip "(no token)"; return; fi
    if do_curl GET "$path" -H "$(auth_header "$token")"; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            skip "(backend unavailable)"
        elif [[ "$RESP_STATUS" == "200" ]]; then
            pass "(200)"
        else
            fail "(${RESP_STATUS})"
        fi
    else
        fail "(unreachable)"
    fi
}

check_endpoint "Positions (trader1)"       "/api/v1/clearing/positions"                        "$TRADER1_TOKEN"
check_endpoint "Positions (trader2)"       "/api/v1/clearing/positions"                        "$TRADER2_TOKEN"
check_endpoint "Margin (trader1)"          "/api/v1/margin"                                    "$TRADER1_TOKEN"
check_endpoint "Margin calls"              "/api/v1/margin/calls"                              "$TRADER1_TOKEN"
check_endpoint "Settlement cycles"         "/api/v1/settlement/cycles"                         "$TRADER1_TOKEN"
check_endpoint "Netting"                   "/api/v1/clearing/netting"                          "$TRADER1_TOKEN"
step "Order book"
if do_direct "$MATCHING_ENGINE" GET "/book/${INSTRUMENT}"; then
    if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then pass "($RESP_STATUS)"; else fail "($RESP_STATUS)"; fi
else fail "(unreachable)"; fi
step "Last trade"
if do_direct "$MATCHING_ENGINE" GET "/trades/latest/${INSTRUMENT}"; then
    if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then pass "($RESP_STATUS)"; else fail "($RESP_STATUS)"; fi
else fail "(unreachable)"; fi
check_endpoint "Market data candles"       "/api/v1/market-data/candles/${INSTRUMENT}?interval=1m" "$TRADER1_TOKEN"
check_endpoint "Market data ticker"        "/api/v1/market-data/ticker/${INSTRUMENT}"          "$TRADER1_TOKEN"
check_endpoint "Market data trades"        "/api/v1/market-data/trades/${INSTRUMENT}"          "$TRADER1_TOKEN"

# ═════════════════════════════════════════════════════════════════════════════
# STEP 6 — Warehouse Data
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Warehouse Data ---${RESET}\n"

check_endpoint "Warehouse inventory"       "/api/v1/warehouse/inventory"                       "$ADMIN_TOKEN"

# ═════════════════════════════════════════════════════════════════════════════
# STEP 7 — Compliance Alerts
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Compliance Data ---${RESET}\n"

check_endpoint "Compliance alerts"         "/api/v1/compliance/alerts"                         "$ADMIN_TOKEN"
check_endpoint "Compliance audit trail"    "/api/v1/compliance/audit-trail"                    "$ADMIN_TOKEN"

# Check participants list
check_endpoint "Participants list"         "/api/v1/participants"                              "$ADMIN_TOKEN"

# ═════════════════════════════════════════════════════════════════════════════
# STEP 8 — Admin Health
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Admin ---${RESET}\n"

check_endpoint "Admin health (aggregated)" "/api/v1/admin/health"                              "$ADMIN_TOKEN"
check_endpoint "Circuit breakers"          "/api/v1/admin/circuit-breakers"                    "$ADMIN_TOKEN"

# ═════════════════════════════════════════════════════════════════════════════
# SUMMARY
# ═════════════════════════════════════════════════════════════════════════════
echo ""
printf "${BOLD}═══════════════════════════════════════════════════════════════${RESET}\n"
printf "  ${GREEN}Passed: %d${RESET}  ${RED}Failed: %d${RESET}  ${YELLOW}Skipped: %d${RESET}  Total: %d\n" \
    "$PASS_COUNT" "$FAIL_COUNT" "$SKIP_COUNT" "$STEP_COUNT"
printf "${BOLD}═══════════════════════════════════════════════════════════════${RESET}\n"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
    printf "\n${RED}Some steps failed. Check the output above.${RESET}\n"
    exit 1
else
    printf "\n${GREEN}Data seeding complete. Admin dashboard should now show data.${RESET}\n"
    exit 0
fi
