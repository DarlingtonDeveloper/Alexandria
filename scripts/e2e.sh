#!/usr/bin/env bash
set -euo pipefail

BASE="http://localhost:${ALEXANDRIA_PORT:-8500}/api/v1"
FAIL=0

pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1 — $2"; FAIL=1; }

echo "=== Alexandria E2E Smoke Tests ==="

# 1. Health check
echo "--- Health ---"
HTTP=$(curl -s -o /tmp/e2e_body -w '%{http_code}' "$BASE/health")
if [ "$HTTP" = "200" ]; then
  pass "GET /api/v1/health → 200"
else
  fail "GET /api/v1/health" "expected 200, got $HTTP"
fi

# 2. Create knowledge
echo "--- Knowledge CRUD ---"
HTTP=$(curl -s -o /tmp/e2e_body -w '%{http_code}' \
  -X POST "$BASE/knowledge" \
  -H "Content-Type: application/json" \
  -H "X-Agent-ID: e2e-test" \
  -d '{"content":"E2E test knowledge entry","category":"discovery","scope":"public","tags":["e2e"]}')
if [ "$HTTP" = "201" ]; then
  pass "POST /api/v1/knowledge → 201"
else
  fail "POST /api/v1/knowledge" "expected 201, got $HTTP (body: $(cat /tmp/e2e_body))"
fi

# Extract ID from response
KNOWLEDGE_ID=$(python3 -c "import json,sys; print(json.load(sys.stdin)['data']['id'])" < /tmp/e2e_body 2>/dev/null || echo "")
if [ -z "$KNOWLEDGE_ID" ]; then
  fail "Extract knowledge ID" "could not parse ID from response"
  # Try jq as fallback
  KNOWLEDGE_ID=$(jq -r '.data.id' /tmp/e2e_body 2>/dev/null || echo "")
fi

# 3. List knowledge
HTTP=$(curl -s -o /tmp/e2e_body -w '%{http_code}' \
  -H "X-Agent-ID: e2e-test" \
  "$BASE/knowledge")
if [ "$HTTP" = "200" ]; then
  pass "GET /api/v1/knowledge → 200"
else
  fail "GET /api/v1/knowledge" "expected 200, got $HTTP"
fi

# 4. Get knowledge by ID
if [ -n "$KNOWLEDGE_ID" ]; then
  HTTP=$(curl -s -o /tmp/e2e_body -w '%{http_code}' \
    -H "X-Agent-ID: e2e-test" \
    "$BASE/knowledge/$KNOWLEDGE_ID")
  if [ "$HTTP" = "200" ]; then
    pass "GET /api/v1/knowledge/$KNOWLEDGE_ID → 200"
  else
    fail "GET /api/v1/knowledge/$KNOWLEDGE_ID" "expected 200, got $HTTP"
  fi

  # 5. Delete knowledge
  HTTP=$(curl -s -o /tmp/e2e_body -w '%{http_code}' \
    -X DELETE \
    -H "X-Agent-ID: e2e-test" \
    "$BASE/knowledge/$KNOWLEDGE_ID")
  if [ "$HTTP" = "200" ]; then
    pass "DELETE /api/v1/knowledge/$KNOWLEDGE_ID → 200"
  else
    fail "DELETE /api/v1/knowledge/$KNOWLEDGE_ID" "expected 200, got $HTTP"
  fi

  # 6. Verify deletion (should 404)
  HTTP=$(curl -s -o /tmp/e2e_body -w '%{http_code}' \
    -H "X-Agent-ID: e2e-test" \
    "$BASE/knowledge/$KNOWLEDGE_ID")
  if [ "$HTTP" = "404" ]; then
    pass "GET deleted knowledge → 404"
  else
    fail "GET deleted knowledge" "expected 404, got $HTTP"
  fi
else
  fail "Knowledge CRUD" "skipped — no ID extracted"
fi

# 7. Stats endpoint
echo "--- Stats ---"
HTTP=$(curl -s -o /tmp/e2e_body -w '%{http_code}' \
  -H "X-Agent-ID: e2e-test" \
  "$BASE/stats")
if [ "$HTTP" = "200" ]; then
  pass "GET /api/v1/stats → 200"
else
  fail "GET /api/v1/stats" "expected 200, got $HTTP"
fi

echo ""
if [ "$FAIL" -eq 0 ]; then
  echo "All Alexandria E2E tests passed."
else
  echo "Some Alexandria E2E tests FAILED."
  exit 1
fi
