# MCP Authenticated Server

A production-ready, fork-friendly Go template for building authenticated MCP (Model Context Protocol) servers backed by a relational database.

Provides authentication, database access, vector search, SQL querying, guardrails, health checks, configuration, container orchestration, and MCP protocol handling out of the box. Fork authors add domain logic (new tools, tables, eval entries) without touching framework code.

## Key Features

- **AWS Cognito JWT authentication** with JWKS caching and singleflight refresh
- **Dual database engine support** — PostgreSQL (with pgvector) and MS SQL Server
- **Vector/semantic search** — parallel KNN + full-text retrieval with RRF merge
- **SQL query mode** — read-only SQL execution with keyword blocking
- **Document ingestion** — structure-aware markdown chunking with idempotency
- **Two-level guardrails** — topic relevance gating and match score filtering
- **HyDE query expansion** — optional hypothetical document embeddings via Claude API
- **Cross-encoder reranking** — optional reranking via HTTP endpoint
- **Container-first** — Podman-preferred, Docker-compatible deployment
- **Evaluation framework** — LLM-as-judge RAG quality measurement

## Architecture

See [docs/DESIGN.md](docs/DESIGN.md) for the full architecture, package dependency DAG, data model, and extension points.

```mermaid
graph TB
    Clients["MCP Clients<br/>(Claude, agents, etc.)"]

    subgraph Server["mcp-server (Go binary)"]
        Auth["Auth Middleware<br/>(Cognito JWT)"]
        MCP["MCP Handler<br/>(Streamable HTTP)"]
        Tools["Tool Registry"]
        SearchTool["search_documents"]
        QueryTool["query_data"]
        IngestTool["ingest_documents"]
        ForkTools["(fork-added tools)"]

        subgraph Pipeline["Search Pipeline"]
            HyDE["HyDE<br/>Query Expansion"]
            Embed["Embed Client<br/>(/v1/embeddings)"]
            Guard["Guardrails<br/>L1: Topic Gate<br/>L2: Score Gate"]
            KNN["KNN Vector Search"]
            FTS["Full-Text Search"]
            RRF["RRF Merge"]
            Rerank["Reranker<br/>(optional)"]
        end

        DAL["Database Abstraction Layer"]
    end

    Cognito["AWS Cognito<br/>(JWKS)"]
    Anthropic["Anthropic API<br/>(Claude)"]
    EmbedServer["Embed Server<br/>(llama-server)"]
    RerankerServer["Reranker Server<br/>(cross-encoder)"]
    PG["PostgreSQL<br/>+ pgvector"]
    MSSQL["MS SQL Server"]

    Clients -- "HTTPS / JSON-RPC<br/>(POST /mcp)" --> Auth
    Auth --> MCP
    Auth -. "validate JWT" .-> Cognito
    MCP --> Tools
    Tools --> SearchTool
    Tools --> QueryTool
    Tools --> IngestTool
    Tools --> ForkTools

    SearchTool --> HyDE
    HyDE -. "generate hypothesis" .-> Anthropic
    HyDE --> Embed
    Embed -. "/v1/embeddings" .-> EmbedServer
    Embed --> Guard
    Guard --> KNN
    Guard --> FTS
    KNN --> RRF
    FTS --> RRF
    RRF --> Rerank
    Rerank -. "/rerank" .-> RerankerServer

    DAL --> PG
    DAL --> MSSQL
    KNN --> DAL
    FTS --> DAL
    QueryTool --> DAL
    IngestTool --> DAL
```

### Request Sequence

A typical `search_documents` call showing the MCP protocol handshake and search pipeline:

