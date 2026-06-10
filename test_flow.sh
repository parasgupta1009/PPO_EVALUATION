#!/bin/bash

BASE_URL="http://localhost:8080"
PASS=0
FAIL=0

check() {
    local desc="$1"
    local expected_status="$2"
    local actual_status="$3"
    local body="$4"

    if [ "$actual_status" -eq "$expected_status" ]; then
        echo "✓ PASS: $desc (status=$actual_status)"
        echo "  Body: $body"
        PASS=$((PASS + 1))
    else
        echo "✗ FAIL: $desc (expected=$expected_status, got=$actual_status)"
        echo "  Body: $body"
        FAIL=$((FAIL + 1))
    fi
    echo ""
}

echo "========================================="
echo "  KV Store - Manual Integration Tests"
echo "========================================="
echo ""

# --- SET Operations ---
echo "--- SET Operations ---"
echo ""

# 1. SET happy path
resp=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/keys" \
  -H "Content-Type: application/json" \
  -d '{"key":"user:1","value":"Alice","ttl_seconds":60}')
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "SET user:1 with TTL 60s" 201 "$status" "$body"

# 2. SET another key with short TTL (for expiry test later)
resp=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/keys" \
  -H "Content-Type: application/json" \
  -d '{"key":"temp","value":"short-lived","ttl_seconds":2}')
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "SET temp with TTL 2s" 201 "$status" "$body"

# 3. SET with missing key (error)
resp=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/keys" \
  -H "Content-Type: application/json" \
  -d '{"value":"nokey","ttl_seconds":10}')
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "SET with missing key → 400" 400 "$status" "$body"

# 4. SET with invalid TTL (error)
resp=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/keys" \
  -H "Content-Type: application/json" \
  -d '{"key":"bad","value":"v","ttl_seconds":0}')
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "SET with ttl_seconds=0 → 400" 400 "$status" "$body"

# 5. SET with invalid JSON (error)
resp=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/keys" \
  -H "Content-Type: application/json" \
  -d 'not json')
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "SET with invalid JSON → 400" 400 "$status" "$body"

# --- GET Operations ---
echo "--- GET Operations ---"
echo ""

# 6. GET existing key
resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/keys/user:1")
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "GET user:1 → 200" 200 "$status" "$body"

# 7. GET nonexistent key
resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/keys/nonexistent")
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "GET nonexistent key → 404" 404 "$status" "$body"

# 8. SET overwrite and GET latest value
curl -s -X POST "$BASE_URL/keys" \
  -H "Content-Type: application/json" \
  -d '{"key":"user:1","value":"Alice-Updated","ttl_seconds":60}' > /dev/null
resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/keys/user:1")
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "GET overwritten user:1 → latest value" 200 "$status" "$body"

# 9. Wait for temp to expire, then GET
echo "  Waiting 3s for 'temp' key to expire..."
sleep 3

resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/keys/temp")
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "GET expired temp → 410" 410 "$status" "$body"

# --- DELETE Operations ---
echo "--- DELETE Operations ---"
echo ""

# 10. DELETE existing key
resp=$(curl -s -w "\n%{http_code}" -X DELETE "$BASE_URL/keys/user:1")
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "DELETE user:1 → 200 deleted successfully" 200 "$status" "$body"

# 11. GET after delete
resp=$(curl -s -w "\n%{http_code}" "$BASE_URL/keys/user:1")
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "GET user:1 after delete → 404" 404 "$status" "$body"

# 12. DELETE nonexistent key
resp=$(curl -s -w "\n%{http_code}" -X DELETE "$BASE_URL/keys/ghost")
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "DELETE nonexistent key → 404" 404 "$status" "$body"

# 13. SET a key with short TTL, wait, then DELETE expired
curl -s -X POST "$BASE_URL/keys" \
  -H "Content-Type: application/json" \
  -d '{"key":"expire-del","value":"v","ttl_seconds":1}' > /dev/null
echo "  Waiting 2s for 'expire-del' to expire..."
sleep 2

resp=$(curl -s -w "\n%{http_code}" -X DELETE "$BASE_URL/keys/expire-del")
status=$(echo "$resp" | tail -1)
body=$(echo "$resp" | sed '$d')
check "DELETE expired key → 410" 410 "$status" "$body"

# --- Summary ---
echo "========================================="
echo "  Results: $PASS passed, $FAIL failed"
echo "========================================="

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
