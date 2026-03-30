# Test Plan -- MCP Authenticated Server

## 1. Unit Test Strategy by Package

Each package has its own `_test.go` files. All unit tests run via `make test` (`go test ./... -short -count=1`) with no external dependencies.

### 1.1 `internal/config` (TEST-03: config validation)

| Test Area | What to Assert |
|-----------|---------------|
| Valid TOML loading | Parses all sections, env var overlay (DATABASE_URL) |
| Missing required fields | Error on empty `auth.region`, `auth.user_pool_id`, `auth.client_id` |
| Score range validation | `min_topic_score` and `min_match_score` outside [0.0, 1.0] rejected |
| URL scheme validation | `embed.host`, `reranker.host`, `hyde.base_url` must be http/https |
| Cross-field: corpus_topic | Non-empty `corpus_topic` with `embed.enabled=false` rejected (GUARD-08) |
| Cross-field: TLS | `tls_cert` without `tls_key` (and vice versa) rejected |
| Cross-field: allowed_dirs | Empty `ingest.allowed_dirs` with `embed.enabled=true` rejected |
| Timeout max | `query.timeout` > 5m rejected |
| Enum fields | `token_use` not in {access, id} rejected; `database.engine` not in {postgres, mssql} rejected |
| SIGHUP reload | `ApplyReload` changes only reloadable fields; invalid reload config returns error, old config preserved; changed sections reported |

### 1.2 `internal/vecmath` (TEST-03: L2 normalization)

| Test Area | What to Assert |
|-----------|---------------|
| L2Normalize | Normalized vector magnitude ~1.0; zero vector unchanged |
| DotProduct | Correct result; mismatched lengths panic |
| CosineSimilarity | Equals DotProduct for L2-normalized inputs |

### 1.3 `internal/engine`

| Test Area | What to Assert |
|-----------|---------------|
| Detection priority | flag > config > PATH (podman first) |
| Injectable lookPath | Podman found -> podman; podman missing + docker found -> docker; neither -> error |
| Command builders | `ComposeCmd`, `RunCmd`, `BuildCmd` return correct `[]string` argv |
| Host gateway | Docker: `--add-host`; Podman: native |

### 1.4 `internal/auth` (TEST-03: JWT validation, per-tool authorization)

| Test Area | What to Assert |
|-----------|---------------|
| **JWT validation (cognito_test.go)** | |
| Valid token | Passes all checks, claims extracted (sub, email, groups, scope) |
| Expired token (exp) | Rejected |
| Future nbf | Rejected |
| Wrong issuer | Rejected |
| Wrong audience/client_id | Rejected (per token_use) |
| Invalid signature | Rejected |
| token_use mismatch | Rejected |
| Unknown kid | Triggers JWKS refetch via singleflight |
| JWKS refetch rate limit | Second refetch within 60s uses cache |
| JWKS fetch timeout | 10s timeout enforced |
| JWKS refetch failure | Cached keys retained (ERR-04) |
| **Authorizer (authorizer_test.go)** | |
| No required groups | Authorized |
| User has required group | Authorized |
| User lacks required group | 403 |
| `ingest_documents` default | Requires explicit group (AUTH-11) |
| Server-wide allowed_groups | Empty = no restriction; non-empty = enforced (AUTH-09) |
| **Context helpers** | WithClaims/ClaimsFromContext round-trip |

### 1.5 `internal/embed` (TEST-03: embedding prefix selection)

| Test Area | What to Assert |
|-----------|---------------|
| Successful embedding | Returns L2-normalized vectors |
| Prefix selection | `query_prefix` for queries, `passage_prefix` for passages (ENH-11) |
| Batch embedding | Multiple inputs in single request |
| HTTP timeout | Bounded |
| Response body limit | 4 MiB cap (SEC-04) |
| Error handling | Unreachable server returns error, not panic (ERR-05) |

### 1.6 `internal/vectorstore`

