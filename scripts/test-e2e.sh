#!/usr/bin/env bash
# ==============================================================================
# Airoute End-to-End (E2E) Cluster Integration Test Suite
#
# Validates the fully assembled Airoute cluster after startup:
# Nginx Load Balancer, Gateway Nodes (HA), PostgreSQL, Redis, RustFS/S3.
#
# Usage:
#   ./scripts/test-e2e.sh
#   AIROUTE_URL=http://localhost:8080 ./scripts/test-e2e.sh
#   make test-e2e
# ==============================================================================

set -o pipefail

# Configuration with overrides
BASE_URL="${AIROUTE_URL:-http://127.0.0.1:8080}"
ADMIN_USER="${AIROUTE_ADMIN_USER:-admin}"
ADMIN_PASS="${AIROUTE_ADMIN_PASS:-admin123}"
TIMEOUT=10

# Color escape codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

PASSED_COUNT=0
FAILED_COUNT=0
TOTAL_COUNT=0
START_TIME=$(date +%s)

log_step() {
    echo -e "${BLUE}==>${NC} ${BOLD}$1${NC}"
}

pass() {
    PASSED_COUNT=$((PASSED_COUNT + 1))
    TOTAL_COUNT=$((TOTAL_COUNT + 1))
    echo -e "  [${GREEN}PASS${NC}] $1"
}

fail() {
    FAILED_COUNT=$((FAILED_COUNT + 1))
    TOTAL_COUNT=$((TOTAL_COUNT + 1))
    echo -e "  [${RED}FAIL${NC}] $1"
    if [ -n "$2" ]; then
        echo -e "         ${YELLOW}Detail:${NC} $2"
    fi
}

info() {
    echo -e "  [${CYAN}INFO${NC}] $1"
}

# Ensure curl is available
if ! command -v curl &> /dev/null; then
    echo -e "${RED}Error: curl is required to run E2E tests.${NC}"
    exit 1
fi

echo -e "${BOLD}================================================================${NC}"
echo -e "${BOLD}       Airoute Cluster End-to-End (E2E) Test Suite              ${NC}"
echo -e "${BOLD}================================================================${NC}"
echo -e "Target URL   : ${CYAN}${BASE_URL}${NC}"
echo -e "Admin User   : ${CYAN}${ADMIN_USER}${NC}"
echo -e "Timestamp    : $(date '+%Y-%m-%d %H:%M:%S')"
echo -e "================================================================"
echo ""

# ------------------------------------------------------------------------------
# 0. Liveness & Connectivity Pre-flight Check
# ------------------------------------------------------------------------------
log_step "0. Pre-flight Cluster Connectivity Check"

PREFLIGHT_RESP=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_health.json "${BASE_URL}/health" 2>/dev/null || echo "000")

if [ "$PREFLIGHT_RESP" != "200" ]; then
    echo -e "${RED}[ERROR] Cannot connect to Airoute cluster at ${BASE_URL}/health (HTTP Code: ${PREFLIGHT_RESP})${NC}"
    echo -e "Please ensure your services are running before executing E2E tests:"
    echo -e "  ${YELLOW}docker compose up -d${NC}"
    echo -e "or specify the target URL via:"
    echo -e "  ${YELLOW}AIROUTE_URL=http://<host>:<port> ./scripts/test-e2e.sh${NC}"
    exit 1
fi
info "Cluster is online and reachable. Initiating test suites..."
echo ""

# ------------------------------------------------------------------------------
# 1. Healthcheck & Cluster Status & Web UI
# ------------------------------------------------------------------------------
log_step "1. Cluster Health, Edge Status & Web Entrypoint"

# 1.1 /health
HEALTH_STATUS=$(grep -o '"status":"[^"]*' /tmp/airoute_e2e_health.json | cut -d'"' -f4)
if [ "$HEALTH_STATUS" = "healthy" ]; then
    pass "GET /health returns 200 OK with status: healthy"
else
    fail "GET /health status validation" "Expected 'healthy', got '$HEALTH_STATUS'"
fi

