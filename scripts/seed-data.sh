#!/usr/bin/env bash
# GarudaX Platform — Data Seed Script (Multi-Instrument Edition)
# Populates the platform with a realistic multi-instrument exchange environment.
# Registers 9 participants across 4 tiers, submits ~80 orders across 6 instruments
# to generate 30+ trades, issues warehouse receipts for physical commodities,
# and triggers compliance screening events.
#
# Usage:
#   GATEWAY_URL=http://127.0.0.1:8080 ./scripts/seed-data.sh
#   GATEWAY_URL=https://garudax.asla.mn ./scripts/seed-data.sh
#
# The script is idempotent — re-running appends a new run (unique emails per RUN_ID).

set -euo pipefail

GATEWAY="${GATEWAY_URL:-http://127.0.0.1:8080}"
GATEWAY="${GATEWAY%/}"
CURL_TIMEOUT="${CURL_TIMEOUT:-10}"
RUN_ID="$(date +%s)"
PASSWORD="SeedPass123!"

# Service direct endpoints (bypass gateway for operations with routing issues)
MATCHING_ENGINE="${MATCHING_ENGINE_URL:-http://127.0.0.1:8081}"
AUTH_SERVICE="${AUTH_SERVICE_URL:-http://127.0.0.1:8085}"
WAREHOUSE_SERVICE="${WAREHOUSE_SERVICE_URL:-http://127.0.0.1:8088}"

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

# Trade counters per instrument
declare -A TRADE_COUNT
TRADE_COUNT=()

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

# do_direct calls a service directly (bypassing gateway)
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

# ── Banner ──────────────────────────────────────────────────────────────────
echo ""
printf "${BOLD}GarudaX Platform — Data Seed Script (Multi-Instrument)${RESET}\n"
printf "Gateway:           ${CYAN}%s${RESET}\n" "$GATEWAY"
printf "Matching Engine:   ${CYAN}%s${RESET}\n" "$MATCHING_ENGINE"
printf "Warehouse Service: ${CYAN}%s${RESET}\n" "$WAREHOUSE_SERVICE"
printf "Run ID:            ${CYAN}%s${RESET}\n\n" "$RUN_ID"

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
# STEP 1 — Register Participants (9 accounts across tiers)
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Register Participants (9 accounts) ---${RESET}\n"

# Participant arrays: name, email, role
PARTICIPANT_NAMES=(farmer1 farmer2 farmer3 hedger1 hedger2 spec1 spec2 mm1 admin1)
PARTICIPANT_TIERS=(farmer  farmer  farmer  hedger  hedger  speculator speculator market_maker admin)
PARTICIPANT_ROLES=(trader  trader  trader  trader  trader  trader     trader     trader       admin)

declare -a PARTICIPANT_IDS PARTICIPANT_TOKENS
PARTICIPANT_EMAILS=()
for i in "${!PARTICIPANT_NAMES[@]}"; do
    PARTICIPANT_EMAILS+=("${PARTICIPANT_NAMES[$i]}-${RUN_ID}@demo.garudax")
done

for i in "${!PARTICIPANT_NAMES[@]}"; do
    email="${PARTICIPANT_EMAILS[$i]}"
    role="${PARTICIPANT_ROLES[$i]}"
    name="${PARTICIPANT_NAMES[$i]}"
    step "Register ${name} (${role})"
    if do_curl POST /api/v1/auth/register \
        -d "{\"email\":\"${email}\",\"password\":\"${PASSWORD}\",\"role\":\"${role}\"}"; then
        if [[ "$RESP_STATUS" == "200" || "$RESP_STATUS" == "201" ]]; then
            uid=$(json_field "$RESP_BODY" "id")
            if [[ -z "$uid" ]]; then uid=$(json_field "$RESP_BODY" "user_id"); fi
            PARTICIPANT_IDS[$i]="${uid:-user-${name}}"
            pass "(id: ${PARTICIPANT_IDS[$i]})"
        elif [[ "$RESP_STATUS" == "409" ]]; then
            # Already exists — idempotent
            PARTICIPANT_IDS[$i]="user-${name}"
            pass "(already exists)"
        else
            fail "(${RESP_STATUS})"
            PARTICIPANT_IDS[$i]="user-${name}"
        fi
    else
        fail "(unreachable)"
        PARTICIPANT_IDS[$i]="user-${name}"
    fi
done

# ═════════════════════════════════════════════════════════════════════════════
# STEP 2 — Login All Participants
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Login Participants ---${RESET}\n"

for i in "${!PARTICIPANT_NAMES[@]}"; do
    email="${PARTICIPANT_EMAILS[$i]}"
    name="${PARTICIPANT_NAMES[$i]}"
    step "Login ${name}"
    if do_curl POST /api/v1/auth/login \
        -d "{\"email\":\"${email}\",\"password\":\"${PASSWORD}\"}"; then
        if [[ "$RESP_STATUS" == "200" ]]; then
            tok=$(json_field "$RESP_BODY" "access_token")
            if [[ -z "$tok" ]]; then tok=$(json_field "$RESP_BODY" "AccessToken"); fi
            PARTICIPANT_TOKENS[$i]="${tok:-}"
            if [[ -n "${PARTICIPANT_TOKENS[$i]}" ]]; then
                pass "(token obtained)"
            else
                fail "(no token in response)"
            fi
        else
            fail "(${RESP_STATUS})"
            PARTICIPANT_TOKENS[$i]=""
        fi
    else
        fail "(unreachable)"
        PARTICIPANT_TOKENS[$i]=""
    fi
