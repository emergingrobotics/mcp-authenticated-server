# Non-Functional Requirements Summary

Extracted from REQUIREMENTS.md for quick reference. Requirement IDs map back to the full specification.

## Configuration (CFG-01..05)

- TOML config file; secrets via env vars only (DATABASE_URL, ANTHROPIC_API_KEY)
- Validated at startup: score ranges, URL schemes (http/https), required strings, timeout maximums, tls_cert/tls_key both-or-neither, cross-field rules (corpus_topic requires embed.enabled, etc.)
- config.toml.example committed with documented fields; config.toml gitignored, mode 0600
- SIGHUP hot-reload:
  - **Reloadable**: search, guardrails, hyde (except base_url), query, log_level
  - **Not reloadable** (restart required): runtime, server.port, server.tls_*, database, auth, embed, reranker.host, hyde.base_url, ingest
  - Network-destination fields non-reloadable to prevent SSRF via config modification
  - Invalid config on reload: log error, keep previous config

## Security (SEC-01..16)

- All exec: explicit []string argv; sh -c prohibited
- Secrets never in logs at any level; user query content at debug only
- HTTP client timeouts: bounded on all outbound calls (JWKS, embed, reranker, HyDE)
- Response body limits: embed 4 MiB, reranker 4 MiB, JWKS 1 MiB, HyDE 1 MiB
- SQL: read-only execution (Postgres: READ ONLY tx; MSSQL: read-only DB user) + keyword blocking
- SSRF mitigation: URL scheme validation; private/reserved IP ranges blocked for non-localhost endpoints
- File ingestion: O_NOFOLLOW, real path verification under allowed base directory
- JWT tokens never logged; auth failure reasons logged without token value
- config.toml.example: no real credentials
- Generated files (env, config.toml): mode 0600
- Container images: non-root user
- Containerfile: no secrets in layers (use --secret mounts); pinned base image with SHA256 digest
- TLS: reverse proxy recommended; direct TLS via tls_cert/tls_key for simple deployments
- Security headers: X-Content-Type-Options: nosniff, Cache-Control: no-store (auth endpoints), X-Frame-Options: DENY
- get-token.sh: credentials from env vars or stdin, never CLI args
- All SQL in codebase: parameterized queries (stated in Forkability Contract)

## Observability (OBS-01..07)

- Structured JSON logging via log/slog
- Log level configurable and SIGHUP-reloadable (debug, info, warn, error)
- MCP tool calls logged at info: tool name, duration, success/error, user sub
- DB query execution time at debug
- Startup log: version, listen address, DB engine, auth issuer, vector status, container engine, guardrail status, HyDE status, reranker status
- SIGHUP reload events at info with changed sections
- Destructive operations (drop=true ingest) at warn with user sub

## Performance (PERF-01..05)

- Concurrent MCP request handling (thread-safe handler)
- Pooled DB connections (single shared pool)
- JWKS cached in memory (no per-request network calls)
- Vector KNN and full-text search in parallel goroutines
- Topic vector computed once at startup, reused for all requests

## Testability (TEST-01..07)

- All DB access through interfaces (mock-substitutable)
- Auth middleware injectable (bypassable in tests)
- Required unit tests: JWT validation (incl. nbf, singleflight), RRF merge, chunking (all types + heading context), SQL keyword blocking (comment stripping, semicolons), config validation (cross-field rules), guardrail scores, L2 normalization, embedding prefix selection, per-tool authorization
- Integration test scaffolding: real PostgreSQL (gated by TEST_DATABASE_URL)
- Makefile targets: test (unit), test-integration, test-coverage (HTML report)
- Container engine detection: injectable lookPath
- govulncheck Makefile target

## Build and Deployment (BUILD-01..05)

- Single static Go binary; Go 1.23+ (go.mod: go 1.23)
- Multi-stage Containerfile: Go compile + final debian:bookworm-slim (pinned SHA256, non-root). Embedding server is external.
- Dockerfile symlink -> Containerfile
- compose.yml: MCP server container (database and embedding server are external)
- Makefile targets: build, test, test-integration, test-coverage, lint, govulncheck, run, container-build, container-up, container-down, container-logs, ingest, schema, validate, eval, eval-stability, download-model, prereqs
- ENGINE variable for container engine override

## Error Handling (ERR-01..20)

| Context | Behavior |
|---------|----------|
| Config validation failure | Exit 1 |
| DB unreachable at startup | Exit 1 (no retry; orchestrator handles restarts) |
| JWKS fetch failure at startup | Exit 1 |
| JWKS refetch failure (runtime) | Warn, continue with cached keys |
| Embed server unreachable | MCP tool error (no crash) |
| HyDE generation failure | Warn, fall back to raw query |
| Reranker failure | Warn, fall back to RRF ordering |
| Bundled llama-server fails to start | Exit 1 |
| L1 guardrail rejection | MCP result: "off_topic" (info log) |
| L2 guardrail rejection | MCP result: "below_threshold" (info log; score at debug) |
| SQL timeout | MCP error: "query timeout" |
| SQL syntax error | Return DB syntax error (safe) |
| SQL other error | Generic "query execution failed" to client; details server-side |
| SQL blocked keyword | MCP error: "query contains disallowed keywords" |
| Ingestion per-file error | Warn, skip, continue |
| Ingestion embed unreachable | Halt ingest, return partial results |
| Ingestion dir not in allowed_dirs | Error immediately |
| Unexpected panic | Recover in handler, log stack trace, return 500 |
| SIGHUP invalid config | Log error, keep previous config |
| SIGTERM/SIGINT | Graceful shutdown (configurable timeout) |

## Dependencies

### Go Modules
| Module | Purpose |
|--------|---------|
| modelcontextprotocol/go-sdk | MCP server implementation |
| jackc/pgx/v5 | PostgreSQL driver |
| pgvector/pgvector-go | pgvector type codec |
| microsoft/go-mssqldb | MS SQL Server driver |
| lestrrat-go/jwx/v2 | JWT/JWKS handling |
| BurntSushi/toml | TOML config parsing |
| anthropics/anthropic-sdk-go | Claude API for HyDE |

### External Infrastructure
| Component | When Required |
|-----------|--------------|
| PostgreSQL 14+ with pgvector | Vector mode |
| MS SQL Server 2019+ | SQL-only mode (alt to Postgres) |
| AWS Cognito User Pool | Always |
| Embedding server | Vector mode (bundled or external) |
| Anthropic API | HyDE or evals only |
| Podman or Docker | Containerized deployment |
