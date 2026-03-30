#!/usr/bin/env bash
set -euo pipefail

# Read Cognito values from an aws-cognito JSON config file and:
#   1. Write region, user_pool_id, client_id into the [auth] section of config.toml
#   2. Write COGNITO_CLIENT_ID, COGNITO_CLIENT_SECRET, COGNITO_DOMAIN, COGNITO_REGION
#      into .envrc (same directory as config.toml) for client-side token acquisition
#
# Usage:
#   ./scripts/configure-auth.sh <cognito-json> <config-toml>
#
# Example:
#   ./scripts/configure-auth.sh cognito/config.json config.toml
#
# The cognito JSON file must contain at minimum: user_pool_id, client_id, region.
# Optional fields: client_secret, domain (needed for get-token.sh).
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

# Extract a string value from JSON by key.
# Uses grep+sed, strips whitespace/CR. No jq dependency.
extract_json_string() {
    local key="$1"
    local file="$2"
    local value
    value=$(grep "\"${key}\"" "${file}" | head -1 | sed 's/.*"'"${key}"'"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' | tr -d '\r')
    echo -n "${value}"
}

REGION=$(extract_json_string "region" "${COGNITO_JSON}")
USER_POOL_ID=$(extract_json_string "user_pool_id" "${COGNITO_JSON}")
CLIENT_ID=$(extract_json_string "client_id" "${COGNITO_JSON}")
CLIENT_SECRET=$(extract_json_string "client_secret" "${COGNITO_JSON}")
DOMAIN=$(extract_json_string "domain" "${COGNITO_JSON}")

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
echo "  client_secret: ${CLIENT_SECRET:+(set)}"
echo "  domain:       ${DOMAIN:-(not set)}"

# --- Update config.toml (server-side: how to validate tokens) ---

# Match the full line: key = "any value" and replace with key = "new value".
# Using ; as sed delimiter to avoid conflicts with URL characters.
sed -i "s;^\(region = \)\".*\"\$;\1\"${REGION}\";" "${CONFIG_TOML}"
sed -i "s;^\(user_pool_id = \)\".*\"\$;\1\"${USER_POOL_ID}\";" "${CONFIG_TOML}"
sed -i "s;^\(client_id = \)\".*\"\$;\1\"${CLIENT_ID}\";" "${CONFIG_TOML}"

echo ""
echo "Updated ${CONFIG_TOML} [auth] section:"
grep -A6 '^\[auth\]' "${CONFIG_TOML}" | head -7

# --- Update .envrc (client-side: how to obtain tokens) ---

ENVRC_DIR="$(dirname "${CONFIG_TOML}")"
ENVRC="${ENVRC_DIR}/.envrc"

# Remove any existing COGNITO_ lines to avoid duplication
if [[ -f "${ENVRC}" ]]; then
    grep -v '^export COGNITO_' "${ENVRC}" > "${ENVRC}.tmp" || true
    mv "${ENVRC}.tmp" "${ENVRC}"
fi

{
    echo "export COGNITO_CLIENT_ID=\"${CLIENT_ID}\""
    echo "export COGNITO_REGION=\"${REGION}\""
    if [[ -n "${CLIENT_SECRET}" ]]; then
        echo "export COGNITO_CLIENT_SECRET=\"${CLIENT_SECRET}\""
    fi
    if [[ -n "${DOMAIN}" ]]; then
        echo "export COGNITO_DOMAIN=\"${DOMAIN}\""
    fi
} >> "${ENVRC}"
chmod 600 "${ENVRC}"

echo ""
echo "Updated ${ENVRC} with COGNITO_ environment variables."
echo "Run 'source ${ENVRC}' or use direnv to load them."