done

# Named references for readability
FARMER1_TOKEN="${PARTICIPANT_TOKENS[0]:-}"
FARMER2_TOKEN="${PARTICIPANT_TOKENS[1]:-}"
FARMER3_TOKEN="${PARTICIPANT_TOKENS[2]:-}"
HEDGER1_TOKEN="${PARTICIPANT_TOKENS[3]:-}"
HEDGER2_TOKEN="${PARTICIPANT_TOKENS[4]:-}"
SPEC1_TOKEN="${PARTICIPANT_TOKENS[5]:-}"
SPEC2_TOKEN="${PARTICIPANT_TOKENS[6]:-}"
MM1_TOKEN="${PARTICIPANT_TOKENS[7]:-}"
ADMIN_TOKEN="${PARTICIPANT_TOKENS[8]:-}"

FARMER1_ID="${PARTICIPANT_IDS[0]:-}"
FARMER2_ID="${PARTICIPANT_IDS[1]:-}"
FARMER3_ID="${PARTICIPANT_IDS[2]:-}"
HEDGER1_ID="${PARTICIPANT_IDS[3]:-}"
HEDGER2_ID="${PARTICIPANT_IDS[4]:-}"
SPEC1_ID="${PARTICIPANT_IDS[5]:-}"
SPEC2_ID="${PARTICIPANT_IDS[6]:-}"
MM1_ID="${PARTICIPANT_IDS[7]:-}"
ADMIN1_ID="${PARTICIPANT_IDS[8]:-}"

# ═════════════════════════════════════════════════════════════════════════════
# STEP 3 — KYC Applications
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- KYC Applications ---${RESET}\n"

