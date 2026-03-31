# Slide Deck Outline: MCP Authenticated Server

## Slide 1: Title

**MCP Authenticated Server**
A production-ready, fork-friendly Go template for building authenticated RAG servers using the Model Context Protocol.

## Slide 2: The Problem

- AI agents (Claude, custom agents) need structured access to private data
- Building a secure, authenticated RAG pipeline from scratch is complex
- Every team rebuilds the same infrastructure: auth, search, ingestion, eval
- Need: a reusable template that handles the plumbing so teams focus on domain logic

## Slide 3: What MCP Is

- Model Context Protocol -- open standard for AI agent tool use
- JSON-RPC over HTTP (Streamable HTTP transport)
- Clients send `initialize`, then `tools/call` requests
- Server exposes tools that agents discover and invoke
- Session-based: initialize handshake, then stateful interaction

## Slide 4: Architecture Overview

*Block diagram (Mermaid) showing:*
- MCP Clients at the top
- HTTPS/JSON-RPC connection
- mcp-server binary with internal components
- External services: PostgreSQL, Embed Server, Anthropic API, Reranker
- AWS Cognito for auth

## Slide 5: Authentication Flow

- Every request carries a JWT (AWS Cognito)
- Auth middleware validates token against JWKS endpoint (cached, singleflight refresh)
- Per-tool authorization: tools can require specific Cognito group membership
- Example: `ingest_documents` requires `admin` group

## Slide 6: The Three Built-In Tools

| Tool | What It Does |
|------|-------------|
| `search_documents` | Semantic + full-text search with guardrails |
| `query_data` | Read-only SQL execution with keyword blocking |
| `ingest_documents` | Document ingestion with structure-aware chunking |

- Fork authors add domain tools without touching framework code

## Slide 7: Request Lifecycle -- Sequence Diagram

*Sequence diagram (Mermaid) showing:*
1. Client -> Server: initialize
2. Server -> Client: capabilities + session ID
3. Client -> Server: tools/call (search_documents)
4. Server -> Cognito JWKS: validate JWT
5. Server -> Anthropic API: HyDE expansion (optional)
6. Server -> Embed Server: embed query
7. Server -> PostgreSQL: parallel KNN + FTS
8. Server -> Reranker: rerank results (optional)
9. Server -> Client: search results

## Slide 8: Search Pipeline Deep Dive

*Flowchart (Mermaid) showing the 13-step pipeline:*
1. Query arrives
2. HyDE expansion (optional) -- Claude generates hypothetical passage
3. Embed via external server
4. L2-normalize
5. Level 1 guardrail: topic relevance gate
6. Parallel: KNN vector search + full-text search
7. RRF merge
8. Reranking (optional)
9. Level 2 guardrail: score threshold
10. Truncate and return

## Slide 9: HyDE -- Query Expansion

- Problem: user queries are short and keyword-like; document passages are dense and specific
- Solution: ask Claude to write a hypothetical 2-3 sentence passage answering the query
- Embed the passage (closer to document space) instead of the raw query
- Falls back gracefully: if API key missing or call fails, uses raw query
- Configurable model, system prompt, enable/disable via SIGHUP

## Slide 10: Retrieval -- Two Arms, One Merge

- **KNN arm**: cosine similarity search on pgvector embeddings
- **FTS arm**: PostgreSQL full-text search with tsvector/tsquery
- Both run in parallel, return ranked candidate pools
- **Reciprocal Rank Fusion**: `score(d) = sum(1/(k + rank_i(d)))`
- Combines semantic understanding with lexical precision

## Slide 11: Cross-Encoder Reranking

- Optional post-retrieval step
- External HTTP service scores (query, document) pairs jointly
- More accurate than embedding similarity but more expensive
- Non-fatal: falls back to RRF scores if reranker is down
- Any service implementing `POST /rerank` works

## Slide 12: Guardrails

- **Level 1 -- Topic Relevance Gate** (pre-database)
  - Cosine similarity between query and corpus topic embedding
  - Blocks off-topic queries before they hit the database
- **Level 2 -- Match Score Gate** (post-retrieval)
  - Checks best result score after RRF/reranking
  - Blocks low-confidence results
- Both: zero overhead when disabled, configurable thresholds, SIGHUP reloadable

## Slide 13: Document Ingestion

- Structure-aware markdown chunking (headings, paragraphs, lists)
- Idempotent: content-hash deduplication (SHA-256)
- Batch embedding with configurable chunk size and batch size
- File type whitelist, size limits, directory restrictions
- Admin-only: requires Cognito group membership

## Slide 14: Database Support

- **PostgreSQL + pgvector**: full feature set (vector search, FTS, ingestion)
- **MS SQL Server**: SQL-only mode (query_data tool only, no vector features)
- Database abstraction layer: `database.Store` interface
- Schema applied via `make schema` or `mcp-server schema`

## Slide 15: Evaluation Framework

- LLM-as-judge approach using Claude
- Three eval label types:
  - `good`: question answerable from corpus -- judge checks retrieval quality
  - `bad`: fabricated question -- judge checks hallucination resistance
  - `off_topic`: outside corpus domain -- LLM reply printed for human review
- Per-label pass rate breakdown
- Stability testing: run 25x, report min/max/avg

## Slide 16: Configuration and Operations

- TOML config file + environment variables for secrets
- Hot reload via SIGHUP: search params, guardrails, HyDE, query limits
- Restart required: database, auth, embedding, TLS
- Health endpoint: `GET /healthz` (no auth)
- Container-first: Podman preferred, Docker supported

## Slide 17: Fork Workflow

- Fork the repo, add your domain logic
- Touch at most 3 files:
  1. `internal/tools/my_tool.go` -- your tool
  2. `cmd/server/main.go` -- register it
  3. `internal/database/postgres/schema.go` -- your tables (optional)
- Write evals in `data/evals/evals.json`
- Everything else (auth, search, guardrails, config) works out of the box

## Slide 18: Security

- All SQL: parameterized queries only
- All exec: explicit argv slices, no shell interpretation
- All secrets: environment variables, never in config files or logs
- File reads: validated against allowed directories
- JWT tokens: never logged
- Container: non-root user, pinned base images with SHA256 digest

## Slide 19: What's Next -- Proposed Guardrail Extensions

Ranked by impact:
1. Prompt injection detection (regex + optional LLM classifier)
2. PII/sensitive data filtering (redact or block)
3. Query length/complexity bounds
4. Rate-based abuse detection
5. Content coherence checking
6. Result diversity guard

## Slide 20: Summary

- Fork-friendly template: add tools, not infrastructure
- Full RAG pipeline: HyDE, embedding, KNN+FTS, RRF, reranking, guardrails
- Production-ready: auth, config reload, health checks, eval framework
- Secure by default: parameterized queries, validated inputs, no secrets in logs
- Extensible: new tools, new guardrails, new database engines