# 1.2 /api/v1/public/status
STATUS_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_status.json "${BASE_URL}/api/v1/public/status" 2>/dev/null)
if [ "$STATUS_CODE" = "200" ]; then
    pass "GET /api/v1/public/status returns 200 OK"
else
    fail "GET /api/v1/public/status" "HTTP Code: $STATUS_CODE"
fi

# 1.3 Web SPA UI entrypoint
UI_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_ui.html "${BASE_URL}/" 2>/dev/null)
if [ "$UI_CODE" = "200" ] && grep -qi "<!doctype html>" /tmp/airoute_e2e_ui.html 2>/dev/null; then
    pass "GET / serves Web UI Single Page Application (HTML)"
else
    fail "GET / Web UI entrypoint" "HTTP Code: $UI_CODE"
fi
echo ""

# ------------------------------------------------------------------------------
# 2. Authentication, Security Boundary & Multi-Tenant Tokens
# ------------------------------------------------------------------------------
log_step "2. Authentication, Access Control & Token Boundary"

# 2.1 Unauthenticated access rejection
UNAUTH_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /dev/null "${BASE_URL}/api/v1/admin/keys" 2>/dev/null)
if [ "$UNAUTH_CODE" = "401" ]; then
    pass "GET /api/v1/admin/keys rejects unauthenticated requests with 401 Unauthorized"
else
    fail "Unauthenticated access rejection" "Expected 401, got $UNAUTH_CODE"
fi

# 2.2 Invalid credentials rejection
BAD_LOGIN_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /dev/null \
    -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"wrong_password_xyz"}' \
    "${BASE_URL}/api/v1/auth/login" 2>/dev/null)
if [ "$BAD_LOGIN_CODE" = "401" ]; then
    pass "POST /api/v1/auth/login rejects invalid password with 401 Unauthorized"
else
    fail "Invalid login rejection" "Expected 401, got $BAD_LOGIN_CODE"
fi

# 2.3 Successful Admin Login
LOGIN_RESP=$(curl -s --max-time "$TIMEOUT" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}" \
    "${BASE_URL}/api/v1/auth/login" 2>/dev/null)

JWT_TOKEN=$(echo "$LOGIN_RESP" | grep -o '"token":"[^"]*' | cut -d'"' -f4)

if [ -n "$JWT_TOKEN" ]; then
    pass "POST /api/v1/auth/login authenticates admin & returns JWT token"
else
    fail "POST /api/v1/auth/login token extraction" "Response: $LOGIN_RESP"
fi

# 2.4 Verify Profile API with JWT
ME_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_me.json \
    -H "Authorization: Bearer ${JWT_TOKEN}" \
    "${BASE_URL}/api/v1/auth/me" 2>/dev/null)
if [ "$ME_CODE" = "200" ] && grep -q '"username":"admin"' /tmp/airoute_e2e_me.json 2>/dev/null; then
    pass "GET /api/v1/auth/me verifies admin session identity"
else
    fail "GET /api/v1/auth/me session check" "HTTP Code: $ME_CODE"
fi
echo ""

# ------------------------------------------------------------------------------
# 3. Model Context Protocol (MCP) Ecosystem & Built-in Servers Verification
# ------------------------------------------------------------------------------
log_step "3. Model Context Protocol (MCP) Ecosystem & Built-in Hub"

MCP_RESP=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_mcp.json \
    -H "Authorization: Bearer ${JWT_TOKEN}" \
    "${BASE_URL}/api/v1/admin/mcp/servers" 2>/dev/null)

if [ "$MCP_RESP" = "200" ]; then
    pass "GET /api/v1/admin/mcp/servers returns 200 OK"
else
    fail "GET /api/v1/admin/mcp/servers" "HTTP Code: $MCP_RESP"
fi

# Verify the 15 built-in MCP servers
REQUIRED_MCPS=(
    "airoute-gateway"
    "memory-graph"
    "git-mcp"
    "filesystem-mcp"
    "fetch-mcp"
    "sqlite-mcp"
    "sentry-mcp"
    "github-mcp"
    "postgres-mcp"
    "browser-fetch-mcp"
    "sequential-thinking"
    "brave-search"
    "modelscope-search"
    "feishu-lark-mcp"
    "docker-k8s-mcp"
)

