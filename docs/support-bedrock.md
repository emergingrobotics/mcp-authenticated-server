# Amazon Bedrock Support

This document describes what changes are needed to use Amazon Bedrock instead of the direct Anthropic API.

## Overview

Bedrock is relevant in two places in this project, both of which are external to the core MCP server:

1. **HyDE (query expansion)** — uses Claude via the Anthropic SDK
2. **Eval scripts (LLM judge)** — calls Claude via curl

The MCP server itself (auth, database, search pipeline, ingestion) does not call Bedrock and requires no changes.

## Current State

| Component | Current approach | Bedrock change needed |
|-----------|-----------------|----------------------|
| `internal/hyde/anthropic.go` | Anthropic SDK with API key auth | Detect Bedrock URL, use AWS SigV4 auth |
| `scripts/eval.sh` | Direct curl to `api.anthropic.com` | Use `aws` CLI or SigV4-signed requests |
| Embed client | OpenAI-compatible `/v1/embeddings` (llama-server) | None -- keep using llama-server |
| Auth (Cognito JWT) | JWKS validation | None -- unrelated to Bedrock |
| Database | PostgreSQL / MSSQL | None -- unrelated to Bedrock |

## HyDE Generator

### What works today

The HyDE generator (`internal/hyde/anthropic.go`) uses the Anthropic Go SDK with API key authentication:

```go
opts := []option.RequestOption{
    option.WithAPIKey(apiKey),
}
if baseURL != "" {
    opts = append(opts, option.WithBaseURL(baseURL))
}
client := anthropic.NewClient(opts...)
```

Config:

```toml
[hyde]
enabled = true
model = "claude-haiku-4-5-20251001"
base_url = ""  # empty = default Anthropic endpoint
```

Environment: `ANTHROPIC_API_KEY`

### What needs to change for Bedrock

Bedrock uses AWS SigV4 authentication instead of API keys. The Anthropic Go SDK supports Bedrock, but the client initialization is different.

**Config changes:**

```toml
[hyde]
enabled = true
model = "us.anthropic.claude-haiku-4-5-20251001-v1:0"  # Bedrock model ID
base_url = ""  # not used for Bedrock
```

A new config field would be needed to select the auth mode:

```toml
[hyde]
auth_mode = "bedrock"  # "api_key" (default) or "bedrock"
region = "us-east-1"   # AWS region for Bedrock endpoint
```

**Code changes in `internal/hyde/anthropic.go`:**

The `NewAnthropicGenerator` function needs to detect the auth mode and construct the client accordingly:

```go
// For Bedrock, use AWS credentials instead of API key
if authMode == "bedrock" {
    awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
        awsconfig.WithRegion(region),
    )
    if err != nil {
        return &NoopGenerator{}
    }
    client := anthropic.NewClient(
        option.WithAWSConfig(awsCfg),
    )
    // ...
}
```

This requires adding `github.com/aws/aws-sdk-go-v2/config` as a dependency.

**Environment:** Instead of `ANTHROPIC_API_KEY`, use standard AWS credentials:

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_SESSION_TOKEN` (optional, for assumed roles)
- `AWS_REGION` or `AWS_DEFAULT_REGION`

Or use IAM roles, EC2 instance profiles, ECS task roles, etc. -- the AWS SDK default credential chain handles all of these.

**Bedrock model IDs** use a different format than Anthropic API model IDs:

| Anthropic API | Bedrock |
|--------------|---------|
| `claude-haiku-4-5-20251001` | `us.anthropic.claude-haiku-4-5-20251001-v1:0` |
| `claude-sonnet-4-20250514` | `us.anthropic.claude-sonnet-4-20250514-v1:0` |

The model ID in config.toml must match the endpoint being used.

## Eval Scripts

### What works today

`scripts/eval.sh` calls the Anthropic API directly via curl:

```bash
curl -sf -X POST "https://api.anthropic.com/v1/messages" \
    -H "x-api-key: ${ANTHROPIC_API_KEY}" \
    -H "anthropic-version: 2023-06-01" \
    -H "Content-Type: application/json" \
    -d '...'
```

### What needs to change for Bedrock

Bedrock requires SigV4-signed requests. Two approaches:

**Option A: Use the AWS CLI `bedrock-runtime invoke-model`:**

```bash
aws bedrock-runtime invoke-model \
    --model-id us.anthropic.claude-haiku-4-5-20251001-v1:0 \
    --region us-east-1 \
    --content-type application/json \
    --accept application/json \
    --body '{"anthropic_version":"bedrock-2023-05-31","max_tokens":256,"messages":[...]}' \
    /dev/stdout
```

**Option B: Use `curl` with `aws-sigv4`:**

```bash
curl -sf -X POST \
    "https://bedrock-runtime.us-east-1.amazonaws.com/model/us.anthropic.claude-haiku-4-5-20251001-v1:0/invoke" \
    --aws-sigv4 "aws:amz:us-east-1:bedrock" \
    --user "${AWS_ACCESS_KEY_ID}:${AWS_SECRET_ACCESS_KEY}" \
    -H "Content-Type: application/json" \
    -d '{"anthropic_version":"bedrock-2023-05-31","max_tokens":256,"messages":[...]}'
```

The eval script would need a config flag (e.g., `EVAL_BACKEND=bedrock`) to switch between the two code paths.

## Embedding Server

**No change needed.** Bedrock offers embedding models (Amazon Titan, Cohere Embed), but they use a different API format — not the OpenAI-compatible `/v1/embeddings` endpoint that the MCP server expects.

Continue using llama-server (or any OpenAI-compatible embedding endpoint) for embeddings. If you want to use Bedrock embeddings, you would need to write a thin proxy that translates between the Bedrock embedding API and the OpenAI format, or add a Bedrock embedding adapter to `internal/embed/`.

## Implementation Priority

If you want to add Bedrock support, the recommended order is:

1. **HyDE generator** — highest value, relatively small code change in `internal/hyde/anthropic.go`. Add an `auth_mode` config field and a second constructor path using AWS credentials.

2. **Eval scripts** — medium value, straightforward script change. Add an `EVAL_BACKEND` env var to switch between direct API and Bedrock.

3. **Embedding adapter** — lowest priority. llama-server on bare metal with GPU will outperform Bedrock embeddings for most workloads, and avoids per-request Bedrock costs.

## IAM Permissions for Bedrock

The AWS credentials used for Bedrock need the following IAM permissions:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "bedrock:InvokeModel"
            ],
            "Resource": [
                "arn:aws:bedrock:us-east-1::foundation-model/us.anthropic.claude-*"
            ]
        }
    ]
}
```

You must also enable model access in the Bedrock console for the specific Claude models you want to use.
