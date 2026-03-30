#!/usr/bin/env bash
set -euo pipefail

# Convenience wrapper: query the MCP server.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SERVER_URL="${SERVER_URL:-http://localhost:8080}"
QUERY="${1:?Usage: $0 <query>}"

TOKEN=$("${SCRIPT_DIR}/get-token.sh")

curl -sf -X POST "${SERVER_URL}/query" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg q "${QUERY}" '{"query": $q}')" | jq .