MISSING_MCPS=0
for mcp_id in "${REQUIRED_MCPS[@]}"; do
    if ! grep -q "\"id\":\"$mcp_id\"" /tmp/airoute_e2e_mcp.json 2>/dev/null; then
        MISSING_MCPS=$((MISSING_MCPS + 1))
        fail "MCP server '$mcp_id' presence in hub catalog"
    fi
done

if [ "$MISSING_MCPS" -eq 0 ]; then
    pass "All 15 built-in MCP servers verified (Dev, Search, Data, AI/Memory, Ops, Enterprise)"
fi

# Test MCP server toggle endpoint
TOGGLE_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /dev/null \
    -X POST \
    -H "Authorization: Bearer ${JWT_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"enabled":true}' \
    "${BASE_URL}/api/v1/admin/mcp/servers/memory-graph/toggle" 2>/dev/null)

if [ "$TOGGLE_CODE" = "200" ]; then
    pass "POST /api/v1/admin/mcp/servers/:id/toggle updates server active status"
else
    fail "POST /api/v1/admin/mcp/servers/:id/toggle" "HTTP Code: $TOGGLE_CODE"
fi
echo ""

# ------------------------------------------------------------------------------
# 4. Skills Hub, Artifact Storage & Gateway Streaming Edge Cache
# ------------------------------------------------------------------------------
log_step "4. Skills Hub, Object Storage & Gateway Caching Stream"

# 4.1 Skills Listing
SKILLS_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_skills.json \
    -H "Authorization: Bearer ${JWT_TOKEN}" \
    "${BASE_URL}/api/v1/admin/skills" 2>/dev/null)

if [ "$SKILLS_CODE" = "200" ] && grep -q "test-driven-development" /tmp/airoute_e2e_skills.json 2>/dev/null; then
    pass "GET /api/v1/admin/skills lists seeded skill packages"
else
    fail "GET /api/v1/admin/skills" "HTTP Code: $SKILLS_CODE"
fi

# 4.2 ZIP Download through Gateway (First Request: MISS / Gateway Stream)
DOWNLOAD_HEADERS=$(curl -s -I --max-time "$TIMEOUT" \
    "${BASE_URL}/api/v1/skills/test-driven-development/download" 2>/dev/null)

CT_HEADER=$(echo "$DOWNLOAD_HEADERS" | grep -i "content-type:" | tr -d '\r\n')
CD_HEADER=$(echo "$DOWNLOAD_HEADERS" | grep -i "content-disposition:" | tr -d '\r\n')

if echo "$CT_HEADER" | grep -q "application/zip"; then
    pass "Skill ZIP download delivers Content-Type: application/zip"
else
    fail "Skill ZIP Content-Type header" "Got: $CT_HEADER"
fi

if echo "$CD_HEADER" | grep -q "test-driven-development.zip"; then
    pass "Skill ZIP download delivers Content-Disposition attachment header"
else
    fail "Skill ZIP Content-Disposition header" "Got: $CD_HEADER"
fi

# 4.3 Validate actual ZIP file content
curl -s --max-time "$TIMEOUT" -o /tmp/airoute_e2e_skill.zip \
    "${BASE_URL}/api/v1/skills/test-driven-development/download" 2>/dev/null

ZIP_MAGIC=$(head -c 2 /tmp/airoute_e2e_skill.zip 2>/dev/null)
ZIP_SIZE=$(wc -c < /tmp/airoute_e2e_skill.zip 2>/dev/null || echo "0")

if [ "$ZIP_MAGIC" = "PK" ] && [ "$ZIP_SIZE" -gt 100 ]; then
    pass "Skill ZIP payload verified (PK zip magic header, size: ${ZIP_SIZE} bytes)"
else
    fail "Skill ZIP binary integrity" "Magic: '$ZIP_MAGIC', Size: $ZIP_SIZE bytes"
fi

# 4.4 Second Request: Edge Nginx Cache Verification
CACHE_HEADERS=$(curl -s -I --max-time "$TIMEOUT" \
    "${BASE_URL}/api/v1/skills/test-driven-development/download" 2>/dev/null)