```mermaid
sequenceDiagram
    participant C as MCP Client
    participant S as mcp-server
    participant Co as AWS Cognito
    participant A as Anthropic API
    participant E as Embed Server
    participant DB as PostgreSQL
    participant R as Reranker

    Note over C,S: Session Initialization
    C->>S: POST /mcp (initialize)
    S->>C: capabilities + Mcp-Session-Id
    C->>S: POST /mcp (notifications/initialized)

    Note over C,S: Tool Call
    C->>S: POST /mcp (tools/call: search_documents)
    S->>Co: Validate JWT (cached JWKS)
    Co-->>S: Token valid

    Note over S: Search Pipeline
    S->>A: HyDE: generate hypothesis (optional)
    A-->>S: Hypothetical passage
    S->>E: POST /v1/embeddings
    E-->>S: Query embedding vector

    Note over S: L1 Guardrail: topic relevance check

    par Parallel Retrieval
        S->>DB: KNN vector search (pgvector)
        DB-->>S: KNN results
    and
        S->>DB: Full-text search (tsvector)
        DB-->>S: FTS results
    end

    Note over S: RRF merge

    S->>R: POST /rerank (optional)
    R-->>S: Reranked scores

    Note over S: L2 Guardrail: score threshold check

    S->>C: Search results (JSON-RPC response)
```

## Prerequisites

Before running the MCP server you need:

1. **llama-server** (or any OpenAI-compatible embedding endpoint) installed and in your PATH. Embedding inference requires bare-metal GPU access for acceptable performance. See [docs/installing-llama-server.md](docs/installing-llama-server.md) for installation instructions.

