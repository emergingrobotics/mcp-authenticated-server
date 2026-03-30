#!/usr/bin/env bash
set -euo pipefail

# SEC-15: Obtain JWT from AWS Cognito.
# Secrets are read from environment variables only, never from CLI args.

FLOW="client_credentials"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --flow)
            FLOW="$2"
            shift 2
            ;;
        *)
            echo "Usage: $0 [--flow client_credentials|user_password]" >&2
            exit 1
            ;;
    esac
done

: "${COGNITO_CLIENT_ID:?Set COGNITO_CLIENT_ID}"
: "${COGNITO_CLIENT_SECRET:?Set COGNITO_CLIENT_SECRET}"
: "${COGNITO_DOMAIN:?Set COGNITO_DOMAIN}"
: "${COGNITO_REGION:?Set COGNITO_REGION}"

TOKEN_URL="https://${COGNITO_DOMAIN}.auth.${COGNITO_REGION}.amazoncognito.com/oauth2/token"

BASIC_AUTH=$(printf '%s:%s' "${COGNITO_CLIENT_ID}" "${COGNITO_CLIENT_SECRET}" | base64 -w0)

case "${FLOW}" in
    client_credentials)
        RESPONSE=$(curl -sf -X POST "${TOKEN_URL}" \
            -H "Content-Type: application/x-www-form-urlencoded" \
            -H "Authorization: Basic ${BASIC_AUTH}" \
            -d "grant_type=client_credentials" \
            -d "scope=${COGNITO_SCOPE:-openid}")
        ;;
    user_password)
        : "${COGNITO_USERNAME:?Set COGNITO_USERNAME}"
        : "${COGNITO_PASSWORD:?Set COGNITO_PASSWORD}"
        RESPONSE=$(curl -sf -X POST "${TOKEN_URL}" \
            -H "Content-Type: application/x-www-form-urlencoded" \
            -H "Authorization: Basic ${BASIC_AUTH}" \
            -d "grant_type=password" \
            -d "username=${COGNITO_USERNAME}" \
            -d "password=${COGNITO_PASSWORD}" \
            -d "scope=${COGNITO_SCOPE:-openid}")
        ;;
    *)
        echo "Unknown flow: ${FLOW}. Use client_credentials or user_password." >&2
        exit 1
        ;;
esac

ACCESS_TOKEN=$(echo "${RESPONSE}" | jq -r '.access_token')

if [[ -z "${ACCESS_TOKEN}" || "${ACCESS_TOKEN}" == "null" ]]; then
    echo "Failed to obtain access token." >&2
    echo "${RESPONSE}" >&2
    exit 1
fi

echo "${ACCESS_TOKEN}"
