#!/usr/bin/env bash
set -euo pipefail

# EVAL-01..07: RAG evaluation script.
# Reads an eval JSON file, searches via the MCP server, judges results via the Anthropic API.
#
# Eval file format (EVAL-02):
#   [{"prompt": "question", "label": "good|bad|off_topic", "notes": "explanation"}, ...]
#
# "good"      = question is answerable from the corpus.
# "bad"       = question contains fabricated details; a passing answer must refuse the false premise.
# "off_topic" = question is outside the corpus domain; not judged, search results are printed for review.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EVAL_FILE="${1:-}"
CONFIG="${SCRIPT_DIR}/../config.toml"
DEFAULT_PORT=$(grep -E '^\s*port\s*=' "${CONFIG}" 2>/dev/null | head -1 | sed 's/.*=\s*"\?\([^"]*\)"\?/\1/' || echo "9090")
SERVER_URL="${MCP_SERVER_URL:-http://localhost:${DEFAULT_PORT}}"
EVAL_MODEL="${EVAL_MODEL:-claude-sonnet-4-20250514}"
EVAL_LIMIT="${EVAL_LIMIT:-0}"

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

# Get auth token
TOKEN=$("${SCRIPT_DIR}/get-token.sh") || {
    echo "Failed to obtain auth token. Check COGNITO_ env vars." >&2
    exit 1
}

# Initialize MCP session (required handshake before tools/call)
INIT_REQUEST=$(jq -n '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
        "protocolVersion": "2025-06-18",
        "capabilities": {},
        "clientInfo": {"name": "eval-script", "version": "1.0"}
    }
}')

INIT_HEADERS=$(mktemp)
trap 'rm -f "${INIT_HEADERS}"' EXIT

curl -sf -X POST "${SERVER_URL}/mcp" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json, text/event-stream" \
    -D "${INIT_HEADERS}" \
    -d "${INIT_REQUEST}" > /dev/null 2>&1 || {
    echo "Failed to initialize MCP session" >&2
    exit 1
}

SESSION_ID=$(grep -i "^Mcp-Session-Id:" "${INIT_HEADERS}" | tr -d '\r\n' | sed 's/^[^:]*: *//')
rm -f "${INIT_HEADERS}"

if [[ -z "${SESSION_ID}" ]]; then
    echo "No Mcp-Session-Id in initialize response" >&2
    exit 1
fi

# Send initialized notification to complete handshake
curl -sf -X POST "${SERVER_URL}/mcp" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json, text/event-stream" \
    -H "Mcp-Session-Id: ${SESSION_ID}" \
    -H "Mcp-Protocol-Version: 2025-06-18" \
    -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' > /dev/null 2>&1

TOTAL=$(jq length "${EVAL_FILE}")
if [[ "${EVAL_LIMIT}" -gt 0 && "${EVAL_LIMIT}" -lt "${TOTAL}" ]]; then
    TOTAL="${EVAL_LIMIT}"
fi

PASS=0
FAIL=0
GOOD_PASS=0
GOOD_TOTAL=0
BAD_PASS=0
BAD_TOTAL=0
OFF_TOPIC_TOTAL=0
FAILED_INDICES=()

echo "Running ${TOTAL} evaluations from ${EVAL_FILE}..."
echo "Model: ${EVAL_MODEL}"
echo "Server: ${SERVER_URL}"
echo "---"

