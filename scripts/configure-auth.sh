#!/usr/bin/env bash
set -euo pipefail

# Read Cognito values from an aws-cognito JSON config file and write them
# into the [auth] section of a config.toml file.
#
# Usage:
#   ./scripts/configure-auth.sh <cognito-json> <config-toml>
#
# Example:
#   ./scripts/configure-auth.sh cognito/config.json config.toml
#
# The cognito JSON file must contain: user_pool_id, client_id, region.
# The config.toml must already exist (copy from config.toml.example first).

COGNITO_JSON="${1:-}"
CONFIG_TOML="${2:-}"

if [[ -z "${COGNITO_JSON}" || -z "${CONFIG_TOML}" ]]; then
    echo "Usage: $0 <cognito-json> <config-toml>" >&2
    echo "" >&2
    echo "Reads auth values from a Cognito JSON file and writes them" >&2
    echo "into the [auth] section of a config.toml file." >&2
    echo "" >&2
    echo "Example:" >&2
    echo "  $0 cognito/config.json config.toml" >&2
    exit 1
fi

if [[ ! -f "${COGNITO_JSON}" ]]; then
    echo "Error: Cognito JSON file not found: ${COGNITO_JSON}" >&2
    exit 1
fi

if [[ ! -f "${CONFIG_TOML}" ]]; then
    echo "Error: config.toml not found: ${CONFIG_TOML}" >&2
    echo "Create it first: cp config.toml.example config.toml && chmod 600 config.toml" >&2
    exit 1
fi

# Extract values from JSON. Uses grep+sed to avoid requiring jq as a dependency.
# Strips trailing whitespace, carriage returns, and commas.
extract_json_string() {
    local key="$1"
    local file="$2"
    grep "\"${key}\"" "${file}" \
        | sed 's/.*"'"${key}"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' \
        | tr -d '\r\n '
}

REGION=$(extract_json_string "region" "${COGNITO_JSON}")
USER_POOL_ID=$(extract_json_string "user_pool_id" "${COGNITO_JSON}")
CLIENT_ID=$(extract_json_string "client_id" "${COGNITO_JSON}")

if [[ -z "${REGION}" ]]; then
    echo "Error: 'region' not found in ${COGNITO_JSON}" >&2
    exit 1
fi
if [[ -z "${USER_POOL_ID}" ]]; then
    echo "Error: 'user_pool_id' not found in ${COGNITO_JSON}" >&2
    exit 1
fi
if [[ -z "${CLIENT_ID}" ]]; then
    echo "Error: 'client_id' not found in ${COGNITO_JSON}" >&2
    exit 1
fi

echo "Read from ${COGNITO_JSON}:"
echo "  region:       ${REGION}"
echo "  user_pool_id: ${USER_POOL_ID}"
echo "  client_id:    ${CLIENT_ID}"

# Replace values in config.toml.
# Match lines like: region = "..." / user_pool_id = "..." / client_id = "..."
# Only replaces the first occurrence of each (inside [auth] section).
sed -i "s|^\(region = \)\"[^\"]*\"|\1\"${REGION}\"|" "${CONFIG_TOML}"
sed -i "s|^\(user_pool_id = \)\"[^\"]*\"|\1\"${USER_POOL_ID}\"|" "${CONFIG_TOML}"
sed -i "s|^\(client_id = \)\"[^\"]*\"|\1\"${CLIENT_ID}\"|" "${CONFIG_TOML}"

echo ""
echo "Updated ${CONFIG_TOML} [auth] section."
echo "Verify with: grep -A5 '\\[auth\\]' ${CONFIG_TOML}"