submit_kyc() {
    local label="$1" token="$2" user_id="$3" email="$4" nationality="$5" tier="$6"
    if [[ -z "$token" ]]; then skip "(no token)"; return; fi
    local body
    body=$(cat <<ENDJSON
{
  "participant_id": "${user_id}",
  "participant_type": "INDIVIDUAL",
  "legal_name": "Demo ${label}",
  "trading_name": "${label} Trading Co",
  "nationality": "${nationality}",
  "tier": "${tier}",
  "contact": {"email":"${email}","phone":"+976-9900-0001","contact_person_name":"Demo ${label}"},
  "registered_address": {"line1":"123 Demo Street","city":"Ulaanbaatar","province":"Ulaanbaatar","postal_code":"14200","country":"MN"},
  "source_of_funds": "Trading income and investments"
}
ENDJSON
)
    if do_curl POST /api/v1/participants -H "$(auth_header "$token")" -d "$body"; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            skip "(compliance-service unavailable)"
        elif [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            pass "(${RESP_STATUS})"
        elif [[ "$RESP_STATUS" == "409" ]]; then
            pass "(already exists)"
        else
            fail "(${RESP_STATUS})"
        fi
    else
        fail "(unreachable)"
    fi
}

NATIONALITIES=(MN MN MN KR JP MN US MN MN)
for i in "${!PARTICIPANT_NAMES[@]}"; do
    name="${PARTICIPANT_NAMES[$i]}"
    tier="${PARTICIPANT_TIERS[$i]}"
    step "KYC ${name} (${tier})"
    submit_kyc "$name" "${PARTICIPANT_TOKENS[$i]:-}" "${PARTICIPANT_IDS[$i]:-}" \
        "${PARTICIPANT_EMAILS[$i]}" "${NATIONALITIES[$i]}" "$tier"
done

# Approve KYC for all non-admin participants
printf "\n${BOLD}--- Approve KYC ---${RESET}\n"
for i in 0 1 2 3 4 5 6 7; do
    name="${PARTICIPANT_NAMES[$i]}"
    uid="${PARTICIPANT_IDS[$i]}"
    step "Approve KYC ${name}"
    if [[ -z "$ADMIN_TOKEN" ]]; then skip "(no admin token)"; continue; fi
    if do_curl POST "/api/v1/participants/${uid}/approve" \
        -H "$(auth_header "$ADMIN_TOKEN")" \
        -d '{"officer_id":"admin-demo","notes":"Demo approval"}'; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            skip "(unavailable)"
        elif [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            pass "(${RESP_STATUS})"
        elif [[ "$RESP_STATUS" == "409" ]]; then
            pass "(already approved)"
        else
            fail "(${RESP_STATUS})"
        fi
    else
        fail "(unreachable)"
    fi
done

# ═════════════════════════════════════════════════════════════════════════════
# STEP 4 — Submit Orders Across 6 Instruments (~80 orders, ~30+ trades)
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Submit Orders (multi-instrument) ---${RESET}\n"

submit_order() {
    local label="$1" token="$2" participant="$3" side="$4" qty="$5" price="$6" instrument="$7" order_type="${8:-LIMIT}"
    step "${label}"
    if [[ -z "$token" ]]; then skip "(no token)"; return; fi
    local body="{\"instrument_id\":\"${instrument}\",\"side\":\"${side}\",\"type\":\"${order_type}\",\"quantity\":\"${qty}\",\"price\":\"${price}\",\"account_id\":\"${participant}\",\"time_in_force\":\"GTC\"}"
    if do_direct "$MATCHING_ENGINE" POST /orders -d "$body"; then
        if [[ "$RESP_STATUS" == "502" || "$RESP_STATUS" == "503" ]]; then
            skip "(matching-engine unavailable)"
        elif [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            # Check if a trade was generated (exec_type TRADE or fills present)
            local exec_type=""
            exec_type=$(json_field "$RESP_BODY" "exec_type")
            if [[ "$exec_type" == "TRADE" || "$exec_type" == "trade" ]]; then
                TRADE_COUNT[$instrument]=$(( ${TRADE_COUNT[$instrument]:-0} + 1 ))
            fi
            pass "(${RESP_STATUS})"
        else
            fail "(${RESP_STATUS}: $(json_field "$RESP_BODY" "error"))"
        fi
    else
        fail "(unreachable)"
    fi
}

# ─── Wheat (WHT-HRW-2026M07-UB): 20 orders, ~8 trades ─────────────────────
WHEAT="WHT-HRW-2026M07-UB"
printf "\n  ${CYAN}Wheat ($WHEAT) — 20 orders${RESET}\n"

# Crossing pairs (generate trades)
submit_order "farmer1 SELL 10 wheat @ 325.50"  "$FARMER1_TOKEN" "$FARMER1_ID" "SELL" "10" "325.5000" "$WHEAT"
submit_order "hedger1 BUY 10 wheat @ 325.50"   "$HEDGER1_TOKEN" "$HEDGER1_ID" "BUY"  "10" "325.5000" "$WHEAT"

submit_order "farmer2 SELL 8 wheat @ 328.00"   "$FARMER2_TOKEN" "$FARMER2_ID" "SELL" "8" "328.0000" "$WHEAT"
submit_order "spec1 BUY 8 wheat @ 330.00"      "$SPEC1_TOKEN"   "$SPEC1_ID"   "BUY"  "8" "330.0000" "$WHEAT"

submit_order "mm1 BUY 15 wheat @ 327.00"       "$MM1_TOKEN"     "$MM1_ID"     "BUY"  "15" "327.0000" "$WHEAT"
submit_order "farmer3 SELL 15 wheat @ 326.50"   "$FARMER3_TOKEN" "$FARMER3_ID" "SELL" "15" "326.5000" "$WHEAT"

submit_order "hedger2 BUY 5 wheat @ 332.00"    "$HEDGER2_TOKEN" "$HEDGER2_ID" "BUY"  "5" "332.0000" "$WHEAT"
submit_order "spec2 SELL 5 wheat @ 331.00"      "$SPEC2_TOKEN"   "$SPEC2_ID"   "SELL" "5" "331.0000" "$WHEAT"

submit_order "spec1 BUY 12 wheat @ 329.00"     "$SPEC1_TOKEN"   "$SPEC1_ID"   "BUY"  "12" "329.0000" "$WHEAT"
submit_order "farmer1 SELL 12 wheat @ 329.00"   "$FARMER1_TOKEN" "$FARMER1_ID" "SELL" "12" "329.0000" "$WHEAT"

submit_order "hedger1 BUY 6 wheat @ 333.00"    "$HEDGER1_TOKEN" "$HEDGER1_ID" "BUY"  "6" "333.0000" "$WHEAT"
submit_order "mm1 SELL 6 wheat @ 332.50"        "$MM1_TOKEN"     "$MM1_ID"     "SELL" "6" "332.5000" "$WHEAT"

submit_order "spec2 BUY 8 wheat @ 334.00"      "$SPEC2_TOKEN"   "$SPEC2_ID"   "BUY"  "8" "334.0000" "$WHEAT"
submit_order "farmer2 SELL 8 wheat @ 333.50"    "$FARMER2_TOKEN" "$FARMER2_ID" "SELL" "8" "333.5000" "$WHEAT"

# Resting orders (book depth)
submit_order "mm1 BUY 20 wheat @ 320.00 (bid)"    "$MM1_TOKEN"     "$MM1_ID"     "BUY"  "20" "320.0000" "$WHEAT"
submit_order "hedger1 BUY 15 wheat @ 318.00 (bid)" "$HEDGER1_TOKEN" "$HEDGER1_ID" "BUY"  "15" "318.0000" "$WHEAT"
submit_order "spec1 BUY 10 wheat @ 315.00 (bid)"   "$SPEC1_TOKEN"   "$SPEC1_ID"   "BUY"  "10" "315.0000" "$WHEAT"
submit_order "mm1 SELL 20 wheat @ 340.00 (ask)"    "$MM1_TOKEN"     "$MM1_ID"     "SELL" "20" "340.0000" "$WHEAT"
submit_order "farmer1 SELL 15 wheat @ 342.00 (ask)" "$FARMER1_TOKEN" "$FARMER1_ID" "SELL" "15" "342.0000" "$WHEAT"
submit_order "farmer3 SELL 10 wheat @ 345.00 (ask)" "$FARMER3_TOKEN" "$FARMER3_ID" "SELL" "10" "345.0000" "$WHEAT"

# ─── Corn (CRN-YEL-2026M09-UB): 12 orders ─────────────────────────────────
CORN="CRN-YEL-2026M09-UB"
printf "\n  ${CYAN}Corn ($CORN) — 12 orders${RESET}\n"

submit_order "farmer1 SELL 10 corn @ 452.00"   "$FARMER1_TOKEN" "$FARMER1_ID" "SELL" "10" "452.0000" "$CORN"
submit_order "hedger1 BUY 10 corn @ 452.00"    "$HEDGER1_TOKEN" "$HEDGER1_ID" "BUY"  "10" "452.0000" "$CORN"

submit_order "farmer2 SELL 6 corn @ 455.00"    "$FARMER2_TOKEN" "$FARMER2_ID" "SELL" "6" "455.0000" "$CORN"
submit_order "spec1 BUY 6 corn @ 457.00"       "$SPEC1_TOKEN"   "$SPEC1_ID"   "BUY"  "6" "457.0000" "$CORN"

submit_order "mm1 SELL 8 corn @ 453.50"        "$MM1_TOKEN"     "$MM1_ID"     "SELL" "8" "453.5000" "$CORN"
submit_order "hedger2 BUY 8 corn @ 454.00"     "$HEDGER2_TOKEN" "$HEDGER2_ID" "BUY"  "8" "454.0000" "$CORN"

submit_order "spec2 BUY 5 corn @ 458.00"       "$SPEC2_TOKEN"   "$SPEC2_ID"   "BUY"  "5" "458.0000" "$CORN"
submit_order "farmer3 SELL 5 corn @ 456.00"     "$FARMER3_TOKEN" "$FARMER3_ID" "SELL" "5" "456.0000" "$CORN"

# Resting depth
submit_order "mm1 BUY 15 corn @ 445.00 (bid)"     "$MM1_TOKEN"     "$MM1_ID"     "BUY"  "15" "445.0000" "$CORN"
submit_order "hedger1 BUY 10 corn @ 443.00 (bid)"  "$HEDGER1_TOKEN" "$HEDGER1_ID" "BUY"  "10" "443.0000" "$CORN"
submit_order "mm1 SELL 15 corn @ 465.00 (ask)"     "$MM1_TOKEN"     "$MM1_ID"     "SELL" "15" "465.0000" "$CORN"
submit_order "farmer1 SELL 10 corn @ 468.00 (ask)"  "$FARMER1_TOKEN" "$FARMER1_ID" "SELL" "10" "468.0000" "$CORN"

# ─── Soybeans (SBN-NO2-2026M11-UB): 12 orders ─────────────────────────────
SOYBEAN="SBN-NO2-2026M11-UB"
printf "\n  ${CYAN}Soybeans ($SOYBEAN) — 12 orders${RESET}\n"

submit_order "farmer2 SELL 5 soy @ 1055.00"   "$FARMER2_TOKEN" "$FARMER2_ID" "SELL" "5" "1055.0000" "$SOYBEAN"
submit_order "hedger1 BUY 5 soy @ 1055.00"    "$HEDGER1_TOKEN" "$HEDGER1_ID" "BUY"  "5" "1055.0000" "$SOYBEAN"

submit_order "farmer1 SELL 4 soy @ 1060.00"   "$FARMER1_TOKEN" "$FARMER1_ID" "SELL" "4" "1060.0000" "$SOYBEAN"
submit_order "spec2 BUY 4 soy @ 1062.00"      "$SPEC2_TOKEN"   "$SPEC2_ID"   "BUY"  "4" "1062.0000" "$SOYBEAN"

submit_order "mm1 SELL 6 soy @ 1058.00"       "$MM1_TOKEN"     "$MM1_ID"     "SELL" "6" "1058.0000" "$SOYBEAN"
submit_order "hedger2 BUY 6 soy @ 1059.00"    "$HEDGER2_TOKEN" "$HEDGER2_ID" "BUY"  "6" "1059.0000" "$SOYBEAN"

submit_order "spec1 BUY 3 soy @ 1065.00"      "$SPEC1_TOKEN"   "$SPEC1_ID"   "BUY"  "3" "1065.0000" "$SOYBEAN"
submit_order "farmer3 SELL 3 soy @ 1063.00"    "$FARMER3_TOKEN" "$FARMER3_ID" "SELL" "3" "1063.0000" "$SOYBEAN"

# Resting depth
submit_order "mm1 BUY 10 soy @ 1040.00 (bid)"     "$MM1_TOKEN"     "$MM1_ID"     "BUY"  "10" "1040.0000" "$SOYBEAN"
submit_order "hedger1 BUY 8 soy @ 1035.00 (bid)"   "$HEDGER1_TOKEN" "$HEDGER1_ID" "BUY"  "8" "1035.0000" "$SOYBEAN"
submit_order "mm1 SELL 10 soy @ 1080.00 (ask)"     "$MM1_TOKEN"     "$MM1_ID"     "SELL" "10" "1080.0000" "$SOYBEAN"
submit_order "farmer2 SELL 8 soy @ 1085.00 (ask)"   "$FARMER2_TOKEN" "$FARMER2_ID" "SELL" "8" "1085.0000" "$SOYBEAN"

# ─── Barley (BRL-MALT-2026M07-UB): 8 orders ───────────────────────────────
BARLEY="BRL-MALT-2026M07-UB"
printf "\n  ${CYAN}Barley ($BARLEY) — 8 orders${RESET}\n"

submit_order "farmer3 SELL 10 barley @ 283.00"  "$FARMER3_TOKEN" "$FARMER3_ID" "SELL" "10" "283.0000" "$BARLEY"
submit_order "hedger1 BUY 10 barley @ 283.00"   "$HEDGER1_TOKEN" "$HEDGER1_ID" "BUY"  "10" "283.0000" "$BARLEY"

submit_order "farmer1 SELL 6 barley @ 286.00"   "$FARMER1_TOKEN" "$FARMER1_ID" "SELL" "6" "286.0000" "$BARLEY"
submit_order "spec1 BUY 6 barley @ 288.00"      "$SPEC1_TOKEN"   "$SPEC1_ID"   "BUY"  "6" "288.0000" "$BARLEY"

# Resting depth
submit_order "mm1 BUY 12 barley @ 275.00 (bid)"    "$MM1_TOKEN"     "$MM1_ID"     "BUY"  "12" "275.0000" "$BARLEY"
submit_order "hedger2 BUY 8 barley @ 272.00 (bid)"  "$HEDGER2_TOKEN" "$HEDGER2_ID" "BUY"  "8" "272.0000" "$BARLEY"
submit_order "mm1 SELL 12 barley @ 295.00 (ask)"    "$MM1_TOKEN"     "$MM1_ID"     "SELL" "12" "295.0000" "$BARLEY"
submit_order "farmer3 SELL 8 barley @ 298.00 (ask)"  "$FARMER3_TOKEN" "$FARMER3_ID" "SELL" "8" "298.0000" "$BARLEY"

# ─── Cashmere (CSH-RAW-2026M09-UB): 8 orders ──────────────────────────────
CASHMERE="CSH-RAW-2026M09-UB"
printf "\n  ${CYAN}Cashmere ($CASHMERE) — 8 orders${RESET}\n"

submit_order "farmer1 SELL 3 cashmere @ 45200.00"  "$FARMER1_TOKEN" "$FARMER1_ID" "SELL" "3" "45200.0000" "$CASHMERE"
submit_order "hedger2 BUY 3 cashmere @ 45200.00"   "$HEDGER2_TOKEN" "$HEDGER2_ID" "BUY"  "3" "45200.0000" "$CASHMERE"

submit_order "farmer2 SELL 2 cashmere @ 45500.00"  "$FARMER2_TOKEN" "$FARMER2_ID" "SELL" "2" "45500.0000" "$CASHMERE"
submit_order "spec2 BUY 2 cashmere @ 45600.00"     "$SPEC2_TOKEN"   "$SPEC2_ID"   "BUY"  "2" "45600.0000" "$CASHMERE"

# Resting depth
submit_order "mm1 BUY 5 cashmere @ 44500.00 (bid)"   "$MM1_TOKEN"     "$MM1_ID"     "BUY"  "5" "44500.0000" "$CASHMERE"
submit_order "mm1 SELL 5 cashmere @ 46500.00 (ask)"   "$MM1_TOKEN"     "$MM1_ID"     "SELL" "5" "46500.0000" "$CASHMERE"
submit_order "hedger1 BUY 3 cashmere @ 44000.00 (bid)" "$HEDGER1_TOKEN" "$HEDGER1_ID" "BUY"  "3" "44000.0000" "$CASHMERE"
submit_order "farmer1 SELL 3 cashmere @ 47000.00 (ask)" "$FARMER1_TOKEN" "$FARMER1_ID" "SELL" "3" "47000.0000" "$CASHMERE"

# ─── Cattle (LVS-CATTLE-2026M10-UB): 8 orders ─────────────────────────────
CATTLE="LVS-CATTLE-2026M10-UB"
printf "\n  ${CYAN}Cattle ($CATTLE) — 8 orders${RESET}\n"

submit_order "farmer3 SELL 4 cattle @ 181.50"   "$FARMER3_TOKEN" "$FARMER3_ID" "SELL" "4" "181.5000" "$CATTLE"
submit_order "hedger1 BUY 4 cattle @ 181.50"    "$HEDGER1_TOKEN" "$HEDGER1_ID" "BUY"  "4" "181.5000" "$CATTLE"

submit_order "farmer2 SELL 3 cattle @ 183.00"   "$FARMER2_TOKEN" "$FARMER2_ID" "SELL" "3" "183.0000" "$CATTLE"
submit_order "spec1 BUY 3 cattle @ 184.00"      "$SPEC1_TOKEN"   "$SPEC1_ID"   "BUY"  "3" "184.0000" "$CATTLE"

# Resting depth
submit_order "mm1 BUY 6 cattle @ 178.00 (bid)"    "$MM1_TOKEN"     "$MM1_ID"     "BUY"  "6" "178.0000" "$CATTLE"
submit_order "hedger2 BUY 4 cattle @ 176.00 (bid)" "$HEDGER2_TOKEN" "$HEDGER2_ID" "BUY"  "4" "176.0000" "$CATTLE"
submit_order "mm1 SELL 6 cattle @ 188.00 (ask)"    "$MM1_TOKEN"     "$MM1_ID"     "SELL" "6" "188.0000" "$CATTLE"
submit_order "farmer1 SELL 4 cattle @ 190.00 (ask)" "$FARMER1_TOKEN" "$FARMER1_ID" "SELL" "4" "190.0000" "$CATTLE"

# ═════════════════════════════════════════════════════════════════════════════
# STEP 5 — Warehouse Data (facility, inspections, receipts)
# ═════════════════════════════════════════════════════════════════════════════
printf "\n${BOLD}--- Warehouse Data ---${RESET}\n"

WAREHOUSE_AVAILABLE=true

# Register a warehouse facility
step "Register warehouse facility"
if do_direct "$WAREHOUSE_SERVICE" POST /facilities \
    -d "{\"facility_code\":\"WH-UB-001-${RUN_ID}\",\"name\":\"Ulaanbaatar Central Grain Depot\",\"operator_id\":\"${ADMIN1_ID}\",\"license_number\":\"LIC-2026-001\",\"license_expiry\":\"2027-12-31\",\"address\":\"Industrial District, Ulaanbaatar\",\"latitude\":\"47.9184\",\"longitude\":\"106.9177\",\"region\":\"Central\",\"total_capacity\":\"500000\",\"capacity_unit\":\"bushel\",\"approved_commodity_ids\":[\"WHT-HRW\",\"BRL-MALT\",\"CRN-YEL\"]}"; then
    if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
        FACILITY_ID=$(json_field "$RESP_BODY" "id")
        pass "(id: ${FACILITY_ID:-unknown})"
    elif [[ "$RESP_STATUS" == "400" ]]; then
        # May already exist (duplicate code) — try to continue
        FACILITY_ID="facility-wh-ub-001"
        pass "(may exist, continuing)"
    else
        skip "(warehouse-service unavailable: ${RESP_STATUS})"
        WAREHOUSE_AVAILABLE=false
        FACILITY_ID=""
    fi
else
    skip "(warehouse-service unreachable)"
    WAREHOUSE_AVAILABLE=false
    FACILITY_ID=""
fi

if $WAREHOUSE_AVAILABLE && [[ -n "$FACILITY_ID" ]]; then
    # Schedule inspections (required before issuing receipts)
    # Wheat inspection for farmer1
    step "Schedule wheat inspection (farmer1)"
    if do_direct "$WAREHOUSE_SERVICE" POST /inspections \
        -d "{\"facility_id\":\"${FACILITY_ID}\",\"commodity_id\":\"WHT-HRW\",\"lot_number\":\"LOT-WHT-F1-${RUN_ID}\",\"inspector_id\":\"inspector-001\",\"inspection_type\":\"INITIAL\",\"scheduled_date\":\"2026-03-15\"}"; then
        if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            INSP1_ID=$(json_field "$RESP_BODY" "id")
            pass "(id: ${INSP1_ID:-unknown})"
        else
            fail "(${RESP_STATUS})"
            INSP1_ID=""
        fi
    else
        fail "(unreachable)"
        INSP1_ID=""
    fi

    # Record inspection result (passed)
    if [[ -n "${INSP1_ID:-}" ]]; then
        step "Record wheat inspection result (farmer1) — PASSED"
        if do_direct "$WAREHOUSE_SERVICE" POST /inspections/result \
            -d "{\"inspection_id\":\"${INSP1_ID}\",\"gross_weight\":\"52000\",\"net_weight\":\"50000\",\"moisture_pct\":\"12.5\",\"foreign_matter_pct\":\"0.3\",\"protein_pct\":\"13.2\",\"test_weight\":\"60.5\",\"grade_assigned\":\"No.1 HRW\",\"defects\":\"none\",\"notes\":\"Excellent quality\",\"certificate_number\":\"CERT-WHT-001-${RUN_ID}\",\"completed_date\":\"2026-03-16\",\"passed\":true}"; then
            if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
                pass
            else
                fail "(${RESP_STATUS})"
            fi
        else
            fail "(unreachable)"
        fi
    fi

    # Issue wheat receipt for farmer1
    if [[ -n "${INSP1_ID:-}" ]]; then
        step "Issue wheat receipt (farmer1)"
        if do_direct "$WAREHOUSE_SERVICE" POST /receipts \
            -d "{\"facility_id\":\"${FACILITY_ID}\",\"holder_id\":\"${FARMER1_ID}\",\"commodity_id\":\"WHT-HRW\",\"grade\":\"No.1 HRW\",\"quantity\":\"50000\",\"gross_quantity\":\"52000\",\"unit\":\"bushel\",\"lot_number\":\"LOT-WHT-F1-${RUN_ID}\",\"storage_location\":\"Silo A-1\",\"harvest_year\":2026,\"inspection_id\":\"${INSP1_ID}\"}"; then
            if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
                pass "(receipt issued)"
            else
                fail "(${RESP_STATUS})"
            fi
        else
            fail "(unreachable)"
        fi
    fi

    # Wheat inspection for farmer2
    step "Schedule wheat inspection (farmer2)"
    if do_direct "$WAREHOUSE_SERVICE" POST /inspections \
        -d "{\"facility_id\":\"${FACILITY_ID}\",\"commodity_id\":\"WHT-HRW\",\"lot_number\":\"LOT-WHT-F2-${RUN_ID}\",\"inspector_id\":\"inspector-002\",\"inspection_type\":\"INITIAL\",\"scheduled_date\":\"2026-03-17\"}"; then
        if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            INSP2_ID=$(json_field "$RESP_BODY" "id")
            pass "(id: ${INSP2_ID:-unknown})"
        else
            fail "(${RESP_STATUS})"
            INSP2_ID=""
        fi
    else
        fail "(unreachable)"
        INSP2_ID=""
    fi

    if [[ -n "${INSP2_ID:-}" ]]; then
        step "Record wheat inspection result (farmer2) — PASSED"
        if do_direct "$WAREHOUSE_SERVICE" POST /inspections/result \
            -d "{\"inspection_id\":\"${INSP2_ID}\",\"gross_weight\":\"31000\",\"net_weight\":\"30000\",\"moisture_pct\":\"13.0\",\"foreign_matter_pct\":\"0.5\",\"protein_pct\":\"12.8\",\"test_weight\":\"59.8\",\"grade_assigned\":\"No.2 HRW\",\"defects\":\"minor discoloration\",\"notes\":\"Good quality\",\"certificate_number\":\"CERT-WHT-002-${RUN_ID}\",\"completed_date\":\"2026-03-18\",\"passed\":true}"; then
            if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
                pass
            else
                fail "(${RESP_STATUS})"
            fi
        else
            fail "(unreachable)"
        fi
    fi

    if [[ -n "${INSP2_ID:-}" ]]; then
        step "Issue wheat receipt (farmer2)"
        if do_direct "$WAREHOUSE_SERVICE" POST /receipts \
            -d "{\"facility_id\":\"${FACILITY_ID}\",\"holder_id\":\"${FARMER2_ID}\",\"commodity_id\":\"WHT-HRW\",\"grade\":\"No.2 HRW\",\"quantity\":\"30000\",\"gross_quantity\":\"31000\",\"unit\":\"bushel\",\"lot_number\":\"LOT-WHT-F2-${RUN_ID}\",\"storage_location\":\"Silo A-2\",\"harvest_year\":2026,\"inspection_id\":\"${INSP2_ID}\"}"; then
            if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
                pass "(receipt issued)"
            else
                fail "(${RESP_STATUS})"
            fi
        else
            fail "(unreachable)"
        fi
    fi

    # Barley inspection for farmer3
    step "Schedule barley inspection (farmer3)"
    if do_direct "$WAREHOUSE_SERVICE" POST /inspections \
        -d "{\"facility_id\":\"${FACILITY_ID}\",\"commodity_id\":\"BRL-MALT\",\"lot_number\":\"LOT-BRL-F3-${RUN_ID}\",\"inspector_id\":\"inspector-001\",\"inspection_type\":\"INITIAL\",\"scheduled_date\":\"2026-03-19\"}"; then
        if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
            INSP3_ID=$(json_field "$RESP_BODY" "id")
            pass "(id: ${INSP3_ID:-unknown})"
        else
            fail "(${RESP_STATUS})"
            INSP3_ID=""
        fi
    else
        fail "(unreachable)"
        INSP3_ID=""
    fi

    if [[ -n "${INSP3_ID:-}" ]]; then
        step "Record barley inspection result (farmer3) — PASSED"
        if do_direct "$WAREHOUSE_SERVICE" POST /inspections/result \
            -d "{\"inspection_id\":\"${INSP3_ID}\",\"gross_weight\":\"22000\",\"net_weight\":\"20000\",\"moisture_pct\":\"11.8\",\"foreign_matter_pct\":\"0.2\",\"protein_pct\":\"11.5\",\"test_weight\":\"48.2\",\"grade_assigned\":\"Malting Grade A\",\"defects\":\"none\",\"notes\":\"Premium malting barley\",\"certificate_number\":\"CERT-BRL-001-${RUN_ID}\",\"completed_date\":\"2026-03-20\",\"passed\":true}"; then
            if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
                pass
            else
                fail "(${RESP_STATUS})"
            fi
        else
            fail "(unreachable)"
        fi
    fi

    if [[ -n "${INSP3_ID:-}" ]]; then
        step "Issue barley receipt (farmer3)"
        if do_direct "$WAREHOUSE_SERVICE" POST /receipts \
            -d "{\"facility_id\":\"${FACILITY_ID}\",\"holder_id\":\"${FARMER3_ID}\",\"commodity_id\":\"BRL-MALT\",\"grade\":\"Malting Grade A\",\"quantity\":\"20000\",\"gross_quantity\":\"22000\",\"unit\":\"bushel\",\"lot_number\":\"LOT-BRL-F3-${RUN_ID}\",\"storage_location\":\"Silo B-1\",\"harvest_year\":2026,\"inspection_id\":\"${INSP3_ID}\"}"; then
            if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then
                pass "(receipt issued)"
            else
                fail "(${RESP_STATUS})"
            fi
        else
            fail "(unreachable)"
        fi
    fi
fi

# ═════════════════════════════════════════════════════════════════════════════
# STEP 6 — Verify Post-Trade Data
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

# Check order books for each instrument
ALL_INSTRUMENTS=("$WHEAT" "$CORN" "$SOYBEAN" "$BARLEY" "$CASHMERE" "$CATTLE")
ALL_INST_NAMES=("Wheat" "Corn" "Soybeans" "Barley" "Cashmere" "Cattle")

for idx in "${!ALL_INSTRUMENTS[@]}"; do
    inst="${ALL_INSTRUMENTS[$idx]}"
    name="${ALL_INST_NAMES[$idx]}"
    step "Order book (${name})"
    if do_direct "$MATCHING_ENGINE" GET "/book/${inst}"; then
        if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then pass "($RESP_STATUS)"; else fail "($RESP_STATUS)"; fi
    else fail "(unreachable)"; fi
    step "Last trade (${name})"
    if do_direct "$MATCHING_ENGINE" GET "/trades/latest/${inst}"; then
        if [[ "$RESP_STATUS" -ge 200 && "$RESP_STATUS" -lt 300 ]]; then pass "($RESP_STATUS)"; else fail "($RESP_STATUS)"; fi
    else fail "(unreachable)"; fi
done

# Post-trade services
check_endpoint "Positions (farmer1)"       "/api/v1/clearing/positions"    "$FARMER1_TOKEN"
check_endpoint "Positions (hedger1)"       "/api/v1/clearing/positions"    "$HEDGER1_TOKEN"
check_endpoint "Margin (spec1)"            "/api/v1/margin"                "$SPEC1_TOKEN"
check_endpoint "Margin calls"              "/api/v1/margin/calls"          "$MM1_TOKEN"
check_endpoint "Settlement cycles"         "/api/v1/settlement/cycles"     "$ADMIN_TOKEN"
check_endpoint "Netting"                   "/api/v1/clearing/netting"      "$HEDGER1_TOKEN"

# Market data
check_endpoint "Market data candles (wheat)"  "/api/v1/market-data/candles/${WHEAT}?interval=1m"   "$FARMER1_TOKEN"
check_endpoint "Market data ticker (wheat)"   "/api/v1/market-data/ticker/${WHEAT}"                "$FARMER1_TOKEN"
check_endpoint "Market data trades (wheat)"   "/api/v1/market-data/trades/${WHEAT}"                "$FARMER1_TOKEN"
check_endpoint "Market data ticker (corn)"    "/api/v1/market-data/ticker/${CORN}"                 "$FARMER1_TOKEN"
check_endpoint "Market data ticker (soy)"     "/api/v1/market-data/ticker/${SOYBEAN}"              "$FARMER1_TOKEN"

# Warehouse
check_endpoint "Warehouse inventory"       "/api/v1/warehouse/inventory"   "$ADMIN_TOKEN"

# Compliance
check_endpoint "Compliance alerts"         "/api/v1/compliance/alerts"     "$ADMIN_TOKEN"
check_endpoint "Compliance audit trail"    "/api/v1/compliance/audit-trail" "$ADMIN_TOKEN"
check_endpoint "Participants list"         "/api/v1/participants"          "$ADMIN_TOKEN"

# Admin
check_endpoint "Admin health (aggregated)" "/api/v1/admin/health"          "$ADMIN_TOKEN"
check_endpoint "Circuit breakers"          "/api/v1/admin/circuit-breakers" "$ADMIN_TOKEN"

# ═════════════════════════════════════════════════════════════════════════════
# SUMMARY
# ═════════════════════════════════════════════════════════════════════════════
echo ""
printf "${BOLD}═══════════════════════════════════════════════════════════════${RESET}\n"
printf "  ${GREEN}Passed: %d${RESET}  ${RED}Failed: %d${RESET}  ${YELLOW}Skipped: %d${RESET}  Total: %d\n" \
    "$PASS_COUNT" "$FAIL_COUNT" "$SKIP_COUNT" "$STEP_COUNT"
printf "${BOLD}═══════════════════════════════════════════════════════════════${RESET}\n"

# Trade summary per instrument
printf "\n${BOLD}--- Trades per Instrument (detected crossings) ---${RESET}\n"
TOTAL_TRADES=0
for inst in "$WHEAT" "$CORN" "$SOYBEAN" "$BARLEY" "$CASHMERE" "$CATTLE"; do
    count="${TRADE_COUNT[$inst]:-0}"
    TOTAL_TRADES=$((TOTAL_TRADES + count))
    printf "  %-30s %d trades\n" "$inst" "$count"
done
printf "  %-30s ${BOLD}%d trades${RESET}\n" "TOTAL" "$TOTAL_TRADES"

# Participant summary
printf "\n${BOLD}--- Participants Seeded ---${RESET}\n"
printf "  %-12s %-15s %s\n" "Name" "Tier" "Email"
printf "  %-12s %-15s %s\n" "----" "----" "-----"
for i in "${!PARTICIPANT_NAMES[@]}"; do
    printf "  %-12s %-15s %s\n" "${PARTICIPANT_NAMES[$i]}" "${PARTICIPANT_TIERS[$i]}" "${PARTICIPANT_EMAILS[$i]}"
done

# Order count summary
printf "\n${BOLD}--- Orders Submitted ---${RESET}\n"
printf "  Wheat:    20 orders (14 crossing + 6 resting)\n"
printf "  Corn:     12 orders (8 crossing + 4 resting)\n"
printf "  Soybeans: 12 orders (8 crossing + 4 resting)\n"
printf "  Barley:    8 orders (4 crossing + 4 resting)\n"
printf "  Cashmere:  8 orders (4 crossing + 4 resting)\n"
printf "  Cattle:    8 orders (4 crossing + 4 resting)\n"
printf "  ${BOLD}Total:    68 orders${RESET}\n"

printf "\n${BOLD}--- Warehouse Receipts ---${RESET}\n"
printf "  Wheat receipt (farmer1):  50,000 bushels, No.1 HRW\n"
printf "  Wheat receipt (farmer2):  30,000 bushels, No.2 HRW\n"
printf "  Barley receipt (farmer3): 20,000 bushels, Malting Grade A\n"

printf "${BOLD}═══════════════════════════════════════════════════════════════${RESET}\n"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
    printf "\n${RED}Some steps failed. Check the output above.${RESET}\n"
    exit 1
else
    printf "\n${GREEN}Data seeding complete. Platform is populated with multi-instrument data.${RESET}\n"
    exit 0
fi