2. **PostgreSQL 14+** with the [pgvector](https://github.com/pgvector/pgvector) extension (for vector mode), or **MS SQL Server 2019+** (SQL-only mode). The database is an external dependency -- you provision and run it yourself. See database setup below.

3. **AWS Cognito User Pool** provisioned with an App Client. The server validates all requests against Cognito-issued JWTs. See [docs/aws-cognito-setup.md](docs/aws-cognito-setup.md) for setup instructions, or the [emergingrobotics/aws-cognito](https://github.com/emergingrobotics/aws-cognito) CLI tool for automated provisioning.

4. **Go 1.23+** for building from source.

## Database Setup

### PostgreSQL with pgvector

Install pgvector if you haven't already:

```bash
# Ubuntu/Debian
sudo apt install postgresql-16-pgvector

# Fedora/RHEL
sudo dnf install pgvector_16

# macOS (Homebrew)
brew install pgvector
```

Create the database and enable the extension:

```bash
sudo -u postgres createdb mcp_server
sudo -u postgres psql -d mcp_server -c 'CREATE EXTENSION IF NOT EXISTS vector'
```

Verify pgvector is installed:

```bash
sudo -u postgres psql -d mcp_server -c 'SELECT extversion FROM pg_extension WHERE extname = $$vector$$'
```

If your PostgreSQL user requires a password, set one:

```bash
sudo -u postgres psql -c "ALTER USER your_username WITH PASSWORD 'your_password'"
```

### MS SQL Server (SQL-only mode, no vector features)

For MSSQL setup, create a read-only database user with SELECT and EXECUTE permissions only. Set `database.engine = "mssql"` in config.toml.

## Quick Start

### Core setup (all modes)

```bash
# 1. Clone
git clone <repo-url>
cd mcp-authenticated-server

# 2. Configure
cp config.toml.example config.toml
chmod 600 config.toml

# If you have an aws-cognito JSON config file, import auth values automatically:
./scripts/configure-auth.sh path/to/cognito.json config.toml

# Or edit config.toml manually -- set [auth] region, user_pool_id, client_id
# from your Cognito User Pool. See docs/aws-cognito-setup.md for details.

# Set DATABASE_URL in your .envrc file.
# Use the database you created in the "Database Setup" section above:
echo 'export DATABASE_URL=postgres://your_username:your_password@localhost:5432/mcp_server?sslmode=disable' >> .envrc
chmod 600 .envrc
source .envrc

# 3. Apply schema
make schema

# 4. Run the MCP server
make run
```

At this point the server is running with the `query_data` tool, which lets MCP clients execute read-only SQL against your database. This is sufficient if your goal is database access without vector search.

### Vector search setup (optional)

If you want semantic search (`search_documents`) and document ingestion (`ingest_documents`), you also need an embedding server and ingested documents:

```bash
# 5. Download the embedding model
make download-model

# 6. Start the embedding server (separate terminal, bare metal for GPU)
make embed-server

# 7. Ingest documents
make ingest DIR=./data

# 8. Run evaluations (optional, requires ANTHROPIC_API_KEY for the LLM judge)
export ANTHROPIC_API_KEY="sk-ant-..."
make eval EVAL_FILE=data/evals/evals.json
```

Set `embed.enabled = true` in config.toml (the default) to register the vector tools. Set `embed.enabled = false` to disable them entirely, in which case only `query_data` is available and no embedding server is needed.

The embedding server (`make embed-server`) runs as a long-lived process in a separate terminal. Any OpenAI-compatible `/v1/embeddings` endpoint works as an alternative (vLLM, TEI, OpenAI API, etc.) -- just set `embed.host` in `config.toml`.

## HyDE (Hypothetical Document Embeddings)

HyDE improves semantic search by expanding the user query into a hypothetical document passage before embedding. Instead of embedding the raw query (which is typically short and keyword-like), the server asks Claude to generate a 2-3 sentence passage that would answer the question, then embeds that passage. This produces an embedding closer to the actual document space, improving retrieval accuracy.

### Enabling HyDE

1. Set `ANTHROPIC_API_KEY` in your environment:

```bash
echo 'export ANTHROPIC_API_KEY="sk-ant-..."' >> .envrc
source .envrc
```

2. Enable in `config.toml`:

```toml
[hyde]
enabled = true
model = "claude-haiku-4-5-20251001"
```

If `ANTHROPIC_API_KEY` is not set, HyDE silently falls back to raw query passthrough (no error, no degradation).

### Configuration

```toml
[hyde]
# Master switch (default: false)
enabled = false

# LLM model for hypothesis generation (default: claude-haiku-4-5-20251001)
model = "claude-haiku-4-5-20251001"

# Override Anthropic API base URL (default: empty = api.anthropic.com)
# Useful for proxies or compatible endpoints.
base_url = ""

# Custom system prompt (default: empty = built-in prompt)
# The built-in prompt instructs Claude to write a 2-3 sentence answer
# as if it appeared verbatim in the documentation.
system_prompt = ""
```

### How it works in the pipeline

1. User query arrives at `search_documents`
2. Claude generates a hypothetical passage answering the query
3. The passage is embedded using the passage prefix (instead of query prefix)
4. The resulting embedding is used for KNN vector search
5. Full-text search still uses the raw query for lexical matching
6. Both arms are merged via RRF as usual

### Reloading

The `[hyde]` section is reloadable via SIGHUP, except `base_url` which requires a restart.

## Cross-Encoder Reranking

The reranker is an optional post-retrieval step that rescores search results using a cross-encoder model. Cross-encoders jointly encode the (query, document) pair, producing more accurate relevance scores than the embedding-based similarity used for initial retrieval. This improves precision at the cost of one additional HTTP call per search.

### Running a reranker server

The reranker is an external HTTP service exposing a `POST /rerank` endpoint. Any server implementing this API works. For example, using a Hugging Face cross-encoder model via a Python server:

```bash
# Example using a cross-encoder model (you provide this service)
# The server must accept POST /rerank with:
#   {"query": "...", "documents": ["...", "..."], "top_n": 5}
# And return:
#   {"results": [{"index": 0, "relevance_score": 0.95}, ...]}
```

### Enabling the reranker

Set the reranker host and enable it in `config.toml`:

```toml
[reranker]
enabled = true
host = "http://localhost:8081"
```

### Configuration

```toml
[reranker]
# Master switch (default: false)
enabled = false

# HTTP endpoint serving POST /rerank (default: http://localhost:8081)
host = "http://localhost:8081"
```

### How it works in the pipeline

1. KNN and full-text search results are merged via Reciprocal Rank Fusion (RRF)
2. The merged candidate texts are sent to the reranker with the original query
3. The reranker returns relevance scores from the cross-encoder
4. Results are re-sorted by cross-encoder score
5. Level 2 guardrail (min_match_score) is applied to the reranked scores

### Failure behavior

If the reranker is unreachable, returns a non-200 status, or returns unparseable output, the server logs a warning and falls back to RRF scores. Search results are still returned -- reranker failure is non-fatal.

### Reranker API contract

The reranker endpoint must implement:

**Request** (`POST /rerank`):
```json
{
  "query": "user's search query",
  "documents": ["chunk 1 text", "chunk 2 text", "..."],
  "top_n": 5
}
```

**Response** (`200 OK`):
```json
{
  "results": [
    {"index": 0, "relevance_score": 0.95},
    {"index": 2, "relevance_score": 0.87}
  ]
}
```

`index` refers to the position in the input `documents` array. `relevance_score` is a float in `[0.0, 1.0]`.

## Build

```bash
make build          # Build binary to bin/mcp-server
make test           # Run unit tests
make test-integration  # Run with real PostgreSQL (requires TEST_DATABASE_URL)
make test-coverage  # HTML coverage report
make lint           # golangci-lint
make govulncheck    # Dependency vulnerability scan
```

## CLI

```bash
./bin/mcp-server serve          # Start HTTP server (default)
./bin/mcp-server ingest --dir /data --drop  # One-shot ingestion
./bin/mcp-server validate       # Validate config and exit
./bin/mcp-server schema         # Apply DB schema and exit
```

All subcommands accept `--config PATH` (default: `config.toml`).

## Configuration

See [config.toml.example](config.toml.example) for all fields with documentation.

Secrets are always environment variables:
- `DATABASE_URL` — database connection string
- `ANTHROPIC_API_KEY` — required for HyDE and evaluations

SIGHUP reloads: `[search]`, `[guardrails]`, `[hyde]` (except base_url), `[query]`, `log_level`.
Restart required: `[database]`, `[auth]`, `[embed]`, `[server].port`, `[server].tls_*`.

## MCP Tools

| Tool | Engine | Description |
|------|--------|-------------|
| `search_documents` | PostgreSQL + embed | Semantic + full-text search with guardrails |
| `query_data` | Both | Read-only SQL query execution |
| `ingest_documents` | PostgreSQL + embed | Document ingestion (requires admin group) |

## Fork Workflow

```bash
# 1. Fork and clone
gh repo fork mcp-authenticated-server --clone
cd mcp-authenticated-server

# 2. Add domain tools
# Create internal/tools/my_tool.go
# Register in cmd/server/main.go

# 3. Add domain schema (optional)
# Add DDL to internal/database/postgres/schema.go

# 4. Write evals
# Create data/evals/evals.json

# 5. Configure, build, and run
cp config.toml.example config.toml
make container-up && make eval
```

Fork authors touch at most 3 files. See [REQUIREMENTS.md section 7](REQUIREMENTS.md#7-forkability-contract) for the full contract.

## Security Invariants

- All SQL queries use parameterized queries
- All process exec calls use `[]string` argument slices (no `sh -c`)
- Secrets come from environment variables only
- File reads validate paths against allowed directories
- JWT tokens are never logged
- Embedding server runs externally, not inside the MCP server container

## Container Engine

Podman is preferred, Docker is supported. Auto-detected from PATH.

```bash
make container-up               # Uses detected engine
make container-up ENGINE=docker # Force Docker
```

## Evaluation

```bash
make eval                # Run eval suite
make eval-stability      # Run 25x, report min/max/avg pass rates
```

Eval entries are JSON with `prompt`, `label` (good/bad), and `notes`. See [data/evals/evals.json.example](data/evals/evals.json.example).