| Test Area | What to Assert |
|-----------|---------------|
| InsertDocument/GetDocumentByPath | Round-trip |
| InsertChunks/SearchKNN/SearchFTS | Correct SQL generation |
| DropAndRecreateTables | Idempotent DDL |
| CreateIVFFlatIndex | Only when >= 100 chunks |

### 1.7 `internal/querystore` (TEST-03: SQL keyword blocking)

| Test Area | What to Assert |
|-----------|---------------|
| **safety_test.go** | |
| DDL blocked | CREATE, ALTER, DROP |
| DML blocked | INSERT, UPDATE, DELETE, TRUNCATE, MERGE, REPLACE |
| Admin blocked | GRANT, REVOKE, EXEC, EXECUTE, xp_, sp_executesql, OPENROWSET, OPENDATASOURCE, BULK, COPY, LOAD, CALL |
| Transaction control blocked | BEGIN, COMMIT, ROLLBACK, SAVEPOINT |
| Session blocked | SET, DECLARE |
| SELECT INTO blocked | Detected and rejected |
| Comment stripping | `-- comment` and `/* block */` removed before scan |
| Semicolons | Rejected outside string literals; allowed inside string literals |
| Word boundaries | "DESCRIPTION" does not match "CREATE"; case-insensitive |
| Valid SELECT | Passes |
| **query_test.go** | |
| Row limit | Default 100, max 1000 |
| Response size cap | 10 MiB with truncation flag |
| Query timeout | Configurable, max 5m |
| Read-only tx (Postgres) | Enforced |
| Error sanitization | Generic message to client; syntax errors passed through |

### 1.8 `internal/search` (TEST-03: RRF merge)

| Test Area | What to Assert |
|-----------|---------------|
| RRF merge formula | `score(d) = sum(1/(k + rank_i(d)))` |
| Both-list document | Combined score from both arms |
| Single-list document | Single-arm score only |
| k-constant | Default 60, configurable |
| Sort order | Descending by merged score |
| Empty lists | No panic, empty result |
| Pipeline orchestration | HyDE -> embed -> L1 -> parallel KNN+FTS -> RRF -> rerank -> L2 -> truncate |

### 1.9 `internal/guardrails` (TEST-03: guardrail scores)

| Test Area | What to Assert |
|-----------|---------------|
| L1 below threshold | Returns "off_topic" error |
| L1 above threshold | Passes |
| L1 disabled (empty corpus_topic) | Skipped, zero overhead |
| L2 below min_match_score | Returns "below_threshold" error |
| L2 above threshold | Passes |
| L2 disabled (score=0) | Skipped |
| Both disabled | No checks |
| Topic vector reuse | Computed once, same pointer/value on subsequent calls |

### 1.10 `internal/ingest` (TEST-03: chunking)

