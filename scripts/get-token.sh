#!/usr/bin/env bash
set -euo pipefail

# SEC-15: Obtain JWT from AWS Cognito.
# Secrets are read from environment variables only, never from CLI args.
#
# Flows:
#   user_password (default) — uses InitiateAuth API with USER_PASSWORD_AUTH.
#     Requires: COGNITO_CLIENT_ID, COGNITO_REGION, COGNITO_USERNAME, COGNITO_PASSWORD
#     Optional: COGNITO_USER_POOL_ID (for admin flows)
#
#   client_credentials — uses OAuth2 token endpoint.
#     Requires: COGNITO_CLIENT_ID, COGNITO_CLIENT_SECRET, COGNITO_DOMAIN, COGNITO_REGION
#     Note: requires a resource server with custom scopes in Cognito.
#
# Token type returned depends on the flow:
#   user_password: returns AccessToken (or IdToken with --id-token flag)
#   client_credentials: returns access_token

FLOW="user_password"
TOKEN_TYPE="access"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --flow)
            FLOW="$2"
            shift 2
            ;;
        --id-token)
            TOKEN_TYPE="id"
            shift
            ;;
        *)
            echo "Usage: $0 [--flow user_password|client_credentials] [--id-token]" >&2
            echo "" >&2
            echo "Flows:" >&2
            echo "  user_password (default)  — USER_PASSWORD_AUTH via InitiateAuth API" >&2
            echo "  client_credentials       — OAuth2 client_credentials grant" >&2
            echo "" >&2
            echo "Environment variables:" >&2
            echo "  user_password:      COGNITO_CLIENT_ID, COGNITO_REGION, COGNITO_USERNAME, COGNITO_PASSWORD" >&2
            echo "  client_credentials: COGNITO_CLIENT_ID, COGNITO_CLIENT_SECRET, COGNITO_DOMAIN, COGNITO_REGION" >&2
            exit 1
            ;;
    esac
done

case "${FLOW}" in
    user_password)
        : "${COGNITO_CLIENT_ID:?Set COGNITO_CLIENT_ID}"
        : "${COGNITO_REGION:?Set COGNITO_REGION}"
        : "${COGNITO_USERNAME:?Set COGNITO_USERNAME}"
        : "${COGNITO_PASSWORD:?Set COGNITO_PASSWORD}"

        RESPONSE=$(aws cognito-idp initiate-auth \
            --client-id "${COGNITO_CLIENT_ID}" \
            --auth-flow USER_PASSWORD_AUTH \
            --auth-parameters "USERNAME=${COGNITO_USERNAME},PASSWORD=${COGNITO_PASSWORD}" \
            --region "${COGNITO_REGION}" \
            --output json 2>&1) || {
            echo "InitiateAuth failed:" >&2
            echo "${RESPONSE}" >&2
            exit 1
        }

        if [[ "${TOKEN_TYPE}" == "id" ]]; then
            TOKEN=$(echo "${RESPONSE}" | jq -r '.AuthenticationResult.IdToken')
        else
            TOKEN=$(echo "${RESPONSE}" | jq -r '.AuthenticationResult.AccessToken')
        fi
        ;;

    client_credentials)
        : "${COGNITO_CLIENT_ID:?Set COGNITO_CLIENT_ID}"
        : "${COGNITO_CLIENT_SECRET:?Set COGNITO_CLIENT_SECRET}"
        : "${COGNITO_DOMAIN:?Set COGNITO_DOMAIN}"
        : "${COGNITO_REGION:?Set COGNITO_REGION}"

        TOKEN_URL="https://${COGNITO_DOMAIN}.auth.${COGNITO_REGION}.amazoncognito.com/oauth2/token"
        BASIC_AUTH=$(printf '%s:%s' "${COGNITO_CLIENT_ID}" "${COGNITO_CLIENT_SECRET}" | base64 -w0)

        RESPONSE=$(curl -sf -X POST "${TOKEN_URL}" \
            -H "Content-Type: application/x-www-form-urlencoded" \
            -H "Authorization: Basic ${BASIC_AUTH}" \
            -d "grant_type=client_credentials" \
            -d "scope=${COGNITO_SCOPE:-openid}" 2>&1) || {
            echo "Token request failed:" >&2
            echo "${RESPONSE}" >&2
            exit 1
        }

        TOKEN=$(echo "${RESPONSE}" | jq -r '.access_token')
        ;;

    *)
        echo "Unknown flow: ${FLOW}. Use user_password or client_credentials." >&2
        exit 1
        ;;
esac

if [[ -z "${TOKEN}" || "${TOKEN}" == "null" ]]; then
    echo "Failed to obtain token." >&2
    echo "Response: ${RESPONSE}" >&2
    exit 1
fi

echo "${TOKEN}"
