# AWS Cognito Setup

The MCP server authenticates all requests using JWT tokens issued by an AWS Cognito User Pool. You must provision a Cognito User Pool and App Client before the server can start.

You can provision Cognito resources using the AWS Console, the AWS CLI, Terraform, CloudFormation, or any other method you prefer. The **[emergingrobotics/aws-cognito](https://github.com/emergingrobotics/aws-cognito)** CLI tool is offered as an optional convenience for quick setup -- it is not required and has no special integration with this project.

## Overview

The MCP server needs three values from Cognito:

| Config Field | Example | Source |
|-------------|---------|--------|
| `auth.region` | `us-east-1` | Your AWS region |
| `auth.user_pool_id` | `us-east-1_aBcDeFgH` | Created by Cognito provisioning |
| `auth.client_id` | `1a2b3c4d5e6f7g8h` | App Client ID from provisioning |

The server derives the JWKS URL and issuer automatically from these three values.

## Option 1: Automated provisioning (optional convenience)

The [emergingrobotics/aws-cognito](https://github.com/emergingrobotics/aws-cognito) CLI tool is a lightweight wrapper that automates User Pool creation, App Client configuration, and user management via CloudFormation. It is entirely optional -- you can skip this and use Option 2 or any other provisioning method instead.

### Install

```bash
go install github.com/emergingrobotics/aws-cognito@latest
```

### Configure

Create a Cognito configuration file (see [cognito/config.json.example](../cognito/config.json.example) for the template):

```json
{
  "user_pool_name": "mcp-server-pool",
  "region": "us-east-1",
  "password_policy": {
    "minimum_length": 12,
    "require_uppercase": true,
    "require_lowercase": true,
    "require_numbers": true,
    "require_symbols": false
  },
  "app_client": {
    "name": "mcp-server-client",
    "generate_secret": true,
    "auth_flows": ["ALLOW_USER_PASSWORD_AUTH", "ALLOW_REFRESH_TOKEN_AUTH"],
    "token_validity": {
      "access_token": "1h",
      "id_token": "1h",
      "refresh_token": "30d"
    }
  }
}
```

Short token lifetimes (1 hour for access tokens) are recommended to limit replay attack windows.

### Provision

```bash
aws-cognito -c    # Creates User Pool + App Client via CloudFormation
```

The tool writes the provisioned `UserPoolId`, `ClientId`, `ClientSecret`, `Domain`, and `Region` back to the config file.

### Create users

```bash
aws-cognito -u    # Syncs users from the config file
```

### Create groups

For per-tool authorization (e.g., `ingest_documents` requires the `admin` group), create groups in the Cognito console or via the AWS CLI:

```bash
aws cognito-idp create-group \
  --user-pool-id us-east-1_aBcDeFgH \
  --group-name admin

aws cognito-idp admin-add-user-to-group \
  --user-pool-id us-east-1_aBcDeFgH \
  --username your-user \
  --group-name admin
```

## Option 2: Manual provisioning (AWS Console)

1. Go to **AWS Console > Cognito > User Pools > Create User Pool**.
2. Configure sign-in: email or username.
3. Configure password policy (12+ characters recommended).
4. Create an **App Client**:
   - Enable `ALLOW_USER_PASSWORD_AUTH` for testing and `ALLOW_USER_SRP_AUTH` for production.
   - Enable `client_credentials` grant if you need machine-to-machine (M2M) access.
   - Set access token validity to 1 hour.
5. Note the **User Pool ID**, **App Client ID**, and **Region**.

## Transfer values to config.toml

Copy the three values into your `config.toml`:

```toml
[auth]
region = "us-east-1"
user_pool_id = "us-east-1_aBcDeFgH"
client_id = "1a2b3c4d5e6f7g8h"
token_use = "access"
allowed_groups = []

[auth.tool_groups]
ingest_documents = ["admin"]
```

- `token_use`: set to `"access"` for M2M / client_credentials flows, or `"id"` if your clients send ID tokens.
- `allowed_groups`: leave empty for no server-wide group restriction, or list groups that all users must belong to.
- `auth.tool_groups`: map tool names to required groups. `ingest_documents` defaults to requiring `admin` because `drop=true` is destructive.

## Obtaining tokens

### Machine-to-machine (client_credentials)

```bash
export COGNITO_CLIENT_ID="your-client-id"
export COGNITO_CLIENT_SECRET="your-client-secret"
export COGNITO_DOMAIN="your-domain-prefix"
export COGNITO_REGION="us-east-1"
./scripts/get-token.sh --flow client_credentials
```

### User login (for testing and scripts)

```bash
export COGNITO_CLIENT_ID="your-client-id"
export COGNITO_USERNAME="your-username"
export COGNITO_PASSWORD="your-password"
./scripts/get-token.sh --flow user_password
```

Credentials are read from environment variables only -- never passed as CLI arguments (visible in `ps` output and shell history).

### Using the token

```bash
TOKEN=$(./scripts/get-token.sh --flow user_password)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/mcp ...
```

## JWT claims used by the server

| Claim | Purpose |
|-------|---------|
| `iss` | Validated against derived issuer URL |
| `aud` / `client_id` | Validated against configured `client_id` (check depends on `token_use`) |
| `exp` | Must be in the future |
| `nbf` | Must be in the past (when present) |
| `token_use` | Must match configured value (`access` or `id`) |
| `sub` | Stored in request context for logging and per-user filtering |
| `email` | Stored in request context |
| `cognito:groups` | Checked against `allowed_groups` and per-tool `required_groups` |
| `scope` | Available in context for fork authors |

## Troubleshooting

**"JWKS fetch failed at startup"** -- The server cannot reach the Cognito JWKS endpoint. Check that `auth.region` and `auth.user_pool_id` are correct and that the server has outbound HTTPS access to `cognito-idp.{region}.amazonaws.com`.

**"invalid issuer"** -- The token was issued by a different User Pool than configured. Verify `auth.user_pool_id` matches the pool that issued the token.

**"invalid client_id" / "invalid audience"** -- The token's `client_id` or `aud` claim does not match `auth.client_id`. Verify the App Client ID. Also check that `auth.token_use` matches the token type (`access` tokens use `client_id`, `id` tokens use `aud`).

**"forbidden: user not in any allowed server group"** -- The token's `cognito:groups` claim does not include any group listed in `auth.allowed_groups`. Add the user to the required group in Cognito.

**"forbidden: user not authorized for tool"** -- The user is not in the group required by `auth.tool_groups` for that specific tool.
