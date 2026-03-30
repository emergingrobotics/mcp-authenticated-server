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
        |                          |
        v                          v
   PostgreSQL + pgvector     External Embed Server
   (or MS SQL Server)        (e.g., llama-server)
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

```bash
# 1. Clone
git clone <repo-url>
cd mcp-authenticated-server

# 2. Download the embedding model
make download-model

# 3. Start the embedding server (separate terminal, bare metal for GPU)
make embed-server

# 4. Configure
cp config.toml.example config.toml
chmod 600 config.toml

# If you have an aws-cognito JSON config file, import auth values automatically:
./scripts/configure-auth.sh cognito/config.json config.toml

# Or edit config.toml manually -- set [auth] region, user_pool_id, client_id
# from your Cognito User Pool. See docs/aws-cognito-setup.md for details.

# Set DATABASE_URL in your .envrc file.
# Use the database you created in the "Database Setup" section above:
echo 'export DATABASE_URL=postgres://your_username:your_password@localhost:5432/mcp_server?sslmode=disable' >> .envrc
chmod 600 .envrc

# 5. Apply schema
make schema

# 6. Ingest documents
make ingest DIR=./data

# 7. Run the MCP server
make run

# 8. Run evaluations (optional, requires ANTHROPIC_API_KEY for the LLM judge)
export ANTHROPIC_API_KEY="sk-ant-..."
make eval EVAL_FILE=data/evals/evals.json
```

The embedding server (`make embed-server`) runs as a long-lived process in a separate terminal. It must be running before ingestion or search will work. Any OpenAI-compatible `/v1/embeddings` endpoint works as an alternative (vLLM, TEI, OpenAI API, etc.) -- just set `embed.host` in `config.toml`.

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
