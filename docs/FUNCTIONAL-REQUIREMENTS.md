# Functional Requirements Summary

Extracted from REQUIREMENTS.md for quick reference. Requirement IDs map back to the full specification.

## MCP Protocol (MCP-01..07)

- Streamable HTTP transport (`POST /mcp`)
- `tools/list` and `tools/call` support
- JSON-RPC 2.0 compliant responses
- Health endpoint `GET /healthz` (no auth, no internal details)
- Use `modelcontextprotocol/go-sdk` (v1.4.x)
- Dynamic tool registration (no framework edits to add tools)
- Graceful shutdown on SIGTERM/SIGINT (configurable timeout, default 15s)

## Authentication -- AWS Cognito (AUTH-01..11)

- JWT Bearer token required on all `/mcp` endpoints
- Validate against Cognito JWKS (derived from region + user_pool_id)
- Full JWT validation: signature, iss, aud/client_id (per token_use), exp, token_use, nbf
- JWKS caching with singleflight refetch on unknown kid (rate-limited to 1/60s)
- JWKS fetch timeout: 10s; retain cache on refetch failure
- Parsed token claims stored in request context (sub, email, groups, scope)
- Config: region, user_pool_id, client_id (server derives JWKS URL + issuer)
- Optional server-wide `allowed_groups` (403 if token lacks listed group)
- Per-tool `Authorizer` interface with group-based built-in implementation
- `ingest_documents` requires explicit group authorization by default
- Log auth failures at warn (reason only, never the token)

## Database -- Dual Engine (DB-01..09)

- PostgreSQL 14+ and MS SQL Server 2019+ (config-selected)
- Interface-abstracted access (tool handlers never import driver packages)
- Drivers: pgx/v5 (Postgres), go-mssqldb (MSSQL)
- Idempotent schema at startup (CREATE TABLE IF NOT EXISTS)
- DATABASE_URL via env var only; never in logs
- Connection pool: max_open=10, max_idle=5, lifetime=5m (configurable)
- Health check uses dedicated connection (not from pool)
- Store interface exposes underlying pool for vectorstore/querystore
- MSSQL: read-only DB user (SELECT + EXECUTE only)

## Vector/Semantic Search -- PostgreSQL Only (VEC-01..10)

- Available only with PostgreSQL + pgvector + embed.enabled=true
- MSSQL: vector tools not registered, vector config sections ignored silently
- embed.enabled=false: vector tools not registered regardless of DB engine
- pgvector-go library, vector(N) column (default N=768)
- Embedding client: OpenAI-compatible /v1/embeddings endpoint
- L2-normalize all embeddings before storage and query
- Parallel retrieval: vector KNN (cosine distance) + full-text search (tsvector/tsquery)
- RRF merge with configurable k-constant (default 60)
- Configurable retrieval pool size (default 20) and IVFFlat probes (default 4)

## Document Ingestion (ING-01..15)

- MCP tool (`ingest_documents`) and CLI subcommand (`ingest`)
- Recursive directory walk; validated against `ingest.allowed_dirs` whitelist
- Configurable file extensions, excluded directories, max file size (1 MiB)
- `.ragignore` support (gitignore-style patterns, no regex)
- Structure-aware chunking (see CHUNK-01..10)
- Configurable chunk size (default 256 tokens); min chunk: 50 chars
- Batched embedding (default 32 chunks/batch)
- Idempotent: unchanged files (SHA-256 hash) are no-ops
- `--drop` flag: drop + recreate tables, rebuild IVFFlat index if >= 100 chunks
- Per-file errors logged and skipped; fatal errors halt immediately
- Build metadata written after ingest
- No symlink following (O_NOFOLLOW + real path verification)

## Structure-Aware Chunking (CHUNK-01..10)

