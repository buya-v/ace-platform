#!/usr/bin/env bash
# Unit tests for scripts/demo.sh
# Tests the script's behavior against a mock HTTP server (Python or ncat).
# Run: bash tests/demo/demo_test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEMO_SCRIPT="$PROJECT_ROOT/scripts/demo.sh"

PASS=0
FAIL=0
MOCK_PID=""
MOCK_PORT=""

cleanup() {
    if [[ -n "$MOCK_PID" ]]; then
        kill "$MOCK_PID" 2>/dev/null || true
        wait "$MOCK_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# ── Test helpers ─────────────────────────────────────────────────────────────
assert_eq() {
    local label="$1" expected="$2" actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        PASS=$((PASS + 1))
        printf "  \033[0;32mPASS\033[0m  %s\n" "$label"
    else
        FAIL=$((FAIL + 1))
        printf "  \033[0;31mFAIL\033[0m  %s (expected '%s', got '%s')\n" "$label" "$expected" "$actual"
    fi
}

assert_contains() {
    local label="$1" expected="$2" actual="$3"
    if echo "$actual" | grep -qF "$expected"; then
        PASS=$((PASS + 1))
        printf "  \033[0;32mPASS\033[0m  %s\n" "$label"
    else
        FAIL=$((FAIL + 1))
        printf "  \033[0;31mFAIL\033[0m  %s (expected output to contain '%s')\n" "$label" "$expected"
    fi
}

assert_exit() {
    local label="$1" expected="$2" actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        PASS=$((PASS + 1))
        printf "  \033[0;32mPASS\033[0m  %s\n" "$label"
    else
        FAIL=$((FAIL + 1))
        printf "  \033[0;31mFAIL\033[0m  %s (expected exit %s, got %s)\n" "$label" "$expected" "$actual"
    fi
}

# ── Find a free port ─────────────────────────────────────────────────────────
find_free_port() {
    python3 -c 'import socket; s=socket.socket(); s.bind(("",0)); print(s.getsockname()[1]); s.close()' 2>/dev/null \
        || echo 18923
}

# ── Mock HTTP server using Python ────────────────────────────────────────────
start_mock_server() {
    local port="$1" behavior="$2"
    MOCK_PORT="$port"

    case "$behavior" in
        healthy)
            # Returns 200 for /healthz, proper JSON for auth/orders/etc.
            python3 -c "
import http.server, json, sys, threading

class Handler(http.server.BaseHTTPRequestHandler):
    user_counter = 0
    def log_message(self, *a): pass
    def do_GET(self):
        if self.path == '/healthz':
            self.send_response(200)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(b'{\"status\":\"ok\"}')
        elif '/clearing/positions' in self.path:
            self.send_response(200)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(b'{\"positions\":[]}')
        elif '/margin' in self.path:
            self.send_response(200)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(b'{\"margin\":\"1000.0000\"}')
        elif '/settlement' in self.path:
            self.send_response(200)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(b'{\"cycles\":[]}')
        elif '/instruments/' in self.path:
            self.send_response(200)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(b'{\"bids\":[],\"asks\":[]}')
        elif '/api/v1/' in self.path:
            self.send_response(200)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(b'{\"data\":[]}')
        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(length) if length else b''
        if '/auth/register' in self.path:
            Handler.user_counter += 1
            uid = 'user-' + str(Handler.user_counter)
            data = json.loads(body) if body else {}
            self.send_response(201)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(json.dumps({'id': uid, 'email': data.get('email','')}).encode())
        elif '/auth/login' in self.path:
            self.send_response(200)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(b'{\"access_token\":\"mock-jwt-token-12345\",\"refresh_token\":\"mock-refresh\",\"expires_in\":3600}')
        elif '/participants' in self.path and '/approve' in self.path:
            self.send_response(200)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(b'{\"status\":\"APPROVED\"}')
        elif '/participants' in self.path:
            data = json.loads(body) if body else {}
            pid = data.get('participant_id', 'p-1')
            self.send_response(201)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(json.dumps({'participant_id': pid, 'status': 'PENDING'}).encode())
        elif '/orders' in self.path:
            self.send_response(201)
            self.send_header('Content-Type','application/json')
            self.end_headers()
            self.wfile.write(b'{\"order_id\":\"ord-1\",\"status\":\"ACCEPTED\"}')
        else:
            self.send_response(404)
            self.end_headers()

srv = http.server.HTTPServer(('127.0.0.1', $port), Handler)
srv.serve_forever()
" &
            ;;
        unhealthy)
            # Returns 503 for everything
            python3 -c "
import http.server
class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        self.send_response(503)
        self.end_headers()
    def do_POST(self):
        self.send_response(503)
        self.end_headers()
