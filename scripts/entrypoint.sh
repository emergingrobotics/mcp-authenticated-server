#!/usr/bin/env bash
set -euo pipefail

# Container entrypoint for the MCP server.
# The embedding server is external and must be started separately.

cleanup() {
    echo "Received shutdown signal"
    exit 0
}

trap cleanup SIGTERM SIGINT

echo "Starting MCP server..."
exec /app/mcp-server "$@"