- Heading stack (levels 1-6) with breadcrumb construction ("A > B > C")
- Code blocks: atomic chunks (chunk_type=code)
- Tables: atomic chunks (chunk_type=table)
- Lists: chunk_type=list
- Paragraphs: accumulated to token budget (chunk_type=paragraph)
- YAML front matter stripped
- Title: first H1 or filename stem
- Embed text: instruction_prefix + filename[: heading_context] + "\n\n" + content
- Chunker interface for fork customization

## Two-Level Guardrails (GUARD-01..08)

- **Level 1 (Topic Relevance)**: embed corpus_topic at startup; cosine similarity gate before DB queries
- **Level 2 (Match Score)**: minimum score gate after retrieval + reranking
- Both independently configurable; zero overhead when disabled
- Scores validated [0.0, 1.0] at config load
- corpus_topic requires embed.enabled=true

## Search Enhancements (ENH-01..11)

- **HyDE**: optional query expansion via Claude API; configurable model/prompt
  - Falls back to raw query on failure (never fails the search)
  - Requires ANTHROPIC_API_KEY; logs warning if missing
- **Reranker**: optional cross-encoder via HTTP /rerank endpoint
  - Falls back to RRF ordering on failure
  - Timeout: 30s; response limit: 4 MiB
- **Embedding prefixes**: configurable query_prefix and passage_prefix

## SQL Query Mode (SQL-01..07)

- `query_data` tool: read-only SQL against both PostgreSQL and MSSQL
- Parameters: query (required), params (optional), limit (default 100, max 1000)
- Read-only transactions (Postgres) + read-only DB user (MSSQL)
- Query timeout: configurable (default 30s, max 5m)
- Response capped at 10 MiB; truncation flag when exceeded
- Blocked keywords: DDL, DML mutations, admin commands, transaction control, session modification, SELECT INTO
- Comment stripping before keyword scan; single statement only (semicolon rejection)
- Error sanitization: generic message to client, details logged server-side

## Built-in MCP Tools

| Tool | Availability | Key Parameters |
|------|-------------|----------------|
| `search_documents` | PostgreSQL + embed.enabled | query, limit (1-20, default 5) |
| `query_data` | Both engines | query, params, limit (default 100) |
| `ingest_documents` | PostgreSQL + embed.enabled | directory, drop (default false) |

## CLI Interface (CLI-01..06)

- `serve` (default): start HTTP server
- `ingest`: one-shot ingestion (not subject to per-tool auth, but subject to allowed_dirs)
- `validate`: config validation (exit 0/1)
- `schema`: apply DB schema and exit
- All accept `--config PATH` (default config.toml)
- `ingest` accepts: `--dir` (repeatable), `--drop`, `--dry-run`, `--verbose`

## Container Engine (ENG-01..10)

- Podman preferred, Docker supported, auto-detected
- Priority: CLI flag > config > PATH detection (podman first)
- Explicit []string argv (no sh -c)
- Engine-aware host gateway handling
- Engine abstraction methods: ComposeCmd, ProjectCmd, RunCmd, BuildCmd, etc.
- Compose files: compose.yml (engine-neutral)
- Injectable lookPath for testing

## External Embedding Server (EMBED-01..06)

- Embedding server runs as a separate external process (bare metal with GPU recommended)
- MCP server does NOT bundle or manage the embedding server
- `embed.host` points to any OpenAI-compatible `/v1/embeddings` endpoint
- Server validates connectivity at startup; continues if unreachable (tools return errors at runtime)
- download-model.sh helper for GGUF files
- README documents llama-server setup, model download, GPU configuration

## Evaluation Framework (EVAL-01..09)

- eval.sh: LLM-as-judge RAG quality measurement
- Eval format: JSON array with prompt, label (good/bad), notes
- Pipeline: optional HyDE expand -> search_documents -> LLM judge -> structured verdict
- Summary: total pass rate, per-label pass rate, failed indices
- eval-stability.sh: run N times (default 25), report min/max/avg
- Authenticates via get-token.sh
- Example evals committed as evals.json.example
