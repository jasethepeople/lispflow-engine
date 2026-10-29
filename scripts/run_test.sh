#!/usr/bin/env bash
# Proves: engine receives input -> processes state rule (pricing plan) -> returns evaluation.
# Usage: ./scripts/run_test.sh   (assumes `docker compose up -d` already ran)
set -euo pipefail

HOST="${ENGINE_HOST:-http://localhost:8080}"
PAYLOAD="$(dirname "$0")/../testdata/sample_payload.json"

echo "-> waiting for engine health..."
for i in $(seq 1 20); do
  if curl -sf "$HOST/health" > /dev/null; then break; fi
  sleep 1
done
curl -sf "$HOST/health"; echo

echo "-> loading plan"
LOAD_BODY=$(jq '.load_plan' "$PAYLOAD")
curl -sf -X POST "$HOST/plans" -H "Content-Type: application/json" -d "$LOAD_BODY"
echo

echo "-> evaluating usage against plan"
EVAL_BODY=$(jq '.evaluate' "$PAYLOAD")
RESPONSE=$(curl -sf -X POST "$HOST/evaluate" -H "Content-Type: application/json" -d "$EVAL_BODY")
echo "$RESPONSE"

# Expected: compute tiered (100@0.05 + 50@0.04=2.00 => 5+2=7.00) + storage overage
# (200GB over 1000 @ 0.02 = 4.00) + egress (50 * 0.12 = 6.00) = 17.00, within [10,500] cap.
EXPECTED_COST="17"
ACTUAL_COST=$(echo "$RESPONSE" | jq -r '.cost')
if (( $(echo "$ACTUAL_COST == $EXPECTED_COST" | bc -l) )); then
  echo "PASS: cost = $ACTUAL_COST (expected $EXPECTED_COST)"
else
  echo "FAIL: cost = $ACTUAL_COST (expected $EXPECTED_COST)"
  exit 1
fi