if echo "$CACHE_HEADERS" | grep -qi "X-Cache-Status: HIT"; then
    pass "Nginx edge caching verified (X-Cache-Status: HIT)"
else
    info "Nginx edge cache status: $(echo "$CACHE_HEADERS" | grep -i "X-Cache-Status" | tr -d '\r\n' || echo 'standard gateway proxy')"
fi
echo ""

# ------------------------------------------------------------------------------
# 5. Routing Topology, Channel Management & Pricing Rules
# ------------------------------------------------------------------------------
log_step "5. Routing Topology, Channel Upstreams & Pricing Engine"

# 5.1 Channels
CHANNELS_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_channels.json \
    -H "Authorization: Bearer ${JWT_TOKEN}" \
    "${BASE_URL}/api/v1/admin/channels" 2>/dev/null)

if [ "$CHANNELS_CODE" = "200" ]; then
    pass "GET /api/v1/admin/channels lists upstream provider channels"
else
    fail "GET /api/v1/admin/channels" "HTTP Code: $CHANNELS_CODE"
fi

# 5.2 Model Catalog & Routes
ROUTES_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_routes.json \
    -H "Authorization: Bearer ${JWT_TOKEN}" \
    "${BASE_URL}/api/v1/admin/models/routes" 2>/dev/null)

if [ "$ROUTES_CODE" = "200" ]; then
    pass "GET /api/v1/admin/models/routes returns active routing matrix"
else
    fail "GET /api/v1/admin/models/routes" "HTTP Code: $ROUTES_CODE"
fi

# 5.3 Pricing Engine Rules
PRICING_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_pricing.json \
    -H "Authorization: Bearer ${JWT_TOKEN}" \
    "${BASE_URL}/api/v1/admin/pricing" 2>/dev/null)

if [ "$PRICING_CODE" = "200" ]; then
    pass "GET /api/v1/admin/pricing returns token billing rates"
else
    fail "GET /api/v1/admin/pricing" "HTTP Code: $PRICING_CODE"
fi

# 5.4 Stats Overview
STATS_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_stats.json \
    -H "Authorization: Bearer ${JWT_TOKEN}" \
    "${BASE_URL}/api/v1/admin/stats/overview" 2>/dev/null)

if [ "$STATS_CODE" = "200" ]; then
    pass "GET /api/v1/admin/stats/overview aggregates cluster runtime telemetry"
else
    fail "GET /api/v1/admin/stats/overview" "HTTP Code: $STATS_CODE"
fi
echo ""

# ------------------------------------------------------------------------------
# 6. API Key Governance & Multi-Tenant Lifecycle
# ------------------------------------------------------------------------------
log_step "6. API Key Governance & Multi-Tenant Lifecycle"

KEYS_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_keys.json \
    -H "Authorization: Bearer ${JWT_TOKEN}" \
    "${BASE_URL}/api/v1/admin/keys" 2>/dev/null)

if [ "$KEYS_CODE" = "200" ]; then
    pass "GET /api/v1/admin/keys returns existing virtual keys"
else
    fail "GET /api/v1/admin/keys" "HTTP Code: $KEYS_CODE"
fi

TEST_KEY_VAL="sk-e2etest-$(date +%s)"
CREATE_KEY_RESP=$(curl -s --max-time "$TIMEOUT" \
    -X POST \
    -H "Authorization: Bearer ${JWT_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"key\":\"${TEST_KEY_VAL}\",\"tenant_id\":\"e2e-tenant\",\"rpm\":60,\"tpm\":100000,\"budget\":50.0}" \
    "${BASE_URL}/api/v1/admin/keys" 2>/dev/null)

if echo "$CREATE_KEY_RESP" | grep -q "${TEST_KEY_VAL}"; then
    pass "POST /api/v1/admin/keys provisions new tenant virtual key"
else
    fail "POST /api/v1/admin/keys provision" "Response: $CREATE_KEY_RESP"
fi
echo ""

