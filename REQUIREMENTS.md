# MCP Authenticated Server — Requirements Document

## Related Documents

- **[docs/DESIGN.md](docs/DESIGN.md)** — Master design document: architecture diagrams, package dependency DAG, key design decisions, data model, extension points, and security invariants.
- **[docs/FUNCTIONAL-REQUIREMENTS.md](docs/FUNCTIONAL-REQUIREMENTS.md)** — Quick-reference summary of all functional requirements organized by subsystem (MCP protocol, authentication, database, vector search, ingestion, chunking, guardrails, SQL query, CLI, containers, eval framework).
- **[docs/NON-FUNCTIONAL-REQUIREMENTS.md](docs/NON-FUNCTIONAL-REQUIREMENTS.md)** — Quick-reference summary of all non-functional requirements: configuration, security, observability, performance, testability, build/deployment, error handling, and dependencies.

## 1. Project Purpose

**mcp-authenticated-server** is a production-ready, fork-friendly template for building authenticated MCP (Model Context Protocol) servers backed by a relational database. The goal is to provide **90%+ of the code** a team needs to stand up a domain-specific MCP server — authentication, database access, vector search, direct SQL querying, guardrails, health checks, configuration, container orchestration, and MCP protocol handling — so that forking the repo and adding domain logic is a matter of hours, not weeks.

Key differentiators:

