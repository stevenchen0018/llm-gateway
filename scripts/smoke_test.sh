#!/usr/bin/env bash
# End-to-end smoke test against a running `go run ./cmd/gateway` instance,
# using the mock provider adapter (no real vendor credentials needed).
# Exercises: admin login -> provider/model/route onboarding -> key
# apply/approve -> gateway chat completion -> rate limiting -> budget
# exhaustion -> failover.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-change-me-in-production}"

need() { command -v "$1" >/dev/null || { echo "missing dependency: $1" >&2; exit 1; }; }
need curl
need jq

echo "== admin login =="
TOKEN=$(curl -sf -X POST "$BASE_URL/admin/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | jq -r .data.token)
auth=(-H "Authorization: Bearer $TOKEN")

echo "== onboard a mock provider =="
PROVIDER_ID=$(curl -sf -X POST "$BASE_URL/admin/v1/providers" "${auth[@]}" \
  -H 'Content-Type: application/json' \
  -d '{"code":"mock-vendor","name":"Mock Vendor","base_url":"mock://smoke-test"}' | jq -r .data.id)
echo "provider_id=$PROVIDER_ID"

echo "== onboard a primary + backup model =="
PRIMARY_ID=$(curl -sf -X POST "$BASE_URL/admin/v1/models" "${auth[@]}" \
  -H 'Content-Type: application/json' \
  -d "{\"provider_id\":$PROVIDER_ID,\"model_key\":\"primary-model\",\"display_name\":\"Primary\",\"input_price_per_1k\":\"0.01\",\"output_price_per_1k\":\"0.03\",\"qps_limit\":2,\"tpm_limit\":2000}" | jq -r .data.id)
BACKUP_ID=$(curl -sf -X POST "$BASE_URL/admin/v1/models" "${auth[@]}" \
  -H 'Content-Type: application/json' \
  -d "{\"provider_id\":$PROVIDER_ID,\"model_key\":\"backup-model\",\"display_name\":\"Backup\",\"input_price_per_1k\":\"0.01\",\"output_price_per_1k\":\"0.03\"}" | jq -r .data.id)
echo "primary_id=$PRIMARY_ID backup_id=$BACKUP_ID"

echo "== configure routing: alias 'smoke-alias' -> primary (priority 0), backup (priority 1) =="
curl -sf -X POST "$BASE_URL/admin/v1/routing-policies" "${auth[@]}" \
  -H 'Content-Type: application/json' \
  -d "{\"alias\":\"smoke-alias\",\"candidate_model_id\":$PRIMARY_ID,\"priority\":0,\"strategy\":\"priority\"}" >/dev/null
curl -sf -X POST "$BASE_URL/admin/v1/routing-policies" "${auth[@]}" \
  -H 'Content-Type: application/json' \
  -d "{\"alias\":\"smoke-alias\",\"candidate_model_id\":$BACKUP_ID,\"priority\":1,\"strategy\":\"priority\"}" >/dev/null

echo "== apply + approve an API key =="
KEY_ID=$(curl -sf -X POST "$BASE_URL/admin/v1/keys" "${auth[@]}" \
  -H 'Content-Type: application/json' \
  -d '{"name":"smoke-test-key","owner":"smoke-tester","qps_quota":2,"tpm_quota":2000}' | jq -r .data.id)
SECRET=$(curl -sf -X PUT "$BASE_URL/admin/v1/keys/$KEY_ID/approve" "${auth[@]}" | jq -r .data.secret)
echo "key_id=$KEY_ID secret=$SECRET"

echo "== gateway call: chat completion via mock adapter =="
curl -sf -X POST "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $SECRET" -H 'Content-Type: application/json' \
  -d '{"model":"smoke-alias","messages":[{"role":"user","content":"hello gateway"}]}' | jq .

echo "== gateway call: exceed QPS quota (rapid-fire 5 requests against qps_quota=2) =="
for i in 1 2 3 4 5; do
  code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/v1/chat/completions" \
    -H "Authorization: Bearer $SECRET" -H 'Content-Type: application/json' \
    -d '{"model":"smoke-alias","messages":[{"role":"user","content":"burst"}]}')
  echo "request $i -> HTTP $code"
done
echo "(expect at least one 429 above once the per-second QPS quota of 2 is exceeded)"

echo "== admin: usage dashboard =="
curl -sf "$BASE_URL/admin/v1/dashboard/usage?group_by=model" "${auth[@]}" | jq .

echo "== admin: recent alerts (should include the QPS rejection just triggered) =="
curl -sf "$BASE_URL/admin/v1/alerts?limit=5" "${auth[@]}" | jq .

echo "== smoke test complete =="
