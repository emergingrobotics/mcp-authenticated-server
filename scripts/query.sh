#!/usr/bin/env bash
set -euo pipefail

# Convenience wrapper: query the MCP server.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONFIG="${SCRIPT_DIR}/../config.toml"
DEFAULT_PORT=$(grep -E '^\s*port\s*=' "${CONFIG}" 2>/dev/null | head -1 | sed 's/.*=\s*"\?\([^"]*\)"\?/\1/' || echo "9090")
SERVER_URL="${SERVER_URL:-http://localhost:${DEFAULT_PORT}}"
QUERY="${1:?Usage: $0 <query>}"

TOKEN=$("${SCRIPT_DIR}/get-token.sh")

curl -sf -X POST "${SERVER_URL}/query" \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg q "${QUERY}" '{"query": $q}')" | jq .
