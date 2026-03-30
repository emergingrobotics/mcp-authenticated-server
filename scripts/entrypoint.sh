#!/usr/bin/env bash
set -euo pipefail

LLAMA_PID=""

cleanup() {
    echo "Received shutdown signal, cleaning up..."
    if [[ -n "${LLAMA_PID}" ]] && kill -0 "${LLAMA_PID}" 2>/dev/null; then
        kill -TERM "${LLAMA_PID}"
        wait "${LLAMA_PID}" 2>/dev/null || true
    fi
    exit 0
}

trap cleanup SIGTERM SIGINT

# EMBED-03: If bundled mode, start llama-server in background on loopback
if [[ "${BUNDLED:-false}" == "true" ]]; then
    LLAMA_MODEL="${LLAMA_MODEL:-/app/models/model.gguf}"
    LLAMA_PORT="${LLAMA_PORT:-8081}"
    LLAMA_HOST="127.0.0.1"

    echo "Starting llama-server on ${LLAMA_HOST}:${LLAMA_PORT}..."
    /app/llama-server \
        --model "${LLAMA_MODEL}" \
        --host "${LLAMA_HOST}" \
        --port "${LLAMA_PORT}" &
    LLAMA_PID=$!

    # EMBED-06: Health check with exponential backoff up to 60s
    MAX_WAIT=60
    WAIT=1
    ELAPSED=0
    while [[ ${ELAPSED} -lt ${MAX_WAIT} ]]; do
        if curl -sf "http://${LLAMA_HOST}:${LLAMA_PORT}/health" >/dev/null 2>&1; then
            echo "llama-server is healthy."
            break
        fi
        echo "Waiting for llama-server... (${ELAPSED}s elapsed)"
        sleep "${WAIT}"
        ELAPSED=$((ELAPSED + WAIT))
        WAIT=$((WAIT * 2))
        if [[ ${WAIT} -gt $((MAX_WAIT - ELAPSED)) ]] && [[ ${ELAPSED} -lt ${MAX_WAIT} ]]; then
            WAIT=$((MAX_WAIT - ELAPSED))
        fi
    done

    if [[ ${ELAPSED} -ge ${MAX_WAIT} ]]; then
        echo "ERROR: llama-server failed to become healthy within ${MAX_WAIT}s"
        exit 1
    fi
fi

echo "Starting MCP server..."
exec /app/mcp-server "$@"