1. **Database flexibility** — first-class support for both PostgreSQL (with pgvector) and Microsoft SQL Server.
2. **AWS Cognito authentication** — JWT validation against Cognito User Pools, with infrastructure provisioned via [emergingrobotics/aws-cognito](https://github.com/emergingrobotics/aws-cognito).
3. **Dual-mode data access** — a single server that can serve vector/semantic search (RAG) **and** structured SQL queries against the same database, selectable per-tool.
4. **Two-level guardrail system** — topic relevance gating and minimum match score filtering to control search quality and prevent hallucination-inducing off-topic results.
5. **Container-first deployment** — Podman-preferred, Docker-compatible container orchestration with external embedding server.
6. **Evaluation framework** — built-in scripts for measuring RAG retrieval quality with LLM-as-judge scoring.

---

## 2. Terminology

| Term | Definition |
|------|-----------|
| **MCP** | Model Context Protocol — JSON-RPC-based protocol for LLM tool use. |
| **Tool** | An MCP-callable function exposed by the server (e.g., `search_documents`, `query_data`). |
| **Cognito User Pool** | AWS-managed OIDC identity provider that issues JWTs. |
| **JWKS** | JSON Web Key Set — public keys used to verify JWT signatures. |
| **pgvector** | PostgreSQL extension for vector similarity search. |
| **RRF** | Reciprocal Rank Fusion — algorithm for merging ranked result lists. |
| **HyDE** | Hypothetical Document Embeddings — query expansion technique that generates a synthetic passage from the query, then embeds the passage instead of the raw question. |
| **Guardrail** | A configurable gate in the search pipeline that rejects off-topic queries or low-quality results before they reach the client. |
| **Heading Context** | Breadcrumb string (e.g., `"Installation > Linux > Docker Setup"`) capturing the hierarchical heading ancestry of a document chunk. |
| **Fork** | A downstream copy of this repo customized for a specific domain. |
| **Container Engine** | Podman or Docker — the runtime used to build and run OCI containers. |

---

## 3. Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                     MCP Clients                         │
│              (Claude, custom agents, etc.)               │
└──────────────────────┬──────────────────────────────────┘
                       │ HTTPS / JSON-RPC
                       ▼
┌─────────────────────────────────────────────────────────┐
│                   mcp-server (Go)                        │
│                                                          │
│  ┌──────────┐  ┌──────────────┐  ┌───────────────────┐  │
│  │ Auth     │  │ MCP Protocol │  │ Tool Registry      │  │
│  │ Middleware│──│ Handler      │──│                    │  │
│  │ (Cognito)│  │ (StreamHTTP) │  │ - search_documents │  │
│  └──────────┘  └──────────────┘  │ - query_data       │  │
│                                  │ - ingest_documents  │  │
│       ┌──────────────┐          │ - (fork-added tools)│  │
│       │ Per-tool     │          └─────────┬───────────┘  │
│       │ Authorizer   │                    │              │
│       └──────────────┘  ┌─────────────────┘              │
│                         │                                │
│  ┌────────────────┐     │  ┌──────────────┐  ┌────────┐ │
│  │ Guardrails     │     │  │ Vector Store │  │ Embed  │ │
│  │ L1: topic gate │     │  │ (search/RRF) │  │ Client │ │
│  │ L2: score gate │     │  └──────┬───────┘  └────────┘ │
│  └────────────────┘     │         │                      │
│  ┌────────────────┐     │  ┌──────┴──────┐  ┌─────────┐ │
│  │ HyDE Generator │     │  │ SQL Store   │  │Reranker │ │
│  │ (query expand) │     │  │ (query exec)│  │(option) │ │
│  └────────────────┘     │  └──────┬──────┘  └─────────┘ │
│                         │         │                      │
│  ┌──────────────────────┼─────────┘                      │
│  │         Database Abstraction                          │
│  │    (PostgreSQL  OR  MS SQL Server)                    │
│  └───────────────────────────────────────────────────────┘
└─────────────────────────────────────────────────────────┘
              │                          │
              ▼                          ▼
   ┌──────────────────┐      ┌──────────────────┐
   │   PostgreSQL     │  OR  │  MS SQL Server   │
   │   + pgvector     │      │  (no vectors)    │
   └──────────────────┘      └──────────────────┘
```

### Package Dependency DAG

All cross-package imports MUST flow downward. No upward or circular imports are permitted.

```
cmd/server
  └── internal/server
        ├── internal/tools
        │     ├── internal/search
        │     │     ├── internal/vectorstore
        │     │     ├── internal/guardrails
        │     │     ├── internal/hyde
        │     │     ├── internal/rerank
        │     │     └── internal/embed
        │     ├── internal/querystore
        │     └── internal/ingest
        │           ├── internal/embed
        │           └── internal/vectorstore
        ├── internal/auth
        ├── internal/config
        ├── internal/database
        │     ├── internal/database/postgres
        │     └── internal/database/mssql
        └── internal/engine
              └── (no internal imports)
  vecmath ← leaf package, imported by guardrails, vectorstore, embed
```

---

## 4. Functional Requirements

### 4.1 MCP Protocol Compliance

| ID | Requirement |
|----|------------|
| MCP-01 | The server MUST implement the MCP Streamable HTTP transport (`POST /mcp`). |
| MCP-02 | The server MUST support tool listing (`tools/list`) and tool invocation (`tools/call`). |
| MCP-03 | The server MUST return well-formed JSON-RPC 2.0 responses for all requests, including errors. |
| MCP-04 | The server MUST expose a health endpoint (`GET /healthz`) that does **not** require authentication. The health endpoint MUST return only a simple status (`{"status":"ok"}` or `{"status":"degraded"}`) — it MUST NOT include internal details such as database host, version, or connection counts. |
| MCP-05 | The server SHOULD use the `modelcontextprotocol/go-sdk` library (currently v1.4.x) as the MCP implementation. |
| MCP-06 | Tool registration MUST be dynamic — fork authors add tools by calling a registration function, not by editing framework code. |
| MCP-07 | The server MUST handle `SIGTERM` and `SIGINT` by initiating a graceful shutdown: stop accepting new connections, allow in-flight requests to complete (configurable timeout, default 15 seconds), close database connections, and exit 0. |

### 4.2 Authentication — AWS Cognito

| ID | Requirement |
|----|------------|
| AUTH-01 | All MCP endpoints (`POST /mcp`) MUST require a valid JWT Bearer token. Unauthenticated requests MUST receive a `401 Unauthorized` JSON response. |
| AUTH-02 | The server MUST validate JWTs against the AWS Cognito JWKS endpoint derived from the configured User Pool. The JWKS URL follows the pattern `https://cognito-idp.{region}.amazonaws.com/{userPoolId}/.well-known/jwks.json`. |
| AUTH-03 | JWT validation MUST verify: (a) cryptographic signature via JWKS public key, (b) `iss` claim matches the Cognito User Pool URL, (c) `aud` claim (for id tokens) or `client_id` claim (for access tokens) matches the configured App Client ID — the check used MUST correspond to the configured `token_use`, (d) `exp` claim is in the future, (e) `token_use` claim matches the configured value (`access` or `id`), (f) `nbf` claim is in the past when the claim is present. |
| AUTH-04 | The server MUST cache the JWKS key set in memory with automatic refresh on unknown `kid`. Refetch MUST use a singleflight pattern so that when multiple concurrent requests encounter an unknown `kid`, exactly one JWKS fetch occurs and all waiters share the result. Refetch MUST be rate-limited to at most one per 60 seconds. The `kid` MUST be extracted from the JWT JOSE header before triggering a refetch. |
| AUTH-05 | The JWKS HTTP fetch MUST have a bounded timeout (10 seconds). On refetch failure, the cached JWKS MUST be retained and the server continues serving with existing keys. |
| AUTH-06 | On successful validation, the parsed token and its claims MUST be stored in the request context so that downstream tool handlers can access user identity (e.g., `sub`, `email`, `cognito:groups`, `scope`). |
| AUTH-07 | Infrastructure provisioning of the Cognito User Pool, App Client, and domain is managed externally by the `emergingrobotics/aws-cognito` CLI tool. The server itself does NOT provision Cognito resources. |
| AUTH-08 | The server configuration MUST accept: `region`, `user_pool_id`, and `client_id`. From these three values the server derives the JWKS URL and issuer. |
| AUTH-09 | The server MUST support an optional `allowed_groups` configuration for server-wide access control. When set, only tokens whose `cognito:groups` claim includes at least one of the listed groups are authorized. All others receive `403 Forbidden`. |
| AUTH-10 | The server MUST log authentication failures at `warn` level with the reason (expired, bad signature, wrong audience, etc.) but MUST NOT log the token itself. |
| AUTH-11 | The server MUST define an `Authorizer` interface that is called per-tool to determine if the authenticated user is authorized for that specific tool. The built-in implementation checks against a per-tool `required_groups` config. Fork authors MAY implement custom `Authorizer` logic (e.g., attribute-based access control). The `ingest_documents` tool MUST require explicit group authorization by default (it is destructive when `drop=true`). |

### 4.3 Database — Dual Engine Support

| ID | Requirement |
|----|------------|
| DB-01 | The server MUST support **PostgreSQL** (14+) and **Microsoft SQL Server** (2019+) as database backends, selected by configuration. |
| DB-02 | Database access MUST be abstracted behind Go interfaces so that engine-specific implementations are isolated. Tool handlers and business logic MUST NOT import driver-specific packages directly. |
| DB-03 | For PostgreSQL, the driver MUST be `jackc/pgx/v5`. For MS SQL Server, the driver MUST be `microsoft/go-mssqldb`. |
| DB-04 | Schema creation MUST be idempotent (`CREATE TABLE IF NOT EXISTS` / equivalent). There is no migration framework — schema is applied at startup. |
| DB-05 | The database connection string MUST be supplied via the `DATABASE_URL` environment variable (never in config files). It MUST NOT appear in log output. |
| DB-06 | Connection pooling MUST be configured with sensible defaults (max open: 10, max idle: 5, lifetime: 5 min) and be overridable via configuration. All authenticated users share the same connection pool. |
| DB-07 | The health endpoint (`/healthz`) MUST verify database connectivity via a separate, dedicated connection (not from the main pool) to prevent pool exhaustion from blocking health checks. |
| DB-08 | The `database.Store` interface MUST expose a method returning the underlying connection pool (e.g., `Pool() *sql.DB` or engine-specific equivalent) for use by `vectorstore` and `querystore` implementations. |
| DB-09 | For **MS SQL Server**, the server MUST connect using a **read-only database user** that has only `SELECT` and `EXECUTE` permissions (no DML write permissions). This is the primary write-prevention control for MSSQL (keyword blocking in SQL-06 is defense-in-depth). The `cognito/config.json.example` and documentation MUST note this requirement. |

### 4.4 Vector / Semantic Search Mode (PostgreSQL only)

| ID | Requirement |
|----|------------|
| VEC-01 | Vector search MUST be available when the database engine is PostgreSQL with the `pgvector` extension enabled. |
| VEC-02 | When `database.engine = "mssql"`, vector tools (`search_documents`, `ingest_documents`) MUST NOT be registered at all — they are omitted from `tools/list`. The `[embed]`, `[search]`, `[reranker]`, `[guardrails]`, `[hyde]`, and `[ingest]` config sections MUST be ignored without error. Startup logs MUST note: `"vector features disabled: engine is mssql"`. |
| VEC-03 | When `embed.enabled = false` (regardless of database engine), vector tools (`search_documents`, `ingest_documents`) MUST NOT be registered. Only `query_data` is available. Startup logs MUST note: `"vector features disabled: embed.enabled is false"`. |
| VEC-04 | The vector store MUST use the `pgvector-go` library and store embeddings in a `vector(N)` column where N is configurable (default 768). |
| VEC-05 | The server MUST support an **embedding client** that calls an OpenAI-compatible `/v1/embeddings` endpoint. The `embed.model` config field MUST be sent as the `model` field in the API request body. The endpoint URL is configurable via `embed.host` (see section 4.9). |
| VEC-06 | Embeddings MUST be L2-normalized before storage and before query. Normalization MUST be performed in-place via the `vecmath` package. |
| VEC-07 | Search MUST support two parallel retrieval arms executed concurrently in goroutines: (a) vector KNN via pgvector `<=>` cosine distance operator, and (b) full-text search via PostgreSQL `tsvector`/`tsquery` using `plainto_tsquery('english', ...)`. |
| VEC-08 | Results from both arms MUST be merged using Reciprocal Rank Fusion (RRF) with a configurable k-constant (default 60). The RRF formula: `score(d) = Σ 1/(k + rank_i(d))` across all arms where `d` appears. |
| VEC-09 | The retrieval pool size (candidates per arm before RRF merge) MUST be configurable (default 20). |
| VEC-10 | IVFFlat index probes MUST be configurable (default 4). `SET ivfflat.probes = N` SHOULD only be issued when an IVFFlat index exists on the chunks table. When no index exists (< 100 chunks), PostgreSQL performs a sequential scan and the SET is unnecessary. |

#### 4.4.1 Document Schema

The vector store MUST maintain two core tables:

**`documents`**
| Column | Type | Constraints |
|--------|------|------------|
| `id` | `BIGSERIAL` | `PRIMARY KEY` |
| `source_path` | `TEXT` | `UNIQUE NOT NULL` |
| `title` | `TEXT` | |
| `content` | `TEXT` | `NOT NULL` |
| `content_hash` | `TEXT` | `NOT NULL` (SHA-256, first 16 hex) |
| `token_count` | `INTEGER` | |
| `created_at` | `TIMESTAMPTZ` | `DEFAULT NOW()` |

**`chunks`**
| Column | Type | Constraints |
|--------|------|------------|
| `id` | `BIGSERIAL` | `PRIMARY KEY` |
| `document_id` | `BIGINT` | `REFERENCES documents(id) ON DELETE CASCADE` |
| `chunk_index` | `INTEGER` | `NOT NULL` |
| `content` | `TEXT` | `NOT NULL` |
| `token_count` | `INTEGER` | |
| `heading_context` | `TEXT` | Breadcrumb of parent headings (e.g., `"Installation > Linux > Docker"`) |
| `chunk_type` | `TEXT` | `paragraph`, `code`, `table`, `list` |
| `embedding` | `vector(N)` | Configurable dimension |
| `content_fts` | `tsvector` | `GENERATED ALWAYS AS (to_tsvector('english', content)) STORED` |
| `created_at` | `TIMESTAMPTZ` | `DEFAULT NOW()` |
| | | `UNIQUE(document_id, chunk_index)` |

**`build_metadata`**
| Column | Type | Constraints |
|--------|------|------------|
| `key` | `TEXT` | `PRIMARY KEY` |
| `value` | `TEXT` | `NOT NULL` |

Indexes:
- `idx_chunks_embedding` — IVFFlat on `embedding` column (created after bulk ingest, with `lists = max(10, floor(sqrt(chunk_count)))`, skipped if chunk_count < 100)
- `idx_chunks_content_fts` — GIN on `content_fts` column

#### 4.4.2 Document Ingestion

| ID | Requirement |
|----|------------|
| ING-01 | The server MUST provide a built-in ingestion capability as both an MCP tool (`ingest_documents`) and a CLI subcommand (`ingest`). |
| ING-02 | Ingestion MUST accept a directory path and recursively walk it for eligible files. The directory MUST be validated against a configured whitelist of **allowed base directories** (`ingest.allowed_dirs`). If the requested directory is not under any allowed base path, the request MUST be rejected with an error. This prevents arbitrary filesystem reading via the MCP tool. |
| ING-03 | Eligible file extensions: `.txt`, `.md`, `.mdx`, `.rst`, `.go`, `.py`, `.js`, `.ts`, `.json`, `.yaml`, `.yml`, `.toml`, `.html`, `.css`, `.sh`, `.env.example`. This list MUST be configurable. |
| ING-04 | Files MUST be excluded if: hidden (except `.env.example`), in excluded directories (`node_modules`, `vendor`, `.git`, `__pycache__`), larger than 1 MiB (configurable), or matched by a `.ragignore` file. `.ragignore` uses gitignore-style glob patterns (one per line, `#` for comments, no negation support). It is read only from the root of the ingested directory and applies recursively. Pattern matching MUST use a library with bounded runtime (e.g., `filepath.Match` or `doublestar`) — no regex. |
| ING-05 | Chunking MUST be structure-aware (see section 4.4.3). |
| ING-06 | Chunk size MUST be configurable in approximate tokens (default 256). Token estimation: `len(text) / 4`. Minimum chunk length: 50 characters. |
| ING-07 | Each chunk MUST be embedded via the configured embedding endpoint, L2-normalized, and stored with its embedding. Embedding MUST be batched (configurable batch size, default 32 chunks per batch) to control memory usage during large ingestions. |
| ING-08 | Ingestion MUST be idempotent — re-ingesting a file with unchanged content (same SHA-256 hash, first 16 hex chars) MUST be a no-op. Changed files MUST have their chunks deleted and re-created. |
| ING-09 | Ingestion MUST support a `--drop` flag (or equivalent) that drops and recreates all tables before ingesting. |
| ING-10 | After a full `--drop` ingest, the IVFFlat index MUST be created if chunk count >= 100. |
| ING-11 | Per-file errors (read failure, embed failure, insert failure) MUST be logged and skipped. The ingest run MUST continue with remaining files. Fatal errors (schema failure, embed server unreachable) MUST halt immediately. |
| ING-12 | Build metadata MUST be written after ingest: embedding model, dimension, total chunks, total documents, duration, timestamp. |
| ING-13 | If the ingestion directory does not exist, MUST return an error. If the directory exists but contains no eligible files, MUST complete successfully with zero documents processed and log a warning. |
| ING-14 | If a file produces zero chunks after chunking (e.g., content is under 50 characters), the document row MUST still be created (for idempotency tracking) but no chunks are inserted. This SHOULD be logged at `warn` level. |
| ING-15 | File reads MUST use `O_NOFOLLOW` or equivalent to prevent symlink-based path traversal. After opening, the resolved real path MUST be verified to be under the allowed directory. |

#### 4.4.3 Structure-Aware Chunking

The chunking algorithm MUST maintain semantic structure:

| ID | Requirement |
|----|------------|
| CHUNK-01 | **Heading stack**: Maintain a stack of heading texts indexed by level (1-6). When a heading at level N is encountered: (a) update the stack at position N, (b) truncate all entries deeper than N, (c) flush any accumulated text as a chunk with the **previous** breadcrumb, (d) do NOT emit the heading itself as a chunk. |
| CHUNK-02 | **Breadcrumb construction**: Join non-empty heading stack entries with `" > "` delimiter. Example: H1 "Installation", H2 "Linux", H3 "Docker" → `"Installation > Linux > Docker"`. |
| CHUNK-03 | **Code blocks**: Fenced code blocks (` ``` `) MUST be emitted as atomic chunks with `chunk_type = "code"`. Content inside code fences is not interpreted for headings or other structure. |
| CHUNK-04 | **Tables**: Consecutive lines starting with `|` MUST be emitted as `chunk_type = "table"`. |
| CHUNK-05 | **Lists**: Lines starting with `-`, `*`, or `N.` MUST be classified as `chunk_type = "list"`. |
| CHUNK-06 | **Paragraphs**: All other text accumulates into `chunk_type = "paragraph"` chunks, flushed when the token budget is exceeded. |
| CHUNK-07 | **YAML front matter**: Optional YAML front matter (delimited by `---`) at the start of a file MUST be stripped before chunking. |
| CHUNK-08 | **Title extraction**: The document title is the first H1 heading, or the filename stem if no H1 exists. |
| CHUNK-09 | **Embed text construction**: When embedding a chunk for storage, the text MUST be constructed as: `[instruction_prefix] + filename[: heading_context] + "\n\n" + content`. This enriches the embedding with location context. |
| CHUNK-10 | A `Chunker` interface MUST be defined (e.g., `ChunkFile(path string, content []byte) ([]Chunk, error)`) that the built-in markdown chunker implements. Fork authors MAY replace it for non-markdown formats. |

#### 4.4.4 Two-Level Guardrail System

The guardrail system prevents off-topic queries from reaching the database and filters low-quality results before they reach the client.

| ID | Requirement |
|----|------------|
| GUARD-01 | **Level 1 — Topic Relevance Gate**: When `guardrails.corpus_topic` is configured (non-empty string), the server MUST: (a) at startup, embed the `corpus_topic` string and L2-normalize it, storing the resulting vector, (b) at search time, compute cosine similarity between the query embedding and the topic vector (since both vectors are L2-normalized per VEC-06, this equals their dot product), (c) if similarity < `guardrails.min_topic_score`, reject the query with an `"off_topic"` error: `"query does not appear to be related to the supported topic area"`. |
| GUARD-02 | Level 1 MUST be evaluated **before** any database queries are executed. This prevents unnecessary load from off-topic queries. |
| GUARD-03 | **Level 2 — Minimum Match Score Gate**: When `guardrails.min_match_score` > 0, after RRF merge and optional reranking, the server MUST check the highest-scoring result. If `best_score < min_match_score`, return an error: `"no content found that is sufficiently relevant to this query"`. The numeric score MUST NOT be included in the client-facing error; it MUST be logged server-side at `debug` level for diagnostics. |
| GUARD-04 | Level 2 MUST be evaluated **after** reranking (or after RRF if reranking is disabled), but **before** result limiting and return. |
| GUARD-05 | Both guardrail scores MUST be validated at config load time to be in the range `[0.0, 1.0]`. |
| GUARD-06 | When both guardrails are disabled (`corpus_topic` empty and `min_match_score` = 0), the search pipeline MUST skip all guardrail checks with zero overhead. |
| GUARD-07 | Startup logging MUST indicate guardrail status: `"Level 1 guardrail enabled: corpus_topic=..."` and/or `"Level 2 guardrail enabled: min_match_score=..."`. |
| GUARD-08 | **Config validation**: If `guardrails.corpus_topic` is non-empty, `embed.enabled` MUST be `true`. Otherwise, config validation MUST fail with error: `"guardrails.corpus_topic requires embed.enabled=true"`. |

#### 4.4.5 Optional Search Enhancements

| ID | Requirement |
|----|------------|
| ENH-01 | **HyDE (Hypothetical Document Embeddings)** — optionally expand the user query into a synthetic passage before embedding. |
| ENH-02 | HyDE MUST use an LLM call (Claude API by default) with a configurable system prompt. Default prompt: `"You are a documentation assistant. Write a 2-3 sentence answer to the following question as if it appeared verbatim in the documentation. Be specific and include exact values, constants, or commands if relevant. Do not hedge or qualify."` Max output tokens: 256. |
| ENH-03 | When HyDE succeeds and produces a hypothesis (regardless of similarity to the raw query), the hypothesis MUST be treated as a passage (using `passage_prefix` for embedding). When HyDE is disabled or the generation call fails, the raw query MUST use `query_prefix`. |
| ENH-04 | HyDE configuration: `hyde.enabled` (bool), `hyde.model` (string, default `"claude-haiku-4-5-20251001"`), `hyde.base_url` (optional override), `hyde.system_prompt` (optional override). Requires `ANTHROPIC_API_KEY` env var when enabled. |
| ENH-05 | If `hyde.enabled = true` but `ANTHROPIC_API_KEY` is not set, the server MUST log a warning at startup and fall back to a no-op generator (raw query passthrough). At runtime, if the Anthropic API returns errors (e.g., revoked key), the search MUST fall back to the raw query (per ERR-06) — it MUST NOT fail the search. |
| ENH-06 | **Cross-encoder reranking** — optionally rerank RRF results using a cross-encoder model via an HTTP `/rerank` endpoint. |
| ENH-07 | Reranker request format: `POST /rerank` with body `{"query": "...", "documents": [...], "top_n": N}`. Response: `{"results": [{"index": N, "relevance_score": F}, ...]}`. Scores are remapped to input order; missing indices remain 0.0. |
| ENH-08 | When the reranker returns scores, chunk scores MUST be replaced with reranker scores and results re-sorted descending. When the reranker returns nil/error, RRF scores are kept as fallback (per ERR-07). |
| ENH-09 | Reranker document text for scoring MUST include heading context: `filename[: heading_context] + "\n\n" + content`. |
| ENH-10 | Reranker HTTP client timeout: 30 seconds. Response body limit: 4 MiB. |
| ENH-11 | **Embedding instruction prefixes** — support configurable `query_prefix` and `passage_prefix` strings prepended to text before embedding (required by some models like Qwen3). |

### 4.5 SQL Query Mode (Both Engines)

| ID | Requirement |
|----|------------|
| SQL-01 | The server MUST expose a `query_data` MCP tool that executes read-only SQL queries against the database and returns results as structured JSON. |
| SQL-02 | The `query_data` tool MUST accept parameters: `query` (SQL string, required), `params` (array of bind parameters, optional — passed as-is to the database driver with driver-level type coercion), `limit` (max rows, default 100, max 1000). |
| SQL-03 | Queries MUST be executed in a read-only transaction where supported. For PostgreSQL: `SET TRANSACTION READ ONLY`. For MS SQL Server: use a read-only database user (DB-09) as the primary control, combined with application-level keyword blocking (SQL-06) as defense-in-depth. |
| SQL-04 | The tool MUST enforce a query timeout (configurable, default 30 seconds, maximum 5 minutes) to prevent long-running queries from consuming resources. Config validation MUST reject timeout values above 5 minutes. |
| SQL-05 | Results MUST be returned as `{ "columns": [...], "rows": [[...], ...], "row_count": N, "truncated": bool }`. Total response size MUST be capped at 10 MiB — if the result set exceeds this, rows MUST be truncated and `truncated` set to `true`. |
| SQL-06 | The tool MUST NOT allow: DDL (`CREATE`, `ALTER`, `DROP`), DML mutations (`INSERT`, `UPDATE`, `DELETE`, `TRUNCATE`, `MERGE`, `REPLACE`), administrative commands (`GRANT`, `REVOKE`, `EXEC`, `EXECUTE`, `xp_`, `sp_executesql`, `OPENROWSET`, `OPENDATASOURCE`, `BULK`, `COPY`, `LOAD`, `CALL`), transaction control (`BEGIN`, `COMMIT`, `ROLLBACK`, `SAVEPOINT`), session modification (`SET`, `DECLARE`), or `SELECT INTO`. SQL comments (`--`, `/* */`) MUST be stripped before keyword scanning. Only a single SQL statement is permitted — semicolons outside of string literals MUST be rejected. This is defense-in-depth; the primary write-prevention controls are read-only transactions (PostgreSQL) and read-only database users (MSSQL). |
| SQL-07 | SQL errors MUST be sanitized before returning to MCP clients. Return a generic `"query execution failed"` message to the client. Log the detailed database error server-side at `warn` level (without the full query text). Syntax errors MAY include the database's syntax error message since they do not leak schema details. |

### 4.6 Built-in MCP Tools

The template ships with three built-in tools. Fork authors may disable any of them and add their own.

#### 4.6.1 `search_documents` (Vector Search)

- **Availability**: PostgreSQL with `embed.enabled = true` only. Not registered otherwise.
- **Parameters**: `query` (string, required), `limit` (integer, 1-20, default 5).
- **Returns**: `{ "results": [ChunkResult], "total_chunks_in_db": int }` where `ChunkResult` contains `content`, `heading_context`, `chunk_type`, `source_path`, `title`, `score`. When the database has zero chunks, returns `{"results": [], "total_chunks_in_db": 0}` — this is not an error.
- **Pipeline**: See section 14 for the complete flow.

#### 4.6.2 `query_data` (SQL Query)

- **Availability**: Both PostgreSQL and MS SQL Server. For MSSQL with no fork-added domain tables, only system tables are queryable; this is expected and not an error.
- **Parameters**: `query` (string, required), `params` (array, optional), `limit` (integer, default 100).
- **Returns**: `{ "columns": [...], "rows": [[...], ...], "row_count": int, "truncated": bool }`.

#### 4.6.3 `ingest_documents` (Document Ingestion via MCP)

- **Availability**: PostgreSQL with `embed.enabled = true` only. Not registered otherwise.
- **Parameters**: `directory` (string, required — validated against `ingest.allowed_dirs`), `drop` (boolean, default false).
- **Returns**: `{ "documents_processed": int, "chunks_created": int, "errors": int, "duration_seconds": float }`.
- **Authorization**: This tool MUST require explicit group authorization by default (e.g., `admin` group) via the per-tool authorizer (AUTH-11), since `drop=true` is destructive.

### 4.7 CLI Interface

| ID | Requirement |
|----|------------|
| CLI-01 | The binary MUST support a `serve` subcommand (default) that starts the HTTP server. |
| CLI-02 | The binary MUST support an `ingest` subcommand that runs document ingestion as a one-shot process and exits. CLI ingestion is NOT subject to per-tool authorization (it runs locally, not via MCP). It IS subject to `ingest.allowed_dirs` validation. |
| CLI-03 | The binary MUST support a `validate` subcommand that validates the configuration file and exits with 0 (valid) or 1 (invalid). |
| CLI-04 | The binary MUST support a `schema` subcommand that applies the database schema and exits. Useful for CI or initialization scripts. |
| CLI-05 | All subcommands MUST accept `--config PATH` to specify the configuration file (default: `config.toml`). |
| CLI-06 | The `ingest` subcommand MUST accept: `--dir PATH` (repeatable), `--drop`, `--dry-run`, `--verbose`. |

### 4.8 Container Engine Support

| ID | Requirement |
|----|------------|
| ENG-01 | The project MUST support both **Podman** and **Docker** as container engines. **Podman is the default.** |
| ENG-02 | Engine selection MUST follow a priority chain: (1) `--engine` CLI flag (highest), (2) `runtime.engine` config field, (3) automatic PATH detection. |
| ENG-03 | Automatic detection MUST prefer Podman: try `podman` first in PATH, fall back to `docker` if not found. If neither is found, exit with a clear error. |
| ENG-04 | For Docker, the compose plugin (`docker compose`) MUST be preferred over the standalone `docker-compose` binary. |
| ENG-05 | All container commands MUST be constructed as explicit `[]string` argument slices — **never** via `sh -c` string interpolation. This prevents shell injection and ensures consistent behavior across engines. |
| ENG-06 | Host gateway handling MUST be engine-aware: Docker uses `--add-host host-gateway:host-gateway`, Podman uses `host.containers.internal` natively. |
| ENG-07 | The engine abstraction MUST provide methods for: `ComposeCmd`, `ProjectCmd` (with project name, env file, compose files), `RunCmd`, `BuildCmd`, `ImageExistsCmd`, `NetworkCreateCmd`, `NetworkExistsCmd`, `InspectHealthCmd`. |
| ENG-08 | Compose files MUST use `compose.yml` (not `docker-compose.yml`) to be engine-neutral. |
| ENG-09 | All Makefile targets that run containers MUST use the detected engine, not hardcode `docker` or `podman`. |
| ENG-10 | Engine detection MUST be tested with an injectable `lookPath` function to avoid requiring real container runtimes in CI. |

### 4.9 External Embedding Server

The embedding server (e.g., llama-server from llama.cpp, or any OpenAI-compatible endpoint) MUST be run as a **separate, external process**. Embedding inference requires bare-metal GPU access for acceptable performance; bundling it inside the MCP server container would negate GPU passthrough benefits and complicate deployment.

| ID | Requirement |
|----|------------|
| EMBED-01 | The MCP server MUST NOT bundle or manage an embedding server process. The embedding server is an external dependency, like PostgreSQL or Cognito. |
| EMBED-02 | The `embed.host` config field MUST point to a running OpenAI-compatible `/v1/embeddings` endpoint. The server validates connectivity at startup when `embed.enabled = true`. |
| EMBED-03 | If the embedding server is unreachable at startup and `embed.enabled = true`, the server MUST log an error and continue (vector tools will return errors at runtime). This allows the MCP server to start before the embedding server is ready. |
| EMBED-04 | The `compose.yml` for local development SHOULD include an example embedding service using a publicly available llama-server container image, or document how to start one separately. |
| EMBED-05 | A `scripts/download-model.sh` helper MUST be provided to download embedding model GGUF files via Hugging Face CLI for use with an external llama-server. |
| EMBED-06 | The documentation (README.md) MUST include clear instructions for setting up an external embedding server, including: recommended llama-server deployment, model download, GPU configuration, and the `embed.host` config setting. |

### 4.10 Evaluation Framework

| ID | Requirement |
|----|------------|
| EVAL-01 | The project MUST include an evaluation script (`scripts/eval.sh`) that measures RAG retrieval quality using an LLM-as-judge approach. |
| EVAL-02 | **Eval file format**: JSON array where each entry has: `prompt` (natural language question), `label` (`"good"` or `"bad"`), `notes` (explanation for the judge). `"good"` means the question is answerable from the corpus. `"bad"` means the question contains fabricated details not in the corpus — a passing answer must refuse the false premise. |
| EVAL-03 | **Eval pipeline**: For each eval entry: (a) optionally expand query via HyDE (`--expand-query` flag), (b) call `search_documents` via MCP to retrieve chunks, (c) build context from retrieved chunks including `heading_context`, (d) call an LLM judge (Anthropic API) with system prompt `"You are a RAG assistant and evaluator"`, (e) request structured JSON verdict: `{"answer": "...", "pass": true/false, "reason": "..."}`. |
| EVAL-04 | **Summary statistics**: The script MUST report: total pass rate, per-label pass rate (good vs. bad), and the indices of failed evals for debugging. |
| EVAL-05 | **Stability testing**: A `scripts/eval-stability.sh` script MUST run the eval suite N times (configurable, default 25) and report: min/max/average pass rates and most frequently failing eval indices with failure percentages. |
| EVAL-06 | The eval script MUST authenticate against the MCP server using the configured Cognito credentials, obtaining a JWT via `scripts/get-token.sh`. |
| EVAL-07 | Environment variables for evals: `ANTHROPIC_API_KEY` (required for judge), `MCP_SERVER_URL` (default `http://localhost:9090`), `EVAL_LIMIT` (max evals to run), `EVAL_MODEL` (judge model override). |
| EVAL-08 | A `data/evals/evals.json.example` MUST be committed with 3-5 example eval entries demonstrating both `"good"` and `"bad"` labels. |
| EVAL-09 | Makefile target `eval` MUST run the eval suite. Target `eval-stability` MUST run stability testing. |

---

## 5. Non-Functional Requirements

### 5.1 Configuration

| ID | Requirement |
|----|------------|
| CFG-01 | Configuration MUST be loaded from a TOML file. |
| CFG-02 | Secrets (`DATABASE_URL`, `ANTHROPIC_API_KEY`) MUST be supplied via environment variables only — never in the config file. |
| CFG-03 | The configuration file MUST be validated at startup. Validation rules include: score ranges [0,1], valid URL schemes (http/https only) for all outbound endpoints (`embed.host`, `reranker.host`, `hyde.base_url`), non-empty required strings, timeout maximums (query: 5m), `tls_cert`/`tls_key` both-or-neither, `guardrails.corpus_topic` requires `embed.enabled=true`, `ingest.allowed_dirs` must be non-empty if `ingest_documents` tool is registered. Missing required fields or invalid values MUST cause a clear error message and immediate exit. |
| CFG-04 | A `config.toml.example` MUST be committed to the repo with all fields documented via comments. The actual `config.toml` MUST be gitignored. `config.toml` MUST be written with file mode `0600`. |
| CFG-05 | The server MUST watch for `SIGHUP` and reload `config.toml` without restarting. On SIGHUP: (a) re-read and validate, (b) if validation fails, log error and keep previous config, (c) if valid, swap atomically. **Reloadable**: `[search]`, `[guardrails]`, `[hyde]` (except `base_url`), `[query]`, `[server].log_level`. **NOT reloadable** (require restart): `[runtime]`, `[server].port`, `[server].tls_*`, `[database]`, `[auth]`, `[embed]`, `[reranker].host`, `[hyde].base_url`, `[ingest]`. Network-destination fields (`host`, `base_url`) are non-reloadable to prevent SSRF via config modification. Log changed sections at `info` level on successful reload. |

**Configuration structure:**

```toml
[runtime]
engine = "podman"                  # "podman" or "docker" (auto-detect if empty)

[server]
port = "9090"                      # HTTP listen port
log_level = "info"                 # debug | info | warn | error
tls_cert = ""                      # optional: path to TLS certificate (both or neither)
tls_key = ""                       # optional: path to TLS private key (both or neither)

[database]
engine = "postgres"                # "postgres" or "mssql"
max_open_conns = 10
max_idle_conns = 5
conn_max_lifetime = "5m"

[auth]
region = "us-east-1"               # AWS region
user_pool_id = "us-east-1_aBcDeFgH" # Cognito User Pool ID
client_id = "1a2b3c4d5e6f7g8h"    # Cognito App Client ID
token_use = "access"               # "access" or "id"
allowed_groups = []                # empty = no server-wide group restriction

[auth.tool_groups]
ingest_documents = ["admin"]       # per-tool required groups (AUTH-11)
# search_documents = []            # empty = any authenticated user
# query_data = []                  # empty = any authenticated user

[embed]
enabled = true                     # false to disable vector features entirely
host = "http://localhost:8079"     # OpenAI-compatible embeddings API (external server)
model = "nomic-embed-text"         # sent in /v1/embeddings request body
dimension = 768                    # embedding vector dimension
query_prefix = ""                  # prefix prepended to queries before embedding
passage_prefix = ""                # prefix prepended to passages before embedding

[search]
probes = 4                         # IVFFlat probes per KNN query
retrieval_pool_size = 20           # candidates per search arm before RRF
rrf_constant = 60                  # RRF k-factor

[reranker]
enabled = false
host = "http://localhost:8081"     # cross-encoder /rerank endpoint

[guardrails]
corpus_topic = ""                  # empty = Level 1 disabled (requires embed.enabled)
min_topic_score = 0.25             # Level 1 threshold [0.0, 1.0]
min_match_score = 0.0              # Level 2 threshold [0.0, 1.0] (0 = disabled)

[hyde]
enabled = false
model = "claude-haiku-4-5-20251001"
base_url = ""                      # empty = default Anthropic endpoint
system_prompt = ""                 # empty = built-in default

[ingest]
chunk_size = 256                   # approximate tokens per chunk
batch_size = 32                    # chunks per embedding batch
max_file_size = "1MiB"            # skip files larger than this
allowed_dirs = ["/data"]           # whitelist of base directories for ingestion
allowed_extensions = [".txt", ".md", ".mdx", ".rst", ".go", ".py", ".js", ".ts", ".json", ".yaml", ".yml", ".toml", ".html", ".css", ".sh"]
excluded_dirs = ["node_modules", "vendor", ".git", "__pycache__"]

[query]
default_limit = 100                # default row limit for query_data
max_limit = 1000                   # hard cap on rows
max_response_size = "10MiB"       # max total response body for query results
timeout = "30s"                    # per-query timeout (max 5m)
```

### 5.2 Security

| ID | Requirement |
|----|------------|
| SEC-01 | All process exec calls MUST use explicit `[]string` argument slices — `sh -c` string interpolation is prohibited. |
| SEC-02 | ALL environment variables containing secrets (`DATABASE_URL`, `ANTHROPIC_API_KEY`, Cognito client secrets) MUST NOT appear in log output at any level. User query content SHOULD be logged at `debug` level only. |
| SEC-03 | HTTP clients (JWKS fetch, embed calls, reranker calls, HyDE calls) MUST have bounded timeouts. |
| SEC-04 | HTTP response bodies from external services MUST be size-limited: embed responses 4 MiB, reranker responses 4 MiB, JWKS responses 1 MiB, HyDE responses 1 MiB. |
| SEC-05 | SQL query tool MUST enforce read-only execution (PostgreSQL: `READ ONLY` transaction; MSSQL: read-only database user) with keyword blocking as defense-in-depth (see SQL-06). |
| SEC-06 | ALL outbound HTTP client endpoints (`embed.host`, `reranker.host`, `hyde.base_url`) MUST be validated for URL scheme (http/https only). Private/reserved IP ranges (RFC 1918, link-local 169.254.0.0/16, loopback 127.0.0.0/8) SHOULD be blocked for non-localhost endpoints to mitigate SSRF. Localhost is allowed for `embed.host` since the embedding server may run on the same host. |
| SEC-07 | File ingestion MUST NOT follow symlinks. Files MUST be opened with `O_NOFOLLOW` or equivalent. After path resolution, the real path MUST be verified to be under the allowed base directory. |
| SEC-08 | JWT tokens MUST NOT be logged. Authentication failure reasons MUST be logged (at warn level) without including the token value. |
| SEC-09 | The config.toml.example file MUST NOT contain real credentials. |
| SEC-10 | Generated `.env` files and `config.toml` MUST be written with mode `0600` (owner read/write only). |
| SEC-11 | Container images MUST run as a non-root user. |
| SEC-12 | The Containerfile MUST NOT include secrets in any layer. Build args for secrets MUST use `--secret` mounts, not `ARG`/`ENV`. The base image MUST be pinned to a specific version and SHA256 digest. The Containerfile builds only the Go server binary — the embedding server is external. |
| SEC-13 | TLS termination SHOULD be handled by a reverse proxy in production. Direct TLS is supported via `[server].tls_cert` and `[server].tls_key` for simpler deployments. |
| SEC-14 | The server MUST include standard security headers on all HTTP responses: `X-Content-Type-Options: nosniff`, `Cache-Control: no-store` (on authenticated endpoints), `X-Frame-Options: DENY`. |
| SEC-15 | `scripts/get-token.sh` MUST read credentials from environment variables or stdin — never from command-line arguments (which are visible in `ps` output and shell history). |
| SEC-16 | All SQL queries in the codebase (including fork-added code) MUST use parameterized queries. This MUST be stated as a security invariant in the Forkability Contract. |

### 5.3 Observability

| ID | Requirement |
|----|------------|
| OBS-01 | Structured logging (JSON format) via Go's `log/slog` standard library. |
| OBS-02 | Log level MUST be configurable (`debug`, `info`, `warn`, `error`) and SIGHUP-reloadable. |
| OBS-03 | Each MCP tool call MUST be logged at `info` level with: tool name, duration, success/error, authenticated user sub. |
| OBS-04 | Database query execution time MUST be logged at `debug` level. |
| OBS-05 | Startup MUST log: server version, listen address, database engine, auth issuer URL, vector features enabled/disabled, container engine (if applicable), guardrail status (L1/L2 enabled/disabled with thresholds), HyDE status, reranker status. |
| OBS-06 | SIGHUP reload events MUST be logged at `info` level with the list of changed config sections. |
| OBS-07 | Destructive operations (ingestion with `drop=true`) MUST be logged at `warn` level with the authenticated user's `sub` claim. |

### 5.4 Performance

| ID | Requirement |
|----|------------|
| PERF-01 | The server MUST handle concurrent MCP requests. The MCP handler MUST be safe for concurrent use. |
| PERF-02 | Database connections MUST be pooled. All users share a single pool (no per-tenant pools). |
| PERF-03 | JWKS keys MUST be cached in memory — no per-request network calls for key material under normal operation. |
| PERF-04 | Vector KNN and full-text search arms MUST execute in parallel (goroutines) and merge results after both complete. |
| PERF-05 | The Level 1 topic vector MUST be computed once at startup and reused for all requests. |

### 5.5 Testability

| ID | Requirement |
|----|------------|
| TEST-01 | All database access MUST go through interfaces. Test suites MUST be able to substitute mock implementations. |
| TEST-02 | Auth middleware MUST be injectable — tests MUST be able to bypass authentication with a test token or mock middleware. |
| TEST-03 | The project MUST include unit tests for: JWT validation logic (including `nbf`, singleflight), RRF merge, chunking algorithm (including heading context and all chunk types), SQL keyword blocking (including comment stripping and semicolon rejection), config validation (including cross-field rules like GUARD-08), guardrail score checks, L2 normalization, embedding prefix selection, per-tool authorization. |
| TEST-04 | The project MUST include integration test scaffolding that runs against a real PostgreSQL instance (gated by `TEST_DATABASE_URL` env var). |
| TEST-05 | Makefile targets: `test` (unit), `test-integration` (integration), `test-coverage` (HTML coverage report). |
| TEST-06 | Container engine detection MUST be tested with an injectable `lookPath` function. |
| TEST-07 | A `govulncheck` target MUST be included in the Makefile for dependency vulnerability scanning. |

### 5.6 Build and Deployment

| ID | Requirement |
|----|------------|
| BUILD-01 | The project MUST build as a single static Go binary. Go 1.23 or later. The `go.mod` directive MUST specify `go 1.23` as the minimum. |
| BUILD-02 | The project MUST include a multi-stage `Containerfile` (with a `Dockerfile` symlink for Docker compatibility) that: (a) compiles the Go binary, (b) produces a final image on a pinned `debian:bookworm-slim@sha256:...` base, running as non-root. The embedding server is external and NOT included in this image. |
| BUILD-03 | The project MUST include a `compose.yml` for running the MCP server container. The database and embedding server are external dependencies managed by the operator. |
| BUILD-04 | The `Makefile` MUST include targets: `build`, `test`, `test-integration`, `test-coverage`, `lint`, `govulncheck`, `run`, `container-build`, `container-up`, `container-down`, `container-logs`, `ingest`, `schema`, `validate`, `eval`, `eval-stability`, `download-model`, `prereqs`. |
| BUILD-05 | Makefile targets MUST auto-detect the container engine (podman preferred) and use it consistently. An `ENGINE` variable MUST allow override: `make container-up ENGINE=docker`. |

---

## 6. Project Structure

```
mcp-authenticated-server/
├── REQUIREMENTS.md                  # This document
├── CLAUDE.md                        # Claude Code project instructions
├── README.md                        # Setup and usage guide
├── Makefile
├── Containerfile                    # Multi-stage build (Go server only)
├── Dockerfile -> Containerfile      # Symlink for Docker compatibility
├── compose.yml                      # MCP server container (DB + embed server are external)
├── go.mod                           # Single module at root
├── go.sum
├── config.toml.example              # Documented example config (mode 0600)
├── .gitignore
│
├── models/                          # GGUF model files (gitignored, for external embed server)
│   └── .gitkeep
│
├── cmd/
│   └── server/
│       └── main.go                  # Entry point: parse flags, load config, wire deps
│
├── internal/
│   ├── config/
│   │   ├── config.go                # TOML loading, env var overlay, validation
│   │   ├── reload.go                # SIGHUP handler, atomic config swap
│   │   └── config_test.go
│   │
│   ├── auth/
│   │   ├── cognito.go               # JWKS cache w/ singleflight, JWT validation
│   │   ├── middleware.go            # HTTP middleware: extract Bearer, validate, ctx
│   │   ├── authorizer.go           # Authorizer interface + per-tool group impl
│   │   ├── context.go              # TokenFromContext, SubjectFromContext, GroupsFromContext
│   │   └── cognito_test.go
│   │
│   ├── database/
│   │   ├── interface.go             # Store interface (Connect, Ping, Close, ApplySchema, Pool)
│   │   ├── postgres/
│   │   │   ├── store.go             # PostgreSQL implementation (pgx)
│   │   │   ├── schema.go           # DDL for documents, chunks, build_metadata
│   │   │   └── store_test.go
│   │   └── mssql/
│   │       ├── store.go             # MS SQL Server implementation (go-mssqldb)
│   │       ├── schema.go           # DDL adapted for T-SQL
│   │       └── store_test.go
│   │
│   ├── vectorstore/
│   │   ├── interface.go             # VectorStore interface (InsertChunk, SearchKNN, SearchFTS)
│   │   ├── postgres.go              # pgvector-backed implementation
│   │   └── postgres_test.go
│   │
│   ├── querystore/
│   │   ├── interface.go             # QueryStore interface (ExecuteReadOnly)
│   │   ├── postgres.go              # PostgreSQL read-only query execution
│   │   ├── mssql.go                 # MS SQL Server read-only query execution
│   │   ├── safety.go               # SQL keyword blocking, comment stripping, validation
│   │   └── safety_test.go
│   │
│   ├── embed/
│   │   ├── interface.go             # Embedder interface
│   │   ├── client.go                # OpenAI-compatible /v1/embeddings HTTP client
│   │   └── client_test.go
│   │
│   ├── search/
│   │   ├── rrf.go                   # Reciprocal Rank Fusion merge
│   │   ├── pipeline.go             # Full search pipeline orchestration
│   │   └── rrf_test.go
│   │
│   ├── vecmath/
│   │   ├── vecmath.go               # L2 normalize, dot product, cosine similarity
│   │   └── vecmath_test.go
│   │
│   ├── ingest/
│   │   ├── walker.go                # Directory walking, file eligibility, ragignore
│   │   ├── chunker.go              # Chunker interface + markdown implementation
│   │   ├── pipeline.go             # Orchestrate: walk → read → chunk → embed → store
│   │   ├── chunker_test.go
│   │   └── walker_test.go
│   │
│   ├── guardrails/
│   │   ├── guardrails.go            # Level 1 + Level 2 check functions
│   │   ├── topic.go                # Topic vector init, cosine similarity check
│   │   └── guardrails_test.go
│   │
│   ├── hyde/
│   │   ├── interface.go             # Generator interface + NoopGenerator
│   │   ├── anthropic.go            # Claude-based HyDE implementation
│   │   └── noop.go
│   │
│   ├── rerank/
│   │   ├── interface.go             # Reranker interface
│   │   ├── client.go               # HTTP /rerank client
│   │   ├── noop.go                 # Disabled placeholder (returns nil)
│   │   └── client_test.go
│   │
│   ├── engine/
│   │   ├── engine.go                # Container engine detection + command builders
│   │   └── engine_test.go          # Tests with injectable lookPath
│   │
│   ├── server/
│   │   ├── server.go                # HTTP mux, MCP handler, health, graceful shutdown
│   │   └── server_test.go
│   │
│   └── tools/
│       ├── registry.go              # Tool registration helpers
│       ├── search.go                # search_documents (with guardrails + HyDE + rerank)
│       ├── query.go                 # query_data tool handler
│       ├── ingest.go                # ingest_documents tool handler
│       ├── search_test.go
│       └── query_test.go
│
├── scripts/
│   ├── entrypoint.sh                # Container entrypoint for the Go server
│   ├── get-token.sh                 # Obtain JWT from Cognito (reads creds from env/stdin)
│   ├── search.sh                    # Call search_documents via curl
│   ├── query.sh                     # Call query_data via curl
│   ├── eval.sh                      # RAG evaluation suite (LLM-as-judge)
│   ├── eval-stability.sh            # Run evals N times, report min/max/avg pass rates
│   ├── download-model.sh            # Download GGUF embedding model via Hugging Face CLI
│   └── install-postgres.sh          # Install PostgreSQL + pgvector extension
│
├── data/
│   └── evals/
│       └── evals.json.example       # Example eval entries (good + bad labels)
│
└── cognito/
    └── config.json.example          # Example emergingrobotics/aws-cognito config
```

**Note on module structure**: A single `go.mod` lives at the repository root. The `internal/` directory is directly under the root module, allowing `cmd/server/main.go` to import all `internal/` packages without cross-module restrictions.

---

## 7. Forkability Contract

The template is designed so that a fork requires **minimal changes** to become a domain-specific MCP server. A fork that adds one new tool and one new database table SHOULD require changes to no more than 3 files.

### 7.1 What a Fork Author Does

1. **Add domain tools** — create new files in `internal/tools/`, call the registration function from `cmd/server/main.go`.
2. **Extend the schema** — add domain-specific tables in a new file under `internal/database/{engine}/`, called from `ApplySchema()`.
3. **Configure** — edit `config.toml` with their Cognito pool, database DSN, embedding endpoint, guardrail settings, per-tool authorization.
4. **Optionally disable built-in tools** — set a config flag or remove registration calls for tools they don't need.
5. **Optionally add domain stores** — new interfaces + implementations under `internal/` following the same pattern.
6. **Write eval entries** — create `data/evals/evals.json` with domain-specific questions to validate retrieval quality.

### 7.2 What a Fork Author Does NOT Do

- Write authentication or authorization code.
- Write MCP protocol handling code.
- Write database connection management or pooling.
- Write embedding, search, chunking, or ingestion pipelines (unless customizing).
- Write guardrail logic.
- Write health checks, config loading, CLI parsing, logging setup.
- Write container engine detection or compose files.
- Write eval infrastructure.

### 7.3 Security Invariants for Fork Authors

Fork authors MUST adhere to these invariants:

- **All SQL queries MUST use parameterized queries** — never string interpolation.
- **All process exec calls MUST use `[]string` argument slices** — never `sh -c`.
- **Secrets MUST come from environment variables** — never config files.
- **File reads MUST validate paths** against allowed directories.

### 7.4 Extension Points

| Extension Point | Mechanism |
|----------------|-----------|
| Add MCP tools | Call `tools.Register(server, ...)` in `cmd/server/main.go` |
| Add database tables | Add DDL to a new schema file, call from `ApplySchema()` |
| Add new store interface | Create `internal/{domain}/interface.go` + implementations |
| Customize auth policy | Implement `auth.Authorizer` interface (per-tool checks) |
| Add CLI subcommands | Add a case in `cmd/server/main.go` command dispatch |
| Replace embedding provider | Implement `embed.Embedder` interface |
| Custom guardrails | Add Level 3+ checks in `guardrails/guardrails.go` |
| Custom chunking strategy | Implement the `Chunker` interface for non-markdown formats |

---

## 8. AWS Cognito Integration Details

### 8.1 Provisioning Workflow (Out of Band)

The Cognito infrastructure is provisioned separately using the `emergingrobotics/aws-cognito` CLI tool:

1. Create a `cognito-config.json` defining: User Pool name, password policy, App Client (OAuth flows: `client_credentials` for M2M, `authorization_code` for user-facing), token validity settings. **Recommendation: use short token lifetimes** (e.g., 1 hour for access tokens) to limit replay attack windows.
2. Run `aws-cognito -c` to provision via CloudFormation.
3. The tool writes outputs to the config file: `UserPoolId`, `ClientId`, `ClientSecret`, `Domain`, `Region`.
4. Transfer `UserPoolId`, `ClientId`, and `Region` to the MCP server's `config.toml` under `[auth]`.
5. Optionally run `aws-cognito -u` to sync users.

### 8.2 Token Acquisition (Client Workflows)

**Machine-to-machine (M2M):**
```bash
# Credentials via environment variables (not CLI args)
export COGNITO_CLIENT_ID="..."
export COGNITO_CLIENT_SECRET="..."
export COGNITO_DOMAIN="..."
export COGNITO_REGION="..."
./scripts/get-token.sh --flow client_credentials
```

**User login (for testing / scripts):**
```bash
export COGNITO_CLIENT_ID="..."
export COGNITO_USERNAME="..."
export COGNITO_PASSWORD="..."  # or read from stdin
./scripts/get-token.sh --flow user_password
```

### 8.3 JWT Claims Map

| Claim | Source | Validated By Server |
|-------|--------|-------------------|
| `iss` | `https://cognito-idp.{region}.amazonaws.com/{poolId}` | Yes — must match derived issuer |
| `aud` (id token) / `client_id` (access token) | App Client ID | Yes — check corresponds to `token_use` config |
| `token_use` | `access` or `id` | Yes — must match configured `token_use` |
| `exp` | Expiration timestamp | Yes — must be in future |
| `nbf` | Not-before timestamp | Yes — must be in past (when present) |
| `sub` | User UUID | Stored in context, not validated |
| `cognito:groups` | Group memberships | Checked against `allowed_groups` and per-tool `required_groups` |
| `email` | User email | Stored in context |
| `scope` | OAuth scopes (access tokens only) | Available in context for fork use |

---

## 9. MS SQL Server Considerations

| ID | Requirement |
|----|------------|
| MSSQL-01 | When `database.engine = "mssql"`, vector features MUST be completely unavailable per VEC-02. |
| MSSQL-02 | Schema DDL MUST use T-SQL equivalents: `BIGINT IDENTITY(1,1)` instead of `BIGSERIAL`, `NVARCHAR(MAX)` instead of `TEXT`, `DATETIMEOFFSET` instead of `TIMESTAMPTZ`. |
| MSSQL-03 | Write prevention MUST use a read-only database user (DB-09) as the primary control, combined with keyword blocking (SQL-06) as defense-in-depth. |
| MSSQL-04 | Connection strings for MSSQL follow the format: `sqlserver://user:password@host:port?database=dbname`. |
| MSSQL-05 | The MSSQL store MUST handle T-SQL-specific error codes and translate them to the same error interface used by the PostgreSQL store. |

---

## 10. Error Handling Strategy

| ID | Context | Behavior |
|----|---------|----------|
| ERR-01 | Config validation failure | Log error, exit 1 immediately. |
| ERR-02 | Database unreachable at startup | Log error, exit 1. Do not retry — let the orchestrator handle restarts. |
| ERR-03 | JWKS fetch failure at startup | Log error, exit 1. The server cannot validate tokens without keys. |
| ERR-04 | JWKS refetch failure (runtime) | Log warn, continue serving with cached keys. Retry on next unknown `kid`. |
| ERR-05 | Embed server unreachable (runtime) | Return MCP tool error to caller. Do not crash. |
| ERR-06 | HyDE generation failure | Log warn, fall back to raw query. Do not fail the search. |
| ERR-07 | Reranker failure | Log warn, fall back to RRF ordering. Do not fail the search. |
| ERR-08 | *(Removed — embedding server is external)* | |
| ERR-09 | Level 1 guardrail rejection | Return MCP tool result with `"off_topic"` error. Logged at `info`. |
| ERR-10 | Level 2 guardrail rejection | Return MCP tool result with `"below_threshold"` error (no score in message). Logged at `info` with score at `debug`. |
| ERR-11 | SQL query timeout | Return MCP tool error with "query timeout" message. |
| ERR-12 | SQL query syntax error | Return database syntax error message to client (safe). |
| ERR-13 | SQL query other error | Return generic "query execution failed" to client. Log details server-side. |
| ERR-14 | SQL blocked keyword detected | Return MCP tool error: "query contains disallowed keywords". |
| ERR-15 | Ingestion: per-file error | Log warn, skip file, continue. |
| ERR-16 | Ingestion: embed server unreachable | Log error, halt ingest, return partial results. |
| ERR-17 | Ingestion: directory not in allowed_dirs | Return error immediately. Do not walk. |
| ERR-18 | Unexpected panic | Recover in HTTP handler, log stack trace, return 500. |
| ERR-19 | SIGHUP reload: invalid config | Log error, keep running with previous config. |
| ERR-20 | SIGTERM / SIGINT | Graceful shutdown per MCP-07. |

---

## 11. Dependencies

### Required Go Modules

| Module | Purpose |
|--------|---------|
| `github.com/modelcontextprotocol/go-sdk` | MCP protocol server implementation |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/pgvector/pgvector-go` | pgvector type codec |
| `github.com/microsoft/go-mssqldb` | MS SQL Server driver |
| `github.com/lestrrat-go/jwx/v2` | JWT parsing, JWKS fetching, signature verification |
| `github.com/BurntSushi/toml` | TOML config parsing |
| `github.com/anthropics/anthropic-sdk-go` | Claude API for HyDE (optional feature) |

### External Infrastructure

| Component | Required | Notes |
|-----------|----------|-------|
| PostgreSQL 14+ with pgvector | For vector mode | `CREATE EXTENSION vector` must be enabled |
| MS SQL Server 2019+ | For SQL-only mode | Alternative to PostgreSQL; requires read-only DB user |
| AWS Cognito User Pool | Always | Provisioned via `emergingrobotics/aws-cognito` |
| Embedding server | For vector mode | External process (e.g., llama-server); bare metal with GPU recommended |
| Anthropic API | Only if HyDE or evals | `ANTHROPIC_API_KEY` env var |
| Podman or Docker | For containerized deployment | Podman preferred |

---

## 12. Development Workflow

### First-time Setup

```bash
# 1. Clone
git clone <repo-url>
cd mcp-authenticated-server

# 2. Install prerequisites
make prereqs  # Installs container engine, Hugging Face CLI

# 3. Start an embedding server (external, bare metal recommended for GPU)
# See README.md for llama-server setup instructions
make download-model  # Downloads GGUF model for use with llama-server
# Start llama-server separately: llama-server -m models/nomic-embed-text-v1.5.Q8_0.gguf --embedding --port 8079

# 4. Configure
cp config.toml.example config.toml
chmod 600 config.toml
# Edit config.toml: set embed.host to your embedding server URL
# Set [auth] section from your Cognito provisioning outputs

# 5. Provision Cognito (separate repo)
cd ../aws-cognito
aws-cognito -c  # Creates User Pool + App Client
# Copy outputs to config.toml [auth] section

# 6. Start local infrastructure
make container-up  # Start the MCP server container

# 7. Apply schema
make schema

# 8. Ingest documents (optional, for vector mode)
make ingest DIR=./data

# 9. Run evals (optional)
make eval EVAL_FILE=./data/evals/evals.json
```

### Fork Workflow

```bash
# 1. Fork the repo
gh repo fork mcp-authenticated-server --clone
cd mcp-authenticated-server

# 2. Add domain tools
# Create internal/tools/my_domain_tool.go
# Register in cmd/server/main.go

# 3. Add domain schema (if needed)
# Add DDL to internal/database/postgres/schema.go
# Add DDL to internal/database/mssql/schema.go

# 4. Write domain evals
# Create data/evals/evals.json

# 5. Configure and run
cp config.toml.example config.toml
chmod 600 config.toml
make container-up
make eval
```

---

## 13. Design Decisions (Resolved)

| Decision | Resolution | Implications |
|----------|-----------|-------------|
| **Container engine** | **Podman preferred, Docker supported.** Auto-detection tries `podman` first, falls back to `docker`. CLI flag and config can override. | Compose files named `compose.yml` (engine-neutral). Makefile uses detected engine. Host gateway handling is engine-aware. |
| **Embed server** | **External process, not bundled.** Embedding inference requires bare-metal GPU access for acceptable performance. Bundling inside the MCP server container would negate GPU passthrough benefits. | `embed.host` points to an external OpenAI-compatible endpoint. Operators deploy llama-server (or equivalent) separately on GPU hardware. Model GGUF files go in `models/` (gitignored). |
| **Vector support on MSSQL** | **No vector support on MS SQL Server.** Permanent constraint. | Vector tools not registered when engine is MSSQL. All vector-related config sections silently ignored. |
| **Multi-tenancy** | **Single connection pool, no per-tenant isolation.** All authenticated users share data and connections. | Fork authors can add per-user filtering in their domain tools using auth context (`sub`, `groups`). |
| **Config hot-reload** | **SIGHUP reload.** Runtime-tunable sections reloaded without restart. Structural sections and network-destination fields require restart (prevents SSRF via config modification). | Server logs reload events. Invalid config on reload is non-fatal — previous config retained. |
| **Transport protocol** | **HTTP/HTTPS only. No gRPC, WebSocket, or SSE.** | Direct TLS supported via config. Reverse proxy recommended for production. |
| **Guardrail architecture** | **Two-level system.** Level 1 (topic relevance) gates before DB queries. Level 2 (match score) gates after retrieval + reranking. | Both levels independently configurable. Zero overhead when disabled. Scores not exposed to clients. |
| **Go module structure** | **Single `go.mod` at repository root.** `internal/` is directly under the root module, accessible by `cmd/server/main.go`. | No cross-module import restrictions. |
| **MSSQL write prevention** | **Read-only database user as primary control.** Keyword blocking is defense-in-depth only. | Documentation must specify the database user setup. This is more reliable than transaction-level controls. |

---

## 14. Search Pipeline — Complete Flow

This section documents the full `search_documents` execution path for implementors. Each step corresponds to formal requirements referenced in parentheses.

```
 1. Receive search request: { query, limit }
 2. Authenticate (middleware — already done before tool handler) [AUTH-01]
 3. Per-tool authorize (Authorizer interface) [AUTH-11]
 4. HyDE expansion (if hyde.enabled) [ENH-01..ENH-05]:
    a. Call Claude API with system prompt + "Question: {query}"
    b. If success:
       - embedText = hypothesis
       - prefix = passage_prefix
    c. If failure or disabled:
       - embedText = query
       - prefix = query_prefix
       - Log warn on failure [ERR-06]
 5. Embed: POST /v1/embeddings with {model, input: prefix + embedText} [VEC-05]
 6. L2-normalize the embedding vector (in-place) [VEC-06]
 7. Level 1 guardrail (if guardrails.corpus_topic configured) [GUARD-01, GUARD-02]:
    a. dotProduct(queryVector, topicVector)
    b. If score < min_topic_score → return "off_topic" error [ERR-09]
 8. Parallel retrieval [VEC-07, VEC-09, PERF-04]:
    a. Goroutine 1: SET ivfflat.probes = N (if index exists);
       SELECT ... ORDER BY embedding <=> $1 LIMIT pool_size
    b. Goroutine 2: SELECT ... WHERE content_fts @@ plainto_tsquery('english', $1)
       ORDER BY score DESC LIMIT pool_size
    c. Wait for both
 9. RRF merge: score(d) = Σ 1/(k + rank_i(d)) for each arm where d appears [VEC-08]
10. Reranking (if reranker.enabled) [ENH-06..ENH-10]:
    a. Build rerank documents: filename[: heading_context] + "\n\n" + content
    b. POST /rerank with { query, documents, top_n }
    c. If success: replace scores, re-sort descending
    d. If error: log warn, keep RRF scores [ERR-07]
11. Level 2 guardrail (if guardrails.min_match_score > 0) [GUARD-03, GUARD-04]:
    a. If merged[0].Score < min_match_score → return "below_threshold" error [ERR-10]
12. Truncate to requested limit
13. Return { results: [...], total_chunks_in_db: N }
    (Empty DB returns { results: [], total_chunks_in_db: 0 } — not an error)
```
