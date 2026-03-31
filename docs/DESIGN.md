# MCP Authenticated Server -- Design Document

## Overview

A production-ready, fork-friendly Go template for building authenticated MCP (Model Context Protocol) servers backed by a relational database. Provides authentication, database access, vector search, SQL querying, guardrails, health checks, configuration, container orchestration, and MCP protocol handling out of the box.

Fork authors add domain logic (new tools, tables, eval entries) without touching framework code.

## Architecture

```mermaid
graph TB
    Clients["MCP Clients<br/>(Claude, agents, etc.)"]

    subgraph Server["mcp-server (Go binary)"]
        Auth["Auth Middleware<br/>(Cognito JWT)"]
        Authz["Per-tool Authorizer"]
        MCP["MCP Handler"]
        Tools["Tool Registry"]
        Search["search_documents<br/>(vector)"]
        Query["query_data<br/>(SQL)"]
        Ingest["ingest_documents"]
        Fork["(fork-added tools)"]

        Guard["Guardrails<br/>L1: topic gate<br/>L2: score gate"]
        Pipeline["Search Pipeline<br/>(KNN + FTS + RRF)"]
        EmbedClient["Embed Client<br/>(/v1/embeddings)"]
        HyDE["HyDE Generator<br/>(query expand)"]
        SQLStore["SQL Store<br/>(read-only exec)"]
        Reranker["Reranker<br/>(optional)"]

        DAL["Database Abstraction Layer<br/>(PostgreSQL + pgvector OR MS SQL Server)"]
    end

    Clients -- "HTTPS / JSON-RPC<br/>(POST /mcp)" --> Auth
    Auth --> MCP
    Auth --> Authz
    MCP --> Tools
    Tools --> Search
    Tools --> Query
    Tools --> Ingest
    Tools --> Fork
    Search --> Guard
    Search --> Pipeline
    Search --> EmbedClient
    Search --> HyDE
    Query --> SQLStore
    Pipeline --> DAL
    SQLStore --> DAL
    Ingest --> DAL
```

## Package Dependency DAG

All imports flow downward. No circular imports.

```mermaid
graph TD
    cmd["cmd/server"]
    server["internal/server"]
    tools["internal/tools"]
    search["internal/search<br/>(pipeline, RRF)"]
    vectorstore["internal/vectorstore"]
    guardrails["internal/guardrails"]
    hyde["internal/hyde"]
    rerank["internal/rerank"]
    embed["internal/embed"]
    querystore["internal/querystore"]
    ingest["internal/ingest"]
    auth["internal/auth"]
    config["internal/config"]
    database["internal/database"]
    postgres["internal/database/postgres"]
    mssql["internal/database/mssql"]
    engine["internal/engine"]
    vecmath["vecmath<br/>(leaf package)"]

    cmd --> server
    server --> tools
    server --> auth
    server --> config
    server --> database
    server --> engine
    tools --> search
    tools --> querystore
    tools --> ingest
    search --> vectorstore
    search --> guardrails
    search --> hyde
    search --> rerank
    search --> embed
    ingest --> embed
    ingest --> vectorstore
    database --> postgres
    database --> mssql
    guardrails -.-> vecmath
    vectorstore -.-> vecmath
    embed -.-> vecmath
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

```mermaid
flowchart TD
    A["Receive { query, limit }"] --> B["Auth middleware<br/>(already applied)"]
    B --> C["Per-tool authorization check"]
    C --> D{"HyDE enabled?"}
    D -- Yes --> E["HyDE expansion<br/>embedText = hypothesis<br/>prefix = passage_prefix"]
    D -- No --> F["embedText = raw query<br/>prefix = query_prefix"]
    E --> G["Embed via /v1/embeddings"]
    F --> G
    G --> H["L2-normalize embedding"]
    H --> I{"L1 Guardrail:<br/>topic relevance"}
    I -- "below threshold" --> Reject1["Return: off_topic error"]
    I -- "pass" --> J

    subgraph Parallel["Parallel Retrieval"]
        J["KNN vector search"]
        K["Full-text search"]
    end

    H --> K
    J --> L["RRF merge<br/>score = sum(1/(k + rank))"]
    K --> L
    L --> M{"Reranker<br/>enabled?"}
    M -- Yes --> N["Cross-encoder rescoring"]
    M -- No --> O
    N --> O{"L2 Guardrail:<br/>min match score"}
    O -- "below threshold" --> Reject2["Return: below_threshold error"]
    O -- "pass" --> P["Truncate to limit"]
    P --> Q["Return results"]
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