srv = http.server.HTTPServer(('127.0.0.1', $port), Handler)
srv.serve_forever()
" &
            ;;
    esac
    MOCK_PID=$!
    # Wait for server to be ready
    for i in $(seq 1 20); do
        if curl -s --max-time 1 "http://127.0.0.1:${port}/healthz" >/dev/null 2>&1; then
            return 0
        fi
        sleep 0.1
    done
    echo "WARN: mock server may not be ready"
}

stop_mock_server() {
    if [[ -n "$MOCK_PID" ]]; then
        kill "$MOCK_PID" 2>/dev/null || true
        wait "$MOCK_PID" 2>/dev/null || true
        MOCK_PID=""
    fi
}

# ══════════════════════════════════════════════════════════════════════════════
echo ""
echo "Demo Script Unit Tests"
echo "═══════════════════════════════════════════════════════════════"

# ── Test 1: --help flag ──────────────────────────────────────────────────────
echo ""
echo "Test Group: CLI flags"

output=$("$DEMO_SCRIPT" --help 2>&1) || true
assert_contains "--help prints usage" "GATEWAY_URL" "$output"
assert_contains "--help prints usage banner" "GarudaX Platform" "$output"

exit_code=0
"$DEMO_SCRIPT" -h >/dev/null 2>&1 || exit_code=$?
assert_eq "-h exits 0" "0" "$exit_code"

# ── Test 2: Gateway unreachable → exit 1 ─────────────────────────────────────
echo ""
echo "Test Group: Gateway unreachable"

exit_code=0
output=$(GATEWAY_URL="http://127.0.0.1:19999" "$DEMO_SCRIPT" 2>&1) || exit_code=$?
assert_eq "unreachable gateway exits 1" "1" "$exit_code"
assert_contains "unreachable shows FAIL" "FAIL" "$output"
assert_contains "unreachable mentions gateway" "gateway" "$output"

# ── Test 3: Healthy mock server → full flow passes ───────────────────────────
echo ""
echo "Test Group: Full flow with healthy mock"

PORT=$(find_free_port)
start_mock_server "$PORT" "healthy"

exit_code=0
output=$(GATEWAY_URL="http://127.0.0.1:${PORT}" "$DEMO_SCRIPT" 2>&1) || exit_code=$?
assert_eq "healthy flow exits 0" "0" "$exit_code"
assert_contains "output contains PASS" "PASS" "$output"
assert_contains "output has Step 1" "Step 1" "$output"
assert_contains "output has Step 2" "Step 2" "$output"
assert_contains "output has Step 3" "Step 3" "$output"
assert_contains "output has Step 4" "Step 4" "$output"
assert_contains "output has Step 5" "Step 5" "$output"
assert_contains "output has Summary" "Summary" "$output"
assert_contains "output shows Passed count" "Passed:" "$output"
assert_contains "output shows RESULT: PASS" "RESULT: PASS" "$output"

# Verify registration steps passed
assert_contains "trader1 registered" "register trader1" "$output"
assert_contains "trader2 registered" "register trader2" "$output"
assert_contains "admin registered" "register admin" "$output"

# Verify login steps passed
assert_contains "trader1 logged in" "login trader1" "$output"
assert_contains "trader2 logged in" "login trader2" "$output"
assert_contains "admin logged in" "login admin" "$output"

# Verify trading steps present
assert_contains "buy order submitted" "buy order" "$output"
assert_contains "sell order submitted" "sell order" "$output"

stop_mock_server

# ── Test 4: Unhealthy gateway (503 on healthz) → exit 1 ─────────────────────
echo ""
echo "Test Group: Unhealthy gateway"

PORT=$(find_free_port)
start_mock_server "$PORT" "unhealthy"

exit_code=0
output=$(GATEWAY_URL="http://127.0.0.1:${PORT}" "$DEMO_SCRIPT" 2>&1) || exit_code=$?
assert_eq "unhealthy gateway exits 1" "1" "$exit_code"
assert_contains "unhealthy shows gateway FAIL" "FAIL" "$output"

stop_mock_server

# ── Test 5: Script is executable ─────────────────────────────────────────────
echo ""
echo "Test Group: File properties"
assert_eq "script is executable" "true" "$(test -x "$DEMO_SCRIPT" && echo true || echo false)"
assert_contains "script has shebang" "#!/usr/bin/env bash" "$(head -1 "$DEMO_SCRIPT")"

# ══════════════════════════════════════════════════════════════════════════════
echo ""
echo "═══════════════════════════════════════════════════════════════"
printf "Results: \033[0;32m%d passed\033[0m, \033[0;31m%d failed\033[0m\n" "$PASS" "$FAIL"
echo ""

if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi
exit 0