| Test Area | What to Assert |
|-----------|---------------|
| **chunker_test.go** | |
| Heading stack | H1 sets level 1; H2 under H1 -> "H1 > H2" breadcrumb |
| Heading level change | H3 after H2 extends; H1 after H3 truncates |
| Heading flush | Accumulated text flushed with PREVIOUS breadcrumb on heading change |
| Code blocks | Fenced ``` -> atomic chunk, chunk_type="code" |
| Tables | Consecutive `\|` lines -> atomic chunk, chunk_type="table" |
| Lists | `-`, `*`, `N.` lines -> chunk_type="list" |
| Paragraphs | Accumulated to token budget, chunk_type="paragraph" |
| YAML front matter | Stripped |
| Title | First H1 or filename stem |
| Embed text format | `instruction_prefix + filename[: heading_context] + "\n\n" + content` |
| Min chunk | < 50 chars skipped |
| Token estimation | `len(text) / 4` |
| Default chunk size | 256 tokens |
| **walker_test.go** | |
| allowed_dirs | Directory outside whitelist -> error |
| File extensions | Only configured extensions included |
| Excluded dirs | node_modules, vendor, .git, __pycache__ skipped |
| Max file size | Oversized files skipped |
| .ragignore | Patterns applied correctly |
| Symlinks | Not followed; real path verified under allowed dir |

### 1.11 `internal/tools`

| Test Area | What to Assert |
|-----------|---------------|
| search_documents | Limit 1-20 (default 5); per-tool auth checked; empty DB returns `{results: [], total_chunks_in_db: 0}` |
| query_data | SQL validation called; limit enforcement; error sanitization; per-tool auth |
| ingest_documents | Group auth enforced (AUTH-11); allowed_dirs validated; drop=true logged at warn |
| Tool logging | All tool calls logged at info: name, duration, success/error, user sub (OBS-03) |

### 1.12 `internal/server`

| Test Area | What to Assert |
|-----------|---------------|
| GET /healthz | Returns `{"status":"ok"}` without auth; checks DB via dedicated connection |
| POST /mcp | Requires auth (401 without token) |
| Security headers | X-Content-Type-Options: nosniff, Cache-Control: no-store, X-Frame-Options: DENY |
| Panic recovery | Returns 500, logs stack trace |
| Graceful shutdown | Drains in-flight requests within timeout |

### 1.13 `internal/hyde`

| Test Area | What to Assert |
|-----------|---------------|
| AnthropicGenerator success | Returns hypothesis with isPassage=true |
| AnthropicGenerator failure | Returns raw query with isPassage=false (ERR-06) |
| NoopGenerator | Returns raw query with isPassage=false |
| Missing API key | Returns NoopGenerator, logs warning |

### 1.14 `internal/rerank`

| Test Area | What to Assert |
|-----------|---------------|
| Successful rerank | Rescored results sorted descending |
| Missing indices | Score 0.0 |
| HTTP timeout | 30s |
| Response body limit | 4 MiB |
| Failure | Returns nil (caller falls back to RRF) |

---

## 2. Integration Test Strategy

Gated by `TEST_DATABASE_URL` environment variable. Run via `make test-integration` (`go test ./... -count=1`).

### 2.1 Prerequisites

- PostgreSQL 14+ with pgvector extension
- `TEST_DATABASE_URL` set to a test database connection string
- Test database is disposable (tests may create/drop tables)

### 2.2 Integration Test Scope

| Package | Tests |
|---------|-------|
| `internal/database/postgres` | Connect, Ping, PingDedicated (separate connection), ApplySchema idempotent (run twice), Close |
| `internal/vectorstore` | Full cycle: insert document -> insert chunks with embeddings -> SearchKNN -> SearchFTS -> verify results |
| `internal/querystore/postgres` | ExecuteReadOnly with read-only transaction; timeout enforcement; parameterized queries |
| `internal/ingest` | End-to-end: walk directory -> chunk -> embed (mock) -> store -> verify idempotency (re-ingest unchanged files = no-op) |
| `internal/server` | HTTP round-trip: health endpoint against real DB; MCP endpoint with mock auth |

### 2.3 Test Isolation

- Each integration test creates its own schema or uses a unique table prefix
- Tests clean up after themselves (DROP tables in teardown)
- Tests must not depend on execution order

---

## 3. Test Phases Aligned with Implementation

Each phase must have all tests passing before proceeding to the next.

| Phase | Package(s) | Key Test Focus |
|-------|-----------|---------------|
| 1 | config | TOML loading, all validation rules, reload |
| 2 | vecmath, engine | L2 normalize, dot product, engine detection with injectable lookPath |
| 3 | database (interface, postgres, mssql) | Schema DDL, pool config, integration: connect/ping/schema |
| 4 | auth | JWT validation (all claims, nbf, singleflight, rate limit), authorizer, middleware |
| 5 | embed | Client with prefix selection, L2 normalization, batch, timeouts |
| 6 | vectorstore | Insert/search cycle, IVFFlat index creation, integration tests |
| 7 | querystore | SQL safety (keyword blocking, comment stripping, semicolons), read-only execution, error sanitization |
| 8 | guardrails, hyde, rerank, search | Guardrail scores, HyDE fallback, reranker fallback, RRF merge, full pipeline |
| 9 | ingest | Chunker (all types + heading context), walker (allowed_dirs, symlinks, ragignore), pipeline idempotency |
| 10 | tools | search_documents, query_data, ingest_documents: authorization, parameter validation, logging |
| 11 | server | Health endpoint, auth enforcement, security headers, panic recovery, graceful shutdown |
| 12 | cmd/server | CLI subcommands compile and dispatch correctly |
| 13 | Makefile, containers | `make build` succeeds, `make test` passes, scripts are executable |
| 14 | Full integration | `make test` all pass, `make build` clean, `validate` subcommand exits 0 with example config |

---

## 4. Edge Cases and Failure Scenarios (ERR-01..20)

| ID | Scenario | Expected Behavior | Test Location |
|----|----------|-------------------|---------------|
| ERR-01 | Config validation failure | Exit 1 with descriptive error | config_test.go |
| ERR-02 | DB unreachable at startup | Exit 1 (no retry) | cmd/server or server_test.go (mock DB that refuses connections) |
| ERR-03 | JWKS fetch failure at startup | Exit 1 | cognito_test.go (HTTP server returning 500) |
| ERR-04 | JWKS refetch failure at runtime | Warn log, continue with cached keys | cognito_test.go (initial success, subsequent failure) |
| ERR-05 | Embed server unreachable | MCP tool error returned, no crash | embed client_test.go + tools/search_test.go |
| ERR-06 | HyDE generation failure | Warn log, fall back to raw query | hyde_test.go (mock API returning error) |
| ERR-07 | Reranker failure | Warn log, fall back to RRF ordering | client_test.go (mock returning error) |
| ERR-08 | Bundled llama-server fails to start | Exit 1 after 60s timeout | entrypoint.sh (manual verification) |
| ERR-09 | L1 guardrail rejection | MCP result: "off_topic", info log | guardrails_test.go |
| ERR-10 | L2 guardrail rejection | MCP result: "below_threshold", info log, score at debug | guardrails_test.go |
| ERR-11 | SQL timeout | MCP error: "query timeout" | querystore query_test.go (context with short deadline) |
| ERR-12 | SQL syntax error | DB syntax error returned to client | querystore query_test.go |
| ERR-13 | SQL other error | Generic "query execution failed" to client, details in server log | querystore query_test.go |
| ERR-14 | SQL blocked keyword | MCP error: "query contains disallowed keywords" | safety_test.go |
| ERR-15 | Ingestion per-file error | Warn, skip file, continue with remaining | ingest pipeline_test.go (one bad file among good) |
| ERR-16 | Ingestion embed unreachable | Halt ingest, return partial results | ingest pipeline_test.go (embed mock fails mid-batch) |
| ERR-17 | Ingestion dir not in allowed_dirs | Error immediately | walker_test.go |
| ERR-18 | Unexpected panic in handler | Recover, log stack trace, return 500 | server_test.go (handler that panics) |
| ERR-19 | SIGHUP invalid config | Log error, keep previous config | config_test.go (reload with invalid values) |
| ERR-20 | SIGTERM/SIGINT | Graceful shutdown within configurable timeout | server_test.go (cancel context, verify drain) |

---

## 5. Makefile Targets

| Target | Command | Purpose |
|--------|---------|---------|
| `test` | `go test ./... -short -count=1` | Unit tests only |
| `test-integration` | `go test ./... -count=1` | Unit + integration (requires TEST_DATABASE_URL) |
| `test-coverage` | `go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out` | HTML coverage report |
| `govulncheck` | `govulncheck ./...` | Dependency vulnerability scan |

---

## 6. Test Conventions

- All DB access through interfaces; unit tests use mocks, never real databases
- Auth middleware injectable; unit tests bypass auth or use test JWT fixtures
- Container engine detection uses injectable `lookPath` -- no real PATH dependency in tests
- Test JWT key pairs generated with `lestrrat-go/jwx/v2` at test time
- Integration tests skip with `t.Skip` when `TEST_DATABASE_URL` is unset
- No test stubs or skips in unit tests -- all assertions must execute
- Tests use `t.Parallel()` where safe to verify concurrency correctness
