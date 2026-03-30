# MCP Authenticated Server -- Design Document

## Overview

A production-ready, fork-friendly Go template for building authenticated MCP (Model Context Protocol) servers backed by a relational database. Provides authentication, database access, vector search, SQL querying, guardrails, health checks, configuration, container orchestration, and MCP protocol handling out of the box.

Fork authors add domain logic (new tools, tables, eval entries) without touching framework code.

## Architecture

```
MCP Clients (Claude, agents, etc.)
        |
        | HTTPS / JSON-RPC (POST /mcp)
        v
+-------------------------------------------------------+
|                 mcp-server (Go binary)                 |
|                                                        |
|  Auth Middleware (Cognito JWT) --> MCP Handler          |
|       |                              |                 |
|  Per-tool Authorizer          Tool Registry            |
|                          - search_documents (vector)   |
|                          - query_data (SQL)            |
|                          - ingest_documents            |
|                          - (fork-added tools)          |
|                                                        |
|  Guardrails        Search Pipeline      Embed Client   |
|  L1: topic gate    (KNN + FTS + RRF)    (/v1/embed)   |
|  L2: score gate                                        |
|                                                        |
|  HyDE Generator    SQL Store            Reranker       |
|  (query expand)    (read-only exec)     (optional)     |
|                                                        |
|  Database Abstraction Layer                             |
|  (PostgreSQL + pgvector  OR  MS SQL Server)            |
+-------------------------------------------------------+
```

## Package Dependency DAG

All imports flow downward. No circular imports.

```
cmd/server
  +-- internal/server
        +-- internal/tools
        |     +-- internal/search (pipeline, RRF)
        |     |     +-- internal/vectorstore
        |     |     +-- internal/guardrails
        |     |     +-- internal/hyde
        |     |     +-- internal/rerank
        |     |     +-- internal/embed
        |     +-- internal/querystore
        |     +-- internal/ingest
        |           +-- internal/embed
        |           +-- internal/vectorstore
        +-- internal/auth
        +-- internal/config
        +-- internal/database
        |     +-- internal/database/postgres
        |     +-- internal/database/mssql
        +-- internal/engine
  vecmath <-- leaf package (imported by guardrails, vectorstore, embed)
```

## Key Design Decisions

| Decision | Resolution |
|----------|-----------|
| Auth provider | AWS Cognito (JWKS + JWT, externally provisioned) |
| Database engines | PostgreSQL (with pgvector) and MS SQL Server |
| Vector on MSSQL | Not supported (permanent constraint) |
| Embed server | External process (e.g., llama-server); bare metal with GPU recommended for performance |
| Container engine | Podman preferred, Docker supported, auto-detected |
| Transport | HTTP/HTTPS only (Streamable HTTP MCP transport) |
| Config format | TOML file + env vars for secrets |
| Config reload | SIGHUP for runtime-tunable sections; structural changes require restart |
| Module structure | Single go.mod at repo root |
| Multi-tenancy | Single shared connection pool; fork authors add per-user filtering |
| Guardrails | Two-level: L1 topic gate (pre-DB), L2 score gate (post-retrieval) |
| Write prevention (MSSQL) | Read-only DB user (primary) + keyword blocking (defense-in-depth) |

## Search Pipeline Flow

```
1. Receive { query, limit }
2. Auth middleware (already applied)
3. Per-tool authorization check
4. HyDE expansion (optional) --> embedText + prefix selection
5. Embed via /v1/embeddings
6. L2-normalize embedding
7. Level 1 guardrail: topic relevance gate (pre-DB)
8. Parallel retrieval: vector KNN || full-text search
9. RRF merge: score(d) = sum(1/(k + rank_i(d)))
10. Reranking (optional): cross-encoder rescoring
11. Level 2 guardrail: minimum match score gate
12. Truncate to limit
13. Return results
```

## Data Model (PostgreSQL)

**documents** -- one row per ingested file
- id (BIGSERIAL PK), source_path (UNIQUE), title, content, content_hash (SHA-256 prefix), token_count, created_at

**chunks** -- one row per document chunk
- id (BIGSERIAL PK), document_id (FK), chunk_index, content, token_count, heading_context, chunk_type, embedding (vector(N)), content_fts (generated tsvector), created_at
- UNIQUE(document_id, chunk_index)

**build_metadata** -- key/value store for ingest run metadata

Indexes: IVFFlat on embedding (when >= 100 chunks), GIN on content_fts.

## Extension Points

| What | How |
|------|-----|
| Add MCP tools | `tools.Register()` in cmd/server/main.go |
| Add DB tables | New DDL in internal/database/{engine}/schema.go |
| Custom auth policy | Implement `auth.Authorizer` interface |
| Custom chunking | Implement `Chunker` interface |
| Custom embed server | Set `embed.host` to any OpenAI-compatible endpoint |
| Custom guardrails | Add Level 3+ checks in guardrails package |

## Security Invariants

- All SQL: parameterized queries only
- All exec: explicit `[]string` argv, no `sh -c`
- All secrets: env vars only, never config files or logs
- All file reads: validated against allowed directories, no symlink following
- JWT tokens: never logged
- Container: non-root user, pinned base images with SHA256 digest
- Embedding server: external process, not bundled in MCP server container
