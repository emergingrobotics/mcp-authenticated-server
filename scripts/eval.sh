#!/usr/bin/env bash
set -euo pipefail

# EVAL-01..07: RAG evaluation script.
# Reads an eval JSON file, searches via the MCP server, judges results via the Anthropic API.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EVAL_FILE="${1:-}"
SERVER_URL="${SERVER_URL:-http://localhost:8080}"

if [[ -z "${EVAL_FILE}" ]]; then
    echo "Usage: $0 <eval-file>" >&2
    echo "Example: $0 data/evals/evals.json" >&2
    exit 1
fi

: "${ANTHROPIC_API_KEY:?Set ANTHROPIC_API_KEY}"

if [[ ! -f "${EVAL_FILE}" ]]; then
    echo "Eval file not found: ${EVAL_FILE}" >&2
    exit 1
fi

TOKEN=$("${SCRIPT_DIR}/get-token.sh")

TOTAL=$(jq length "${EVAL_FILE}")
PASS=0
FAIL=0

echo "Running ${TOTAL} evaluations from ${EVAL_FILE}..."
echo "---"

for i in $(seq 0 $((TOTAL - 1))); do
    QUERY=$(jq -r ".[$i].query" "${EVAL_FILE}")
    EXPECTED=$(jq -r ".[$i].expected" "${EVAL_FILE}")
    CRITERIA=$(jq -r ".[$i].criteria // \"Does the result contain the expected information?\"" "${EVAL_FILE}")

    echo "[$((i + 1))/${TOTAL}] Query: ${QUERY}"

    # Search via MCP server
    SEARCH_RESULT=$(curl -sf -X POST "${SERVER_URL}/search" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        -d "$(jq -n --arg q "${QUERY}" '{"query": $q}')" 2>&1) || {
        echo "  FAIL: Search request failed"
        FAIL=$((FAIL + 1))
        continue
    }

    # Judge via Anthropic API
    JUDGE_PROMPT="You are an evaluation judge. Given the search query, expected answer, search result, and evaluation criteria, determine if the search result is acceptable.

Query: ${QUERY}
Expected: ${EXPECTED}
Criteria: ${CRITERIA}

Search Result:
${SEARCH_RESULT}

Respond with exactly PASS or FAIL on the first line, followed by a brief explanation."

    JUDGE_RESPONSE=$(curl -sf -X POST "https://api.anthropic.com/v1/messages" \
        -H "x-api-key: ${ANTHROPIC_API_KEY}" \
        -H "anthropic-version: 2023-06-01" \
        -H "Content-Type: application/json" \
        -d "$(jq -n \
            --arg prompt "${JUDGE_PROMPT}" \
            '{
                "model": "claude-sonnet-4-20250514",
                "max_tokens": 256,
                "messages": [{"role": "user", "content": $prompt}]
            }')" 2>&1) || {
        echo "  FAIL: Judge request failed"
        FAIL=$((FAIL + 1))
        continue
    }

    VERDICT=$(echo "${JUDGE_RESPONSE}" | jq -r '.content[0].text' | head -1)

    if [[ "${VERDICT}" == "PASS" ]]; then
        echo "  PASS"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: ${VERDICT}"
        FAIL=$((FAIL + 1))
    fi
done

echo "---"
echo "Results: ${PASS}/${TOTAL} passed, ${FAIL}/${TOTAL} failed"
RATE=$(awk "BEGIN { printf \"%.1f\", (${PASS}/${TOTAL})*100 }")
echo "Pass rate: ${RATE}%"

if [[ ${FAIL} -gt 0 ]]; then
    exit 1
fi