for i in $(seq 0 $((TOTAL - 1))); do
    PROMPT=$(jq -r ".[$i].prompt" "${EVAL_FILE}")
    LABEL=$(jq -r ".[$i].label" "${EVAL_FILE}")
    NOTES=$(jq -r ".[$i].notes // \"\"" "${EVAL_FILE}")

    echo -n "[$((i + 1))/${TOTAL}] (${LABEL}) ${PROMPT:0:80}"
    [[ ${#PROMPT} -gt 80 ]] && echo -n "..."
    echo ""

    # Search via MCP server (JSON-RPC tools/call)
    MCP_REQUEST=$(jq -n \
        --arg query "${PROMPT}" \
        '{
            "jsonrpc": "2.0",
            "id": 1,
            "method": "tools/call",
            "params": {
                "name": "search_documents",
                "arguments": {"query": $query, "limit": 5}
            }
        }')

    SEARCH_RESULT=$(curl -sf -X POST "${SERVER_URL}/mcp" \
        -H "Authorization: Bearer ${TOKEN}" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json, text/event-stream" \
        -H "Mcp-Session-Id: ${SESSION_ID}" \
        -H "Mcp-Protocol-Version: 2025-06-18" \
        -d "${MCP_REQUEST}" 2>&1) || {
        echo "  FAIL: Search request failed"
        FAIL=$((FAIL + 1))
        FAILED_INDICES+=("$i")
        [[ "${LABEL}" == "good" ]] && GOOD_TOTAL=$((GOOD_TOTAL + 1))
        [[ "${LABEL}" == "bad" ]] && BAD_TOTAL=$((BAD_TOTAL + 1))
        [[ "${LABEL}" == "off_topic" ]] && OFF_TOPIC_TOTAL=$((OFF_TOPIC_TOTAL + 1))
        continue
    }

    # Extract JSON from SSE response (data: lines) or use as-is if already JSON
    if echo "${SEARCH_RESULT}" | grep -q '^data: '; then
        SEARCH_JSON=$(echo "${SEARCH_RESULT}" | grep '^data: ' | sed 's/^data: //' | head -1)
    else
        SEARCH_JSON="${SEARCH_RESULT}"
    fi

    # Extract search result content for the judge
    CONTEXT=$(echo "${SEARCH_JSON}" | jq -r '
        .result.content[]?.text // empty' 2>/dev/null || echo "${SEARCH_JSON}")

    # off_topic: ask the LLM to answer the question using search results, print its reply
    if [[ "${LABEL}" == "off_topic" ]]; then
        OFF_TOPIC_TOTAL=$((OFF_TOPIC_TOTAL + 1))

        OT_RESPONSE=$(curl -sf -X POST "https://api.anthropic.com/v1/messages" \
            -H "x-api-key: ${ANTHROPIC_API_KEY}" \
            -H "anthropic-version: 2023-06-01" \
            -H "Content-Type: application/json" \
            -d "$(jq -n \
                --arg prompt "Question: ${PROMPT}

Search Results:
${CONTEXT}

Answer the question using only the search results above. If the search results do not contain relevant information, say you cannot answer this question." \
                --arg model "${EVAL_MODEL}" \
                '{
                    "model": $model,
                    "max_tokens": 256,
                    "system": "You are a RAG assistant. Answer questions using only the provided search results. If the results are not relevant to the question, say you cannot answer this question. Do not explain what the search results contain or what topics are available.",
                    "messages": [{"role": "user", "content": $prompt}]
                }')" 2>&1) || {
            echo "  REPLY: (LLM request failed)"
            continue
        }

        OT_TEXT=$(echo "${OT_RESPONSE}" | jq -r '.content[0].text' 2>/dev/null || echo "(no response)")
        echo "  REPLY: ${OT_TEXT}"
        echo ""
        continue
    fi

    # Build judge prompt based on label
    if [[ "${LABEL}" == "good" ]]; then
        GOOD_TOTAL=$((GOOD_TOTAL + 1))
        JUDGE_SYSTEM="You are a RAG assistant and evaluator. Given a question and search results from a document corpus, determine if the search results contain enough information to answer the question correctly."
        JUDGE_PROMPT="Question: ${PROMPT}

Notes (what a correct answer should cover): ${NOTES}

Search Results:
${CONTEXT}

Does the search result contain the information needed to answer the question?
Respond with a JSON object: {\"pass\": true/false, \"reason\": \"brief explanation\"}"
    else
        BAD_TOTAL=$((BAD_TOTAL + 1))
        JUDGE_SYSTEM="You are a RAG assistant and evaluator. The following question contains fabricated details that are NOT in the document corpus. A good search system should either return no relevant results or return results that contradict the false premise."
        JUDGE_PROMPT="Question (contains false premise): ${PROMPT}

Notes: ${NOTES}

Search Results:
${CONTEXT}

Did the search system correctly avoid supporting the false premise? (Either by returning no results, low-relevance results, or results that contradict the premise.)
Respond with a JSON object: {\"pass\": true/false, \"reason\": \"brief explanation\"}"
    fi

    JUDGE_RESPONSE=$(curl -sf -X POST "https://api.anthropic.com/v1/messages" \
        -H "x-api-key: ${ANTHROPIC_API_KEY}" \
        -H "anthropic-version: 2023-06-01" \
        -H "Content-Type: application/json" \
        -d "$(jq -n \
            --arg system "${JUDGE_SYSTEM}" \
            --arg prompt "${JUDGE_PROMPT}" \
            --arg model "${EVAL_MODEL}" \
            '{
                "model": $model,
                "max_tokens": 256,
                "system": $system,
                "messages": [{"role": "user", "content": $prompt}]
            }')" 2>&1) || {
        echo "  FAIL: Judge request failed"
        FAIL=$((FAIL + 1))
        FAILED_INDICES+=("$i")
        continue
    }

    JUDGE_TEXT=$(echo "${JUDGE_RESPONSE}" | jq -r '.content[0].text' 2>/dev/null || echo "")

    # Extract pass/fail from judge response (look for "pass": true/false in JSON)
    if echo "${JUDGE_TEXT}" | grep -qi '"pass"[[:space:]]*:[[:space:]]*true'; then
        echo "  PASS"
        PASS=$((PASS + 1))
        [[ "${LABEL}" == "good" ]] && GOOD_PASS=$((GOOD_PASS + 1))
        [[ "${LABEL}" == "bad" ]] && BAD_PASS=$((BAD_PASS + 1))
    else
        REASON=$(echo "${JUDGE_TEXT}" | jq -r '.reason // empty' 2>/dev/null || echo "${JUDGE_TEXT:0:100}")
        echo "  FAIL: ${REASON}"
        FAIL=$((FAIL + 1))
        FAILED_INDICES+=("$i")
    fi
done

echo "---"
JUDGED=$((TOTAL - OFF_TOPIC_TOTAL))
if [[ ${JUDGED} -gt 0 ]]; then
    RATE=$(awk "BEGIN { printf \"%.1f\", (${PASS}/${JUDGED})*100 }")
    echo "Results: ${PASS}/${JUDGED} passed, ${FAIL}/${JUDGED} failed (pass rate: ${RATE}%)"
else
    echo "Results: no judged evals (all off_topic)"
fi

if [[ ${GOOD_TOTAL} -gt 0 ]]; then
    GOOD_RATE=$(awk "BEGIN { printf \"%.1f\", (${GOOD_PASS}/${GOOD_TOTAL})*100 }")
    echo "  good label:      ${GOOD_PASS}/${GOOD_TOTAL} (${GOOD_RATE}%)"
fi
if [[ ${BAD_TOTAL} -gt 0 ]]; then
    BAD_RATE=$(awk "BEGIN { printf \"%.1f\", (${BAD_PASS}/${BAD_TOTAL})*100 }")
    echo "  bad label:       ${BAD_PASS}/${BAD_TOTAL} (${BAD_RATE}%)"
fi
if [[ ${OFF_TOPIC_TOTAL} -gt 0 ]]; then
    echo "  off_topic label: ${OFF_TOPIC_TOTAL} (not judged, replies printed above)"
fi

if [[ ${#FAILED_INDICES[@]} -gt 0 ]]; then
    echo ""
    echo "Failed indices: ${FAILED_INDICES[*]}"
fi

if [[ ${FAIL} -gt 0 ]]; then
    exit 1
fi
