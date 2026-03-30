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
- **Container-first** — Podman-preferred, Docker-compatible with bundled embedding server
- **Evaluation framework** — LLM-as-judge RAG quality measurement

## Architecture

See [docs/DESIGN.md](docs/DESIGN.md) for the full architecture, package dependency DAG, data model, and extension points.

```
MCP Clients (Claude, agents, etc.)
        |
        | HTTPS / JSON-RPC (POST /mcp)
        v
+-------------------------------------------------------+
|                 mcp-server (Go binary)                 |
|  Auth Middleware (Cognito JWT) --> MCP Handler          |
|  Tool Registry: search_documents, query_data,          |
|                 ingest_documents, (fork-added tools)   |
|  Search Pipeline: HyDE -> Embed -> Guardrails ->       |
|                   KNN+FTS -> RRF -> Rerank -> Filter   |
|  Database Abstraction (PostgreSQL OR MSSQL)            |
+-------------------------------------------------------+
```

## Quick Start

```bash
# 1. Clone
git clone --recurse-submodules <repo-url>
cd mcp-authenticated-server

# 2. Install prerequisites
make prereqs

# 3. Download embedding model
make download-model

# 4. Configure
cp config.toml.example config.toml
chmod 600 config.toml
# Edit config.toml with your Cognito + database settings

# 5. Start local infrastructure
make container-up

# 6. Apply schema
make schema

# 7. Ingest documents
make ingest DIR=./data

# 8. Run evaluations
make eval
```

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