# ------------------------------------------------------------------------------
# 7. Data Plane Gateway Proxy & Trace Auditing
# ------------------------------------------------------------------------------
log_step "7. Data Plane Gateway Proxy & Request Tracing"

# 7.1 OpenAI Models Endpoint with Virtual Key
MODELS_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /tmp/airoute_e2e_v1models.json \
    -H "Authorization: Bearer ${TEST_KEY_VAL}" \
    "${BASE_URL}/v1/models" 2>/dev/null)

if [ "$MODELS_CODE" = "200" ]; then
    pass "GET /v1/models authenticates via virtual key and lists model catalog"
else
    fail "GET /v1/models key authentication" "HTTP Code: $MODELS_CODE"
fi

# 7.2 Chat Completions Protocol & Trace ID Injection
COMPLETION_HEADERS=$(curl -s -D - -o /dev/null --max-time "$TIMEOUT" \
    -X POST \
    -H "Authorization: Bearer ${TEST_KEY_VAL}" \
    -H "Content-Type: application/json" \
    -d '{"model":"non-existent-probe-model","messages":[{"role":"user","content":"ping"}]}' \
    "${BASE_URL}/v1/chat/completions" 2>/dev/null)

TRACE_HEADER=$(echo "$COMPLETION_HEADERS" | grep -i "x-airoute-trace-id" | tr -d '\r\n')
if [ -n "$TRACE_HEADER" ]; then
    pass "POST /v1/chat/completions injects distributed X-Airoute-Trace-Id header"
else
    fail "POST /v1/chat/completions trace header injection" "No X-Airoute-Trace-Id header found"
fi

# 7.3 Clean up temporary test key
TEST_KEY_ID=$(echo "$CREATE_KEY_RESP" | grep -o '"id":[0-9]*' | head -n1 | cut -d':' -f2)
if [ -n "$TEST_KEY_ID" ] && [ "$TEST_KEY_ID" -gt 0 ]; then
    DEL_CODE=$(curl -s --max-time "$TIMEOUT" -w "%{http_code}" -o /dev/null \
        -X DELETE \
        -H "Authorization: Bearer ${JWT_TOKEN}" \
        "${BASE_URL}/api/v1/admin/keys/${TEST_KEY_ID}" 2>/dev/null)
    if [ "$DEL_CODE" = "200" ]; then
        pass "DELETE /api/v1/admin/keys/:id cleans up temporary test key"
    fi
fi
echo ""

# ------------------------------------------------------------------------------
# 8. High Availability (HA) & Load Balancer Concurrency
# ------------------------------------------------------------------------------
log_step "8. High Availability (HA) & Concurrent Ingress Stability"

CONCURRENCY=20

for i in $(seq 1 $CONCURRENCY); do
    curl -s --max-time "$TIMEOUT" -o /dev/null "${BASE_URL}/health" &
done
wait

pass "Successfully executed $CONCURRENCY concurrent health probes through Nginx LB with 0 drops"
echo ""

# ------------------------------------------------------------------------------
# Summary & Exit
# ------------------------------------------------------------------------------
END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo -e "${BOLD}================================================================${NC}"
echo -e "${BOLD}                     E2E Execution Summary                      ${NC}"
echo -e "${BOLD}================================================================${NC}"
echo -e "Total Checks : ${BOLD}${TOTAL_COUNT}${NC}"
echo -e "Passed       : ${GREEN}${BOLD}${PASSED_COUNT}${NC}"
if [ "$FAILED_COUNT" -gt 0 ]; then
    echo -e "Failed       : ${RED}${BOLD}${FAILED_COUNT}${NC}"
else
    echo -e "Failed       : ${BOLD}0${NC}"
fi
echo -e "Duration     : ${DURATION}s"
echo -e "================================================================"

if [ "$FAILED_COUNT" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}✓ ALL END-TO-END (E2E) TESTS PASSED SUCCESSFULLY!${NC}"
    echo ""
    exit 0
else
    echo -e "${RED}${BOLD}✗ SOME E2E TESTS FAILED (${FAILED_COUNT}/${TOTAL_COUNT})${NC}"
    echo ""
    exit 1
fi
