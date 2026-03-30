# MCP Authenticated Server Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-ready, fork-friendly Go MCP server with Cognito JWT auth, dual-engine database (PostgreSQL+pgvector / MSSQL), vector search, SQL querying, document ingestion, guardrails, HyDE, reranking, and container orchestration.

**Architecture:** Single Go binary (`cmd/server/main.go`) wiring internal packages following a strict downward dependency DAG. Leaf packages (vecmath, config, engine) have no internal dependencies. Mid-tier packages (embed, auth, database, querystore, vectorstore, guardrails, hyde, rerank, ingest, search) depend only on leaves or each other per the DAG. Top-tier packages (tools, server) orchestrate everything. All database access through interfaces; all auth injectable.

**Tech Stack:** Go 1.23+, pgx/v5, pgvector-go, go-mssqldb, lestrrat-go/jwx/v2, BurntSushi/toml, anthropic-sdk-go, modelcontextprotocol/go-sdk, log/slog

**Spec:** `/mcp-authenticated-server/REQUIREMENTS.md` (authoritative), `docs/DESIGN.md`, `docs/FUNCTIONAL-REQUIREMENTS.md`, `docs/NON-FUNCTIONAL-REQUIREMENTS.md`

---

## Phase 1: Project Foundation

### Task 1.1: Go Module and Dependencies

**Files:**
- Create: `go.mod`
- Create: `go.sum` (auto-generated)

- [ ] **Step 1: Initialize Go module**

```bash
cd /mcp-authenticated-server && go mod init github.com/emergingrobotics/mcp-authenticated-server
```

Ensure `go.mod` says `go 1.23`.

- [ ] **Step 2: Add all required dependencies**

```bash
go get github.com/modelcontextprotocol/go-sdk@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/pgvector/pgvector-go@latest
go get github.com/microsoft/go-mssqldb@latest
go get github.com/lestrrat-go/jwx/v2@latest
go get github.com/BurntSushi/toml@latest
go get github.com/anthropics/anthropic-sdk-go@latest
go get github.com/bmatcuk/doublestar/v4@latest
```

- [ ] **Step 3: Verify go.mod is correct**

```bash
cat go.mod
```

Expected: module path, go 1.23, all dependencies listed.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "Initialize Go module with all required dependencies."
```

### Task 1.2: Gitignore

**Files:**
- Create: `.gitignore`

- [ ] **Step 1: Create .gitignore**

```gitignore
# Secrets and config
.env
.envrc
config.toml

# Build artifacts
bin/
*.exe

# Editor backups
*~

# Go
vendor/

# Models (large binary files)
models/*.gguf

# OS
.DS_Store

# LLM working files
.llm/
```

- [ ] **Step 2: Commit**

```bash
git add .gitignore
git commit -m "Add .gitignore with mandatory entries and Go conventions."
```

### Task 1.3: Config Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `config.toml.example`

- [ ] **Step 1: Write config struct and validation tests**

Create `internal/config/config_test.go` with tests for:
- Loading valid TOML config
- Missing required fields (region, user_pool_id, client_id)
- Score range validation: `min_topic_score` and `min_match_score` must be [0.0, 1.0]
- URL scheme validation: embed.host, reranker.host, hyde.base_url must be http/https
- Cross-field: `corpus_topic` non-empty requires `embed.enabled=true` (GUARD-08)
- Cross-field: `tls_cert` and `tls_key` must both be set or both empty
- Cross-field: `ingest.allowed_dirs` non-empty when embed.enabled=true
- Timeout max: query.timeout <= 5m
- Token use: must be "access" or "id"
- Database engine: must be "postgres" or "mssql"
- Config file mode 0600 check

The `Config` struct must match the TOML structure exactly from section 5.1 of REQUIREMENTS.md:

```go
type Config struct {
    Runtime    RuntimeConfig    `toml:"runtime"`
    Server     ServerConfig     `toml:"server"`
    Database   DatabaseConfig   `toml:"database"`
    Auth       AuthConfig       `toml:"auth"`
    Embed      EmbedConfig      `toml:"embed"`
    Search     SearchConfig     `toml:"search"`
    Reranker   RerankerConfig   `toml:"reranker"`
    Guardrails GuardrailsConfig `toml:"guardrails"`
    Hyde       HydeConfig       `toml:"hyde"`
    Ingest     IngestConfig     `toml:"ingest"`
    Query      QueryConfig      `toml:"query"`
}
```

Each sub-struct maps to the TOML sections in REQUIREMENTS.md section 5.1.

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /mcp-authenticated-server && go test ./internal/config/ -v -count=1
```

Expected: FAIL (config.go doesn't exist yet)

- [ ] **Step 3: Implement config.go**

Implement `Load(path string) (*Config, error)`:
1. Read TOML file with `BurntSushi/toml`
2. Overlay env vars: `DATABASE_URL` -> `Config.Database.URL`, `ANTHROPIC_API_KEY` -> stored but not in config struct (use `os.Getenv` at point of use)
3. Call `Validate()` which checks all rules from CFG-03
4. Parse duration strings (`conn_max_lifetime`, `timeout`, `max_file_size`) into Go types
5. Return validated config

Key validation rules (CFG-03):
- Score ranges [0.0, 1.0] for guardrails
- URL scheme validation (http/https) for embed.host, reranker.host, hyde.base_url
- Non-empty required: auth.region, auth.user_pool_id, auth.client_id
- timeout max 5m for query.timeout
- tls_cert/tls_key both-or-neither
- corpus_topic requires embed.enabled=true
- ingest.allowed_dirs non-empty when embed.enabled
- database.engine must be "postgres" or "mssql"
- auth.token_use must be "access" or "id"

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/ -v -count=1
```

Expected: ALL PASS

- [ ] **Step 5: Create config.toml.example**

Copy the exact TOML structure from REQUIREMENTS.md section 5.1 with all comment documentation. No real credentials.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go config.toml.example
git commit -m "Add config package with TOML loading, validation, and example config."
```

### Task 1.4: Config Reload (SIGHUP)

**Files:**
- Create: `internal/config/reload.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write reload tests**

Test that:
- `ReloadableConfig` contains only reloadable fields (search, guardrails, hyde except base_url, query, log_level)
- `ExtractReloadable(*Config)` extracts the reloadable subset
- `ApplyReload(current *Config, reloaded *Config)` only changes reloadable fields
- Invalid config on reload returns error (caller keeps old config)
- Changed sections are reported for logging

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/ -v -run TestReload -count=1
```

- [ ] **Step 3: Implement reload.go**

Implement:
- `ReloadableConfig` struct with reloadable fields
- `ExtractReloadable(cfg *Config) ReloadableConfig`
- `ApplyReload(current *Config, reloaded ReloadableConfig) []string` returns list of changed section names
- Reloadable fields per CFG-05: `[search]`, `[guardrails]`, `[hyde]` (except `base_url`), `[query]`, `[server].log_level`
- Non-reloadable (require restart): `[runtime]`, `[server].port`, `[server].tls_*`, `[database]`, `[auth]`, `[embed]`, `[reranker].host`, `[hyde].base_url`, `[ingest]`

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/ -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/config/reload.go internal/config/config_test.go
git commit -m "Add SIGHUP config reload with reloadable field extraction."
```

---

## Phase 2: Leaf Packages

### Task 2.1: vecmath Package

**Files:**
- Create: `internal/vecmath/vecmath.go`
- Create: `internal/vecmath/vecmath_test.go`

- [ ] **Step 1: Write vecmath tests**

Tests for:
- `L2Normalize(v []float32)` normalizes in-place; zero vector returns zero vector
- `DotProduct(a, b []float32) float32` computes dot product
- `CosineSimilarity(a, b []float32) float32` — for L2-normalized vectors equals dot product
- Panics or errors on mismatched lengths
- Normalized vector has magnitude ~1.0

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/vecmath/ -v -count=1
```

- [ ] **Step 3: Implement vecmath.go**

```go
package vecmath

import "math"

// L2Normalize normalizes the vector in-place to unit length.
func L2Normalize(v []float32) {
    var sum float64
    for _, x := range v {
        sum += float64(x) * float64(x)
    }
    norm := float32(math.Sqrt(sum))
    if norm == 0 {
        return
    }
    for i := range v {
        v[i] /= norm
    }
}

// DotProduct computes the dot product of two vectors.
func DotProduct(a, b []float32) float32 {
    if len(a) != len(b) {
        panic("vecmath: mismatched vector lengths")
    }
    var sum float32
    for i := range a {
        sum += a[i] * b[i]
    }
    return sum
}

// CosineSimilarity computes cosine similarity. For L2-normalized vectors, this equals DotProduct.
func CosineSimilarity(a, b []float32) float32 {
    return DotProduct(a, b)
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/vecmath/ -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/vecmath/vecmath.go internal/vecmath/vecmath_test.go
git commit -m "Add vecmath package with L2 normalize, dot product, cosine similarity."
```

### Task 2.2: Container Engine Package

**Files:**
- Create: `internal/engine/engine.go`
- Create: `internal/engine/engine_test.go`

- [ ] **Step 1: Write engine tests**

Tests with injectable `lookPath func(string) (string, error)`:
- Podman found in PATH -> selects podman
- Podman not found, Docker found -> selects docker
- Neither found -> error
- CLI flag override -> uses specified engine
- Config override -> uses specified engine
- Priority: flag > config > PATH detection
- `ComposeCmd` returns correct args for podman vs docker
- `ProjectCmd` includes project name, env file, compose files
- `RunCmd`, `BuildCmd` return correct arg slices
- Host gateway: Docker uses `--add-host host-gateway:host-gateway`, Podman uses native
- Docker compose plugin preferred over standalone docker-compose

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/engine/ -v -count=1
```

- [ ] **Step 3: Implement engine.go**

Key types and functions:
```go
type Engine struct {
    Name     string // "podman" or "docker"
    Path     string // absolute path to binary
}

type Options struct {
    CLIFlag    string
    ConfigVal  string
    LookPath   func(string) (string, error) // injectable for testing
}

func Detect(opts Options) (*Engine, error)
func (e *Engine) ComposeCmd(args ...string) []string
func (e *Engine) ProjectCmd(project string, envFile string, composeFiles []string, args ...string) []string
func (e *Engine) RunCmd(args ...string) []string
func (e *Engine) BuildCmd(args ...string) []string
func (e *Engine) ImageExistsCmd(image string) []string
func (e *Engine) NetworkCreateCmd(name string) []string
func (e *Engine) NetworkExistsCmd(name string) []string
func (e *Engine) InspectHealthCmd(container string) []string
func (e *Engine) HostGatewayArgs() []string
```

Requirements: ENG-01 through ENG-10. All commands as explicit `[]string` argv — never `sh -c`.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/engine/ -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/engine/engine.go internal/engine/engine_test.go
git commit -m "Add container engine detection with podman preference and injectable lookPath."
```

---

## Phase 3: Database Layer

### Task 3.1: Database Interface

**Files:**
- Create: `internal/database/interface.go`

- [ ] **Step 1: Define Store interface**

```go
package database

import (
    "context"
    "database/sql"
)

// Store abstracts database operations across PostgreSQL and MSSQL.
type Store interface {
    Connect(ctx context.Context, dsn string) error
    Ping(ctx context.Context) error
    PingDedicated(ctx context.Context, dsn string) error // DB-07: health check on dedicated connection
    Close() error
    ApplySchema(ctx context.Context) error
    Pool() *sql.DB
    Engine() string // "postgres" or "mssql"
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/database/interface.go
git commit -m "Define database Store interface for dual-engine abstraction."
```

### Task 3.2: PostgreSQL Store

**Files:**
- Create: `internal/database/postgres/store.go`
- Create: `internal/database/postgres/schema.go`
- Create: `internal/database/postgres/store_test.go`

- [ ] **Step 1: Write PostgreSQL store tests**

Unit tests (no real DB):
- `ApplySchema` generates correct DDL (documents, chunks, build_metadata tables)
- Connection pool settings applied correctly (max_open, max_idle, lifetime)
- `Engine()` returns `"postgres"`

Integration tests (gated by `TEST_DATABASE_URL`):
- Connect, Ping, ApplySchema idempotent, Close
- PingDedicated uses separate connection

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/database/postgres/ -v -count=1 -short
```

- [ ] **Step 3: Implement store.go**

PostgreSQL store using `pgx/v5/stdlib` for `database/sql` compatibility:
- `Connect`: parse DSN, configure pool (DB-06: max_open=10, max_idle=5, lifetime=5m configurable)
- `Ping`: standard pool ping
- `PingDedicated`: open a new one-off connection, ping, close (DB-07)
- `Pool`: return `*sql.DB`
- `Close`: close pool

- [ ] **Step 4: Implement schema.go**

DDL for PostgreSQL per section 4.4.1:
- `documents` table: id BIGSERIAL PK, source_path TEXT UNIQUE NOT NULL, title TEXT, content TEXT NOT NULL, content_hash TEXT NOT NULL, token_count INTEGER, created_at TIMESTAMPTZ DEFAULT NOW()
- `chunks` table: id BIGSERIAL PK, document_id BIGINT FK REFERENCES documents(id) ON DELETE CASCADE, chunk_index INTEGER NOT NULL, content TEXT NOT NULL, token_count INTEGER, heading_context TEXT, chunk_type TEXT, embedding vector(%d), content_fts tsvector GENERATED ALWAYS AS (to_tsvector('english', content)) STORED, created_at TIMESTAMPTZ DEFAULT NOW(), UNIQUE(document_id, chunk_index)
- `build_metadata` table: key TEXT PK, value TEXT NOT NULL
- GIN index on content_fts
- All `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`
- `CREATE EXTENSION IF NOT EXISTS vector` at start

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/database/postgres/ -v -count=1 -short
```

- [ ] **Step 6: Commit**

```bash
git add internal/database/postgres/store.go internal/database/postgres/schema.go internal/database/postgres/store_test.go
git commit -m "Add PostgreSQL store with pgx driver, schema DDL, and connection pooling."
```

### Task 3.3: MSSQL Store

**Files:**
- Create: `internal/database/mssql/store.go`
- Create: `internal/database/mssql/schema.go`
- Create: `internal/database/mssql/store_test.go`

- [ ] **Step 1: Write MSSQL store tests**

Unit tests:
- Schema DDL uses T-SQL equivalents (BIGINT IDENTITY, NVARCHAR(MAX), DATETIMEOFFSET) per MSSQL-02
- `Engine()` returns `"mssql"`
- Connection pool settings applied

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/database/mssql/ -v -count=1 -short
```

- [ ] **Step 3: Implement store.go and schema.go**

MSSQL store using `go-mssqldb`:
- Same interface as PostgreSQL store
- Schema: T-SQL DDL per MSSQL-02 (no vector columns, no pgvector extension)
- Only `build_metadata` table (chunks/documents only for PostgreSQL+vector)
- `PingDedicated`: separate connection for health check

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/database/mssql/ -v -count=1 -short
```

- [ ] **Step 5: Commit**

```bash
git add internal/database/mssql/store.go internal/database/mssql/schema.go internal/database/mssql/store_test.go
git commit -m "Add MSSQL store with T-SQL schema and go-mssqldb driver."
```

---

## Phase 4: Auth Package

### Task 4.1: Auth Context Helpers

**Files:**
- Create: `internal/auth/context.go`

- [ ] **Step 1: Implement context helpers**

```go
package auth

import "context"

type contextKey int

const tokenKey contextKey = iota

type Claims struct {
    Subject string
    Email   string
    Groups  []string
    Scope   string
    Raw     map[string]interface{}
}

func WithClaims(ctx context.Context, c *Claims) context.Context
func ClaimsFromContext(ctx context.Context) *Claims
func SubjectFromContext(ctx context.Context) string
func GroupsFromContext(ctx context.Context) []string
```

- [ ] **Step 2: Commit**

```bash
git add internal/auth/context.go
git commit -m "Add auth context helpers for storing and retrieving JWT claims."
```

### Task 4.2: Authorizer Interface and Group-Based Implementation

**Files:**
- Create: `internal/auth/authorizer.go`
- Create: `internal/auth/authorizer_test.go`

- [ ] **Step 1: Write authorizer tests**

Tests for:
- No required groups -> authorized
- User has required group -> authorized
- User lacks required group -> not authorized (403)
- `ingest_documents` requires explicit group by default (AUTH-11)
- Server-wide `allowed_groups` check (AUTH-09)
- Empty `allowed_groups` -> no server-wide restriction

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/auth/ -v -run TestAuthorizer -count=1
```

- [ ] **Step 3: Implement authorizer.go**

```go
type Authorizer interface {
    Authorize(ctx context.Context, toolName string) error
}

type GroupAuthorizer struct {
    AllowedGroups map[string][]string // tool name -> required groups
    ServerGroups  []string            // server-wide allowed_groups
}

func (a *GroupAuthorizer) Authorize(ctx context.Context, toolName string) error
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/auth/ -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/auth/authorizer.go internal/auth/authorizer_test.go
git commit -m "Add Authorizer interface with group-based implementation."
```

### Task 4.3: Cognito JWT Validation

**Files:**
- Create: `internal/auth/cognito.go`
- Create: `internal/auth/cognito_test.go`

- [ ] **Step 1: Write JWT validation tests**

Tests for (AUTH-01 through AUTH-10):
- Valid token passes all checks
- Expired token rejected (exp)
- Future nbf rejected
- Wrong issuer rejected
- Wrong audience/client_id rejected (depends on token_use)
- Invalid signature rejected
- Unknown kid triggers JWKS refetch (singleflight)
- JWKS refetch rate limited to 1/60s (AUTH-04)
- JWKS fetch timeout of 10s (AUTH-05)
- JWKS refetch failure retains cached keys (AUTH-05)
- Claims extracted correctly: sub, email, cognito:groups, scope (AUTH-06)
- token_use mismatch rejected (AUTH-03e)

Use test JWKS/JWT generation with `lestrrat-go/jwx/v2` for test key pairs.

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/auth/ -v -run TestCognito -count=1
```

- [ ] **Step 3: Implement cognito.go**

Key components:
```go
type CognitoValidator struct {
    issuer     string
    clientID   string
    tokenUse   string
    jwksURL    string
    keyCache   jwk.Cache    // lestrrat-go/jwx/v2 auto-refresh cache
    sfGroup    singleflight.Group
    lastFetch  atomic.Int64 // unix timestamp of last JWKS fetch
    minRefresh int64        // 60 seconds rate limit
}

func NewCognitoValidator(region, userPoolID, clientID, tokenUse string) (*CognitoValidator, error)
func (v *CognitoValidator) Validate(tokenString string) (*Claims, error)
func (v *CognitoValidator) FetchJWKS(ctx context.Context) error
```

JWKS URL: `https://cognito-idp.{region}.amazonaws.com/{userPoolId}/.well-known/jwks.json`
Issuer: `https://cognito-idp.{region}.amazonaws.com/{userPoolId}`

Validation order: parse JWT header -> extract kid -> lookup key (refetch if unknown, singleflight, rate-limited) -> verify signature -> verify iss -> verify aud/client_id per token_use -> verify exp -> verify nbf (if present) -> verify token_use claim -> extract claims

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/auth/ -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/auth/cognito.go internal/auth/cognito_test.go
git commit -m "Add Cognito JWT validation with JWKS caching and singleflight refetch."
```

### Task 4.4: Auth Middleware

**Files:**
- Create: `internal/auth/middleware.go`

- [ ] **Step 1: Implement HTTP middleware**

```go
func Middleware(validator *CognitoValidator, authorizer Authorizer) func(http.Handler) http.Handler
```

- Extract `Authorization: Bearer <token>` header
- Validate token via CognitoValidator
- Store claims in context via `WithClaims`
- On failure: 401 JSON response, log reason at warn (never log the token — AUTH-10)
- Check server-wide `allowed_groups` (AUTH-09) -> 403 if fails

- [ ] **Step 2: Commit**

```bash
git add internal/auth/middleware.go
git commit -m "Add auth HTTP middleware for JWT Bearer token validation."
```

---

## Phase 5: Embed Client

### Task 5.1: Embed Interface and Client

**Files:**
- Create: `internal/embed/interface.go`
- Create: `internal/embed/client.go`
- Create: `internal/embed/client_test.go`

- [ ] **Step 1: Write embed client tests**

Tests for:
- Successful embedding request returns normalized vectors
- Batch embedding (multiple inputs)
- HTTP timeout handling
- Response body size limit (4 MiB — SEC-04)
- Error response handling
- Model name sent in request body (VEC-05)
- Query prefix and passage prefix applied correctly (ENH-11)

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/embed/ -v -count=1
```

- [ ] **Step 3: Implement interface.go and client.go**

```go
// interface.go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    EmbedWithPrefix(ctx context.Context, texts []string, prefix string) ([][]float32, error)
}

// client.go
type Client struct {
    host          string
    model         string
    queryPrefix   string
    passagePrefix string
    httpClient    *http.Client // bounded timeout
}

func NewClient(host, model, queryPrefix, passagePrefix string) *Client
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error)
func (c *Client) EmbedWithPrefix(ctx context.Context, texts []string, prefix string) ([][]float32, error)
```

Request format: `POST /v1/embeddings` with `{"model": "...", "input": ["..."]}`.
Response: `{"data": [{"embedding": [...]}]}`.
All returned embeddings L2-normalized via vecmath.L2Normalize (VEC-06).
Response body limited to 4 MiB (SEC-04).

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/embed/ -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/embed/interface.go internal/embed/client.go internal/embed/client_test.go
git commit -m "Add OpenAI-compatible embed client with L2 normalization and prefix support."
```

---

## Phase 6: Vector Store

### Task 6.1: VectorStore Interface and PostgreSQL Implementation

**Files:**
- Create: `internal/vectorstore/interface.go`
- Create: `internal/vectorstore/postgres.go`
- Create: `internal/vectorstore/postgres_test.go`

- [ ] **Step 1: Write vectorstore tests**

Unit tests with mock DB:
- `InsertDocument` creates document row, returns ID
- `InsertChunks` batch inserts with embeddings
- `DeleteChunksByDocumentID` removes chunks
- `SearchKNN` returns ranked results by cosine distance
- `SearchFTS` returns ranked results by text search
- `GetDocumentByPath` for idempotency checks
- `UpdateDocument` for changed files
- `DropAndRecreateTables` for --drop mode
- `CreateIVFFlatIndex` when chunk count >= 100
- `SetIVFFlatProbes` only when index exists
- `GetChunkCount` returns total chunks
- `WriteBuildMetadata` stores key-value pairs
- `GetTotalChunksInDB` for search results

Integration tests (gated by TEST_DATABASE_URL):
- Full insert/search cycle

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/vectorstore/ -v -count=1 -short
```

- [ ] **Step 3: Implement interface.go**

```go
type Chunk struct {
    DocumentID     int64
    ChunkIndex     int
    Content        string
    TokenCount     int
    HeadingContext string
    ChunkType      string
    Embedding      []float32
}

type ChunkResult struct {
    Content        string
    HeadingContext string
    ChunkType      string
    SourcePath     string
    Title          string
    Score          float64
}

type Document struct {
    ID          int64
    SourcePath  string
    Title       string
    Content     string
    ContentHash string
    TokenCount  int
}

type VectorStore interface {
    InsertDocument(ctx context.Context, doc *Document) (int64, error)
    GetDocumentByPath(ctx context.Context, path string) (*Document, error)
    UpdateDocument(ctx context.Context, doc *Document) error
    DeleteChunksByDocumentID(ctx context.Context, docID int64) error
    InsertChunks(ctx context.Context, chunks []Chunk) error
    SearchKNN(ctx context.Context, embedding []float32, limit int) ([]ChunkResult, error)
    SearchFTS(ctx context.Context, query string, limit int) ([]ChunkResult, error)
    GetChunkCount(ctx context.Context) (int, error)
    DropAndRecreateTables(ctx context.Context, dimension int) error
    CreateIVFFlatIndex(ctx context.Context) error
    SetIVFFlatProbes(ctx context.Context, probes int) error
    WriteBuildMetadata(ctx context.Context, metadata map[string]string) error
}
```

- [ ] **Step 4: Implement postgres.go**

PostgreSQL implementation using pgx pool directly (via `database.Store.Pool()`):
- `SearchKNN`: `SELECT ... ORDER BY embedding <=> $1 LIMIT $2` (VEC-07a)
- `SearchFTS`: `SELECT ... WHERE content_fts @@ plainto_tsquery('english', $1) ORDER BY ts_rank(content_fts, ...) DESC LIMIT $2` (VEC-07b)
- `InsertChunks`: batch insert with pgvector embedding type
- `CreateIVFFlatIndex`: `CREATE INDEX idx_chunks_embedding ON chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = ...)` where lists = max(10, floor(sqrt(count)))
- All SQL uses parameterized queries (SEC-16)

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/vectorstore/ -v -count=1 -short
```

- [ ] **Step 6: Commit**

```bash
git add internal/vectorstore/interface.go internal/vectorstore/postgres.go internal/vectorstore/postgres_test.go
git commit -m "Add VectorStore interface and PostgreSQL implementation with pgvector."
```

---

## Phase 7: Query Store

### Task 7.1: SQL Safety (Keyword Blocking)

**Files:**
- Create: `internal/querystore/safety.go`
- Create: `internal/querystore/safety_test.go`

- [ ] **Step 1: Write SQL safety tests**

Tests for (SQL-06):
- DDL blocked: CREATE, ALTER, DROP
- DML blocked: INSERT, UPDATE, DELETE, TRUNCATE, MERGE, REPLACE
- Admin blocked: GRANT, REVOKE, EXEC, EXECUTE, xp_, sp_executesql, OPENROWSET, OPENDATASOURCE, BULK, COPY, LOAD, CALL
- Transaction control blocked: BEGIN, COMMIT, ROLLBACK, SAVEPOINT
- Session modification blocked: SET, DECLARE
- SELECT INTO blocked
- SQL comments stripped before scanning: `-- comment`, `/* block comment */`
- Semicolons rejected (single statement only)
- Semicolons inside string literals allowed
- Valid SELECT queries pass
- Case-insensitive matching
- Keywords at word boundaries (not inside words like "DESCRIPTION")

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/querystore/ -v -run TestSafety -count=1
```

- [ ] **Step 3: Implement safety.go**

```go
func ValidateQuery(query string) error
func StripComments(query string) string
func ContainsBlockedKeyword(query string) (string, bool)
func HasMultipleStatements(query string) bool
```

Strip `--` line comments and `/* */` block comments first, then scan for blocked keywords at word boundaries, then check for semicolons outside string literals.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/querystore/ -v -run TestSafety -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/querystore/safety.go internal/querystore/safety_test.go
git commit -m "Add SQL query safety validation with keyword blocking and comment stripping."
```

### Task 7.2: QueryStore Interface and Implementations

**Files:**
- Create: `internal/querystore/interface.go`
- Create: `internal/querystore/postgres.go`
- Create: `internal/querystore/mssql.go`
- Create: `internal/querystore/query_test.go`

- [ ] **Step 1: Write querystore tests**

Tests for:
- Result format: `{columns, rows, row_count, truncated}` (SQL-05)
- Row limit enforcement (default 100, max 1000) (SQL-02)
- Response size cap 10 MiB with truncation flag (SQL-05)
- Query timeout enforcement (SQL-04)
- Read-only transaction for PostgreSQL (SQL-03)
- Error sanitization: generic message to client (SQL-07)
- Syntax errors returned as-is (SQL-07)
- Bind parameters passed through (SQL-02)

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/querystore/ -v -run TestQuery -count=1
```

- [ ] **Step 3: Implement interface.go, postgres.go, mssql.go**

```go
// interface.go
type QueryResult struct {
    Columns   []string        `json:"columns"`
    Rows      [][]interface{} `json:"rows"`
    RowCount  int             `json:"row_count"`
    Truncated bool            `json:"truncated"`
}

type QueryStore interface {
    ExecuteReadOnly(ctx context.Context, query string, params []interface{}, limit int, timeout time.Duration) (*QueryResult, error)
}
```

PostgreSQL: wraps in `SET TRANSACTION READ ONLY` (SQL-03).
MSSQL: relies on read-only DB user (DB-09) + keyword blocking.
Both: enforce timeout via context deadline, limit rows, cap response at 10 MiB.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/querystore/ -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/querystore/interface.go internal/querystore/postgres.go internal/querystore/mssql.go internal/querystore/query_test.go
git commit -m "Add QueryStore interface with PostgreSQL and MSSQL read-only query execution."
```

---

## Phase 8: Search Pipeline Components

### Task 8.1: Guardrails

**Files:**
- Create: `internal/guardrails/guardrails.go`
- Create: `internal/guardrails/topic.go`
- Create: `internal/guardrails/guardrails_test.go`

- [ ] **Step 1: Write guardrails tests**

Tests for (GUARD-01 through GUARD-08):
- Level 1: query embedding below topic threshold -> "off_topic" error
- Level 1: query embedding above threshold -> passes
- Level 1: disabled (empty corpus_topic) -> skipped with zero overhead
- Level 2: best score below min_match_score -> "below_threshold" error
- Level 2: best score above threshold -> passes
- Level 2: disabled (min_match_score=0) -> skipped
- Both disabled -> no checks performed
- Topic vector computed once and reused (PERF-05)

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/guardrails/ -v -count=1
```

- [ ] **Step 3: Implement guardrails.go and topic.go**

```go
// topic.go
type TopicGuard struct {
    topicVector   []float32
    minScore      float32
}

func NewTopicGuard(topicEmbedding []float32, minScore float32) *TopicGuard
func (g *TopicGuard) Check(queryEmbedding []float32) error

// guardrails.go
type Guardrails struct {
    topic      *TopicGuard // nil if disabled
    minMatch   float32     // 0 = disabled
}

func New(topic *TopicGuard, minMatchScore float32) *Guardrails
func (g *Guardrails) CheckTopicRelevance(queryEmbedding []float32) error // Level 1
func (g *Guardrails) CheckMatchScore(bestScore float64) error            // Level 2
func (g *Guardrails) TopicEnabled() bool
func (g *Guardrails) MatchEnabled() bool
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/guardrails/ -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/guardrails/guardrails.go internal/guardrails/topic.go internal/guardrails/guardrails_test.go
git commit -m "Add two-level guardrail system with topic relevance and match score gates."
```

### Task 8.2: HyDE Generator

**Files:**
- Create: `internal/hyde/interface.go`
- Create: `internal/hyde/anthropic.go`
- Create: `internal/hyde/noop.go`

- [ ] **Step 1: Implement HyDE interface and implementations**

```go
// interface.go
type Generator interface {
    Generate(ctx context.Context, query string) (hypothesis string, isPassage bool, err error)
}

// noop.go
type NoopGenerator struct{}
func (n *NoopGenerator) Generate(ctx context.Context, query string) (string, bool, error) {
    return query, false, nil
}

// anthropic.go
type AnthropicGenerator struct {
    client       *anthropic.Client
    model        string
    systemPrompt string
}
func NewAnthropicGenerator(model, systemPrompt, baseURL string) (*AnthropicGenerator, error)
func (g *AnthropicGenerator) Generate(ctx context.Context, query string) (string, bool, error)
```

AnthropicGenerator: calls Claude API (ENH-02), returns hypothesis as passage (isPassage=true). On failure, returns raw query with isPassage=false and logs warning (ENH-05, ERR-06). Default system prompt from ENH-02. Max tokens: 256. Response body limit: 1 MiB (SEC-04).

If `ANTHROPIC_API_KEY` not set at construction time, return NoopGenerator and log warning (ENH-05).

- [ ] **Step 2: Write HyDE tests**

Create `internal/hyde/hyde_test.go` with tests for:
- AnthropicGenerator returns hypothesis with isPassage=true on success
- AnthropicGenerator returns raw query with isPassage=false on API failure (ENH-05)
- NoopGenerator returns raw query with isPassage=false
- Missing ANTHROPIC_API_KEY returns NoopGenerator (ENH-05)

- [ ] **Step 3: Run tests to verify they pass**

```bash
go test ./internal/hyde/ -v -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/hyde/interface.go internal/hyde/anthropic.go internal/hyde/noop.go internal/hyde/hyde_test.go
git commit -m "Add HyDE generator with Anthropic Claude implementation and noop fallback."
```

### Task 8.3: Reranker Client

**Files:**
- Create: `internal/rerank/interface.go`
- Create: `internal/rerank/client.go`
- Create: `internal/rerank/noop.go`
- Create: `internal/rerank/client_test.go`

- [ ] **Step 1: Write reranker tests**

Tests for:
- Successful rerank returns rescored results sorted descending
- Document text includes heading context: `filename[: heading_context] + "\n\n" + content` (ENH-09)
- Missing indices in response get score 0.0
- HTTP timeout 30s (ENH-10)
- Response body limit 4 MiB (ENH-10)
- Error returns nil (caller falls back to RRF) (ERR-07)
- Noop returns nil

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/rerank/ -v -count=1
```

- [ ] **Step 3: Implement reranker**

```go
// interface.go
type RerankResult struct {
    Index          int
    RelevanceScore float64
}

type Reranker interface {
    Rerank(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error)
}

// client.go - POST /rerank with {query, documents, top_n} (ENH-07)
// noop.go - returns nil, nil
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/rerank/ -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/rerank/interface.go internal/rerank/client.go internal/rerank/noop.go internal/rerank/client_test.go
git commit -m "Add reranker client with cross-encoder HTTP interface and noop fallback."
```

### Task 8.4: RRF Merge and Search Pipeline

**Files:**
- Create: `internal/search/rrf.go`
- Create: `internal/search/pipeline.go`
- Create: `internal/search/rrf_test.go`

- [ ] **Step 1: Write RRF merge tests**

Tests for (VEC-08):
- Two result lists merged with correct RRF scores: `score(d) = sum(1/(k + rank_i(d)))`
- Document appearing in both lists gets combined score
- Document in only one list gets single-arm score
- k-constant configurable (default 60)
- Results sorted by merged score descending
- Empty lists handled

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/search/ -v -run TestRRF -count=1
```

- [ ] **Step 3: Implement rrf.go**

```go
type RankedResult struct {
    ChunkResult vectorstore.ChunkResult
    Score       float64
}

func MergeRRF(knnResults, ftsResults []vectorstore.ChunkResult, kConstant int) []RankedResult
```

Merge by chunk ID (document_id + chunk_index or content match). Score formula: `score(d) = sum(1/(k + rank))` where rank is 1-indexed position in each arm.

- [ ] **Step 4: Implement pipeline.go**

Full search pipeline per section 14 of REQUIREMENTS.md:

```go
type Pipeline struct {
    embedder    embed.Embedder
    vectorStore vectorstore.VectorStore
    guardrails  *guardrails.Guardrails
    hyde        hyde.Generator
    reranker    rerank.Reranker
    config      *config.SearchConfig
}

func NewPipeline(deps ...) *Pipeline
func (p *Pipeline) Search(ctx context.Context, query string, limit int) (*SearchResult, error)
```

Pipeline steps (section 14):
1. HyDE expansion (if enabled) -> embedText + prefix selection
2. Embed via embedder with appropriate prefix
3. L2-normalize (already done by embed client, but verify)
4. Level 1 guardrail check (pre-DB)
5. Parallel retrieval: goroutine 1 = KNN, goroutine 2 = FTS (PERF-04)
6. RRF merge
7. Reranking (if enabled)
8. Level 2 guardrail check (post-retrieval)
9. Truncate to limit
10. Return results

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/search/ -v -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/search/rrf.go internal/search/pipeline.go internal/search/rrf_test.go
git commit -m "Add RRF merge and full search pipeline with guardrails, HyDE, and reranking."
```

---

## Phase 9: Document Ingestion

### Task 9.1: Structure-Aware Chunker

**Files:**
- Create: `internal/ingest/chunker.go`
- Create: `internal/ingest/chunker_test.go`

- [ ] **Step 1: Write chunker tests**

Tests for (CHUNK-01 through CHUNK-10):
- Heading stack: H1 sets level 1, H2 under H1 creates breadcrumb "H1 > H2"
- Heading level change: H3 after H2 extends stack; H1 after H3 truncates stack
- Breadcrumb construction: join with " > " delimiter (CHUNK-02)
- Heading change flushes accumulated text as chunk with PREVIOUS breadcrumb (CHUNK-01c)
- Code blocks: fenced ``` are atomic chunks with chunk_type="code" (CHUNK-03)
- Tables: consecutive `|` lines are atomic chunk_type="table" (CHUNK-04)
- Lists: lines starting with `-`, `*`, `N.` are chunk_type="list" (CHUNK-05)
- Paragraphs: accumulated to token budget, chunk_type="paragraph" (CHUNK-06)
- YAML front matter stripped (CHUNK-07)
- Title: first H1 or filename stem (CHUNK-08)
- Embed text: `instruction_prefix + filename[: heading_context] + "\n\n" + content` (CHUNK-09)
- Minimum chunk: 50 characters (ING-06)
- Token estimation: len(text) / 4 (ING-06)
- Default chunk size 256 tokens (ING-06)

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ingest/ -v -run TestChunk -count=1
```

- [ ] **Step 3: Implement chunker.go**

```go
type Chunk struct {
    Content        string
    HeadingContext string
    ChunkType      string // "paragraph", "code", "table", "list"
    TokenCount     int
}

type Chunker interface {
    ChunkFile(path string, content []byte, chunkSize int) (title string, chunks []Chunk, err error)
}

type MarkdownChunker struct{}

func (c *MarkdownChunker) ChunkFile(path string, content []byte, chunkSize int) (string, []Chunk, error)
```

Implementation:
- Parse line by line
- Track heading stack (levels 1-6)
- Detect code fences, table lines, list lines
- Accumulate paragraphs, flush when token budget exceeded
- Strip YAML front matter at start
- Token estimation: `len(text) / 4`

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ingest/ -v -run TestChunk -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/ingest/chunker.go internal/ingest/chunker_test.go
git commit -m "Add structure-aware markdown chunker with heading context and type detection."
```

### Task 9.2: Directory Walker

**Files:**
- Create: `internal/ingest/walker.go`
- Create: `internal/ingest/walker_test.go`

- [ ] **Step 1: Write walker tests**

Tests for (ING-02 through ING-04, ING-13, ING-15):
- Directory validated against allowed_dirs whitelist (ING-02)
- Directory not in allowed_dirs -> error (ING-02)
- File extensions filtered (ING-03)
- Hidden files excluded (except .env.example) (ING-04)
- Excluded directories skipped (node_modules, vendor, .git, __pycache__) (ING-04)
- Files > max_file_size skipped (ING-04)
- `.ragignore` patterns applied (ING-04)
- Non-existent directory -> error (ING-13)
- Empty directory -> zero files, no error (ING-13)
- Symlinks not followed (ING-15)
- Real path verified under allowed directory (ING-15)

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/ingest/ -v -run TestWalk -count=1
```

- [ ] **Step 3: Implement walker.go**

```go
type WalkOptions struct {
    AllowedDirs       []string
    AllowedExtensions []string
    ExcludedDirs      []string
    MaxFileSize       int64
}

type FileEntry struct {
    Path    string
    Content []byte
}

func ValidateDirectory(dir string, allowedDirs []string) error
func Walk(dir string, opts WalkOptions) ([]FileEntry, error)
func LoadRagignore(dir string) ([]string, error)
func MatchesRagignore(path string, patterns []string) bool
```

File reading: use `O_NOFOLLOW` equivalent. After opening, `filepath.EvalSymlinks` and verify real path is under allowed dir. Pattern matching: `doublestar` or `filepath.Match` (bounded runtime, no regex — ING-04).

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/ingest/ -v -run TestWalk -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/ingest/walker.go internal/ingest/walker_test.go
git commit -m "Add directory walker with allowed_dirs validation, ragignore, and symlink protection."
```

### Task 9.3: Ingestion Pipeline

**Files:**
- Create: `internal/ingest/pipeline.go`

- [ ] **Step 1: Implement ingestion pipeline**

```go
type Pipeline struct {
    vectorStore vectorstore.VectorStore
    embedder    embed.Embedder
    chunker     Chunker
    config      *config.IngestConfig
}

type IngestResult struct {
    DocumentsProcessed int
    ChunksCreated      int
    Errors             int
    Duration           time.Duration
}

func NewPipeline(vs vectorstore.VectorStore, emb embed.Embedder, chunker Chunker, cfg *config.IngestConfig) *Pipeline
func (p *Pipeline) Ingest(ctx context.Context, dir string, drop bool) (*IngestResult, error)
```

Pipeline (ING-01 through ING-15):
1. Validate directory against allowed_dirs
2. If drop: drop and recreate tables (ING-09)
3. Walk directory for eligible files
4. For each file:
   a. Compute SHA-256 hash (first 16 hex chars) (ING-08)
   b. Check existing document by path — if hash unchanged, skip (ING-08)
   c. Chunk file content
   d. If zero chunks but content exists, create doc row anyway (ING-14)
   e. Embed chunks in batches (ING-07, default 32)
   f. L2-normalize embeddings
   g. Insert/update document + chunks
   h. On per-file error: log warn, skip, continue (ING-11)
5. If embed server unreachable: halt immediately (ING-11)
6. If drop and chunk_count >= 100: create IVFFlat index (ING-10)
7. Write build metadata (ING-12)
8. Return results

- [ ] **Step 2: Commit**

```bash
git add internal/ingest/pipeline.go
git commit -m "Add ingestion pipeline with idempotency, batched embedding, and drop mode."
```

---

## Phase 10: MCP Tools

### Task 10.1: Tool Registry

**Files:**
- Create: `internal/tools/registry.go`

- [ ] **Step 1: Implement tool registry**

Dynamic tool registration (MCP-06):

```go
package tools

import (
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register adds a tool to the MCP server. Fork authors call this to add domain tools.
func Register(server *mcp.Server, name string, description string, schema map[string]interface{}, handler mcp.ToolHandlerFunc)
```

This wraps the go-sdk's tool registration API. The exact API depends on the go-sdk version — consult the library's documentation during implementation.

- [ ] **Step 2: Commit**

```bash
git add internal/tools/registry.go
git commit -m "Add dynamic tool registration helper for MCP server."
```

### Task 10.2: search_documents Tool

**Files:**
- Create: `internal/tools/search.go`
- Create: `internal/tools/search_test.go`

- [ ] **Step 1: Write search tool tests**

Tests for:
- Valid search returns results with correct structure (section 4.6.1)
- Limit enforced (1-20, default 5)
- Per-tool authorization checked
- Empty DB returns `{results: [], total_chunks_in_db: 0}`
- Tool logged at info with: tool name, duration, success/error, user sub (OBS-03)

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tools/ -v -run TestSearch -count=1
```

- [ ] **Step 3: Implement search.go**

Handler that:
1. Extracts claims from context
2. Checks per-tool authorization
3. Validates parameters (query required, limit 1-20 default 5)
4. Delegates to search.Pipeline
5. Returns `{results: [...], total_chunks_in_db: N}`
6. Logs tool call at info level (OBS-03)

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tools/ -v -run TestSearch -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tools/search.go internal/tools/search_test.go
git commit -m "Add search_documents MCP tool with authorization and logging."
```

### Task 10.3: query_data Tool

**Files:**
- Create: `internal/tools/query.go`
- Create: `internal/tools/query_test.go`

- [ ] **Step 1: Write query tool tests**

Tests for:
- Valid query returns `{columns, rows, row_count, truncated}`
- SQL validation called (blocked keywords rejected)
- Limit enforcement (default 100, max 1000)
- Query timeout enforcement
- Error sanitization (generic to client, details logged)
- Per-tool authorization checked
- Tool call logged (OBS-03)

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tools/ -v -run TestQuery -count=1
```

- [ ] **Step 3: Implement query.go**

Handler that:
1. Extracts claims, checks authorization
2. Validates SQL via `querystore.ValidateQuery`
3. Executes via QueryStore.ExecuteReadOnly
4. Sanitizes errors (SQL-07)
5. Returns result structure
6. Logs at info (OBS-03)

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tools/ -v -run TestQuery -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tools/query.go internal/tools/query_test.go
git commit -m "Add query_data MCP tool with SQL safety validation and error sanitization."
```

### Task 10.4: ingest_documents Tool

**Files:**
- Create: `internal/tools/ingest.go`

- [ ] **Step 1: Write ingest tool tests**

Create `internal/tools/ingest_test.go` with tests for:
- Group authorization enforced (AUTH-11) — user without admin group rejected
- Directory validated against allowed_dirs
- `drop=true` logged at warn with user sub (OBS-07)
- Returns correct result structure (section 4.6.3)
- Tool call logged at info (OBS-03)

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/tools/ -v -run TestIngest -count=1
```

- [ ] **Step 3: Implement ingest tool**

Handler that:
1. Requires explicit group authorization by default (AUTH-11)
2. Validates `directory` parameter against allowed_dirs
3. Logs destructive `drop=true` at warn with user sub (OBS-07)
4. Delegates to ingest.Pipeline
5. Returns `{documents_processed, chunks_created, errors, duration_seconds}`

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/tools/ -v -run TestIngest -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/tools/ingest.go internal/tools/ingest_test.go
git commit -m "Add ingest_documents MCP tool with mandatory group authorization."
```

---

## Phase 11: HTTP Server

### Task 11.1: Server Package

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/server_test.go`

- [ ] **Step 1: Write server tests**

Tests for:
- Health endpoint `GET /healthz` returns `{"status":"ok"}` without auth (MCP-04)
- Health endpoint checks DB via dedicated connection (DB-07)
- MCP endpoint `POST /mcp` requires auth (AUTH-01)
- Security headers on all responses: X-Content-Type-Options: nosniff, Cache-Control: no-store, X-Frame-Options: DENY (SEC-14)
- Graceful shutdown on context cancellation (MCP-07)
- Panic recovery returns 500 (ERR-18)
- Concurrent request handling (PERF-01)

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/server/ -v -count=1
```

- [ ] **Step 3: Implement server.go**

```go
type Server struct {
    httpServer *http.Server
    mcpServer  *mcp.Server
    db         database.Store
    config     *config.Config
}

func New(cfg *config.Config, db database.Store, mcpServer *mcp.Server) *Server
func (s *Server) Start(ctx context.Context) error
func (s *Server) Shutdown(ctx context.Context) error
```

HTTP mux:
- `GET /healthz` -> health handler (no auth middleware)
- `POST /mcp` -> auth middleware -> MCP handler

Security headers middleware (SEC-14).
Panic recovery middleware (ERR-18).
Graceful shutdown: stop accepting, drain in-flight (configurable timeout default 15s), close DB (MCP-07).

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/server/ -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go
git commit -m "Add HTTP server with health endpoint, security headers, and graceful shutdown."
```

---

## Phase 12: Main Entry Point and CLI

### Task 12.1: cmd/server/main.go

**Files:**
- Create: `cmd/server/main.go`

- [ ] **Step 1: Implement main.go**

Entry point that:
1. Parses CLI flags: `--config PATH` (default `config.toml`)
2. Dispatches subcommands: `serve` (default), `ingest`, `validate`, `schema` (CLI-01..06)
3. Sets up structured JSON logging via slog (OBS-01)
4. Loads and validates config
5. For `validate`: validate config and exit 0/1 (CLI-03)
6. For `schema`: connect DB, apply schema, exit (CLI-04)
7. For `serve`:
   a. Connect to database
   b. Apply schema
   c. Initialize auth (CognitoValidator, fetch JWKS — exit 1 on failure per ERR-03)
   d. Initialize embed client (if enabled)
   e. Compute topic vector at startup if corpus_topic configured (PERF-05)
   f. Initialize guardrails, HyDE, reranker
   g. Create MCP server, register tools conditionally:
      - `search_documents` + `ingest_documents`: only if postgres + embed.enabled (VEC-02, VEC-03)
      - `query_data`: always
   h. Create HTTP server, start serving
   i. Set up SIGHUP handler for config reload (CFG-05)
   j. Set up SIGTERM/SIGINT handler for graceful shutdown (MCP-07)
   k. Log startup info (OBS-05): version, listen address, DB engine, auth issuer, vector status, guardrail status, HyDE status, reranker status
8. For `ingest`:
   a. Connect DB, apply schema
   b. Initialize embed client
   c. Run ingestion pipeline with CLI flags: `--dir` (repeatable), `--drop`, `--dry-run`, `--verbose` (CLI-06)
   d. Not subject to per-tool auth (CLI-02)
   e. Subject to allowed_dirs validation (CLI-02)
   f. Exit when done

- [ ] **Step 2: Verify it compiles**

```bash
go build -o bin/mcp-server ./cmd/server/
```

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "Add main entry point with CLI subcommands and full dependency wiring."
```

---

## Phase 13: Build Infrastructure

### Task 13.1: Makefile

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Create Makefile**

All required targets from BUILD-04:
- Default target: print available targets
- `build`: `go build -o bin/mcp-server ./cmd/server/`
- `test`: `go test ./... -short -count=1`
- `test-integration`: `go test ./... -count=1` (requires TEST_DATABASE_URL)
- `test-coverage`: `go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out`
- `lint`: `golangci-lint run`
- `govulncheck`: `govulncheck ./...`
- `run`: `go run ./cmd/server/ serve`
- `container-build`: auto-detect engine, build image
- `container-up`: compose up
- `container-down`: compose down
- `container-logs`: compose logs -f
- `ingest`: `go run ./cmd/server/ ingest --dir $(DIR)`
- `schema`: `go run ./cmd/server/ schema`
- `validate`: `go run ./cmd/server/ validate`
- `eval`: `./scripts/eval.sh`
- `eval-stability`: `./scripts/eval-stability.sh`
- `download-model`: `./scripts/download-model.sh`
- `prereqs`: install container engine + huggingface CLI

ENGINE variable for override (BUILD-05): `make container-up ENGINE=docker`
Auto-detect engine (podman preferred) at top of Makefile.

- [ ] **Step 2: Verify make prints targets**

```bash
make
```

- [ ] **Step 3: Verify make build works**

```bash
make build
```

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "Add Makefile with all required build, test, container, and utility targets."
```

### Task 13.2: llama.cpp Submodule

**Files:**
- Create: `.gitmodules`

- [ ] **Step 1: Add llama.cpp as git submodule pinned to tagged release (EMBED-01)**

```bash
git submodule add https://github.com/ggerganov/llama.cpp.git llama.cpp
cd llama.cpp && git checkout <tagged-release> && cd ..
git add .gitmodules llama.cpp
git commit -m "Add llama.cpp as git submodule pinned to tagged release."
```

Note: Select the latest stable tagged release at implementation time. Submodule updates must be reviewed before committing (EMBED-01).

### Task 13.3: Container Files

**Files:**
- Create: `Containerfile`
- Create: `Dockerfile` (symlink -> Containerfile)
- Create: `compose.yml`
- Create: `scripts/entrypoint.sh`

- [ ] **Step 1: Create Containerfile**

Multi-stage build (BUILD-02):
1. Stage 1: Go compile stage — build static binary
2. Stage 2: llama.cpp build stage — compile llama-server from submodule
3. Stage 3: Final image on `debian:bookworm-slim` (pinned SHA256 — SEC-12)
   - Non-root user (SEC-11)
   - Copy Go binary + llama-server binary
   - No secrets in layers (SEC-12)
   - Copy entrypoint.sh

- [ ] **Step 2: Create compose.yml**

Local dev stack (BUILD-03):
- `server` service: build from Containerfile, port 8080, env vars, depends on postgres
- `postgres` service: postgres + pgvector image, port 5432, volume for data

- [ ] **Step 3: Create entrypoint.sh**

Container entrypoint (EMBED-03, EMBED-06):
1. If bundled=true: start llama-server in background on 127.0.0.1:PORT
2. Health check with backoff up to 60s (EMBED-06)
3. If health check fails after 60s: exit 1 (ERR-08)
4. Start Go server
5. Trap SIGTERM/SIGINT, forward to Go server

- [ ] **Step 4: Create Dockerfile symlink**

```bash
ln -s Containerfile Dockerfile
```

- [ ] **Step 5: Commit**

```bash
git add Containerfile compose.yml scripts/entrypoint.sh Dockerfile
git commit -m "Add multi-stage Containerfile, compose.yml, and entrypoint script."
```

### Task 13.4: Helper Scripts

**Files:**
- Create: `scripts/get-token.sh`
- Create: `scripts/search.sh`
- Create: `scripts/query.sh`
- Create: `scripts/download-model.sh`
- Create: `scripts/eval.sh`
- Create: `scripts/eval-stability.sh`
- Create: `scripts/install-postgres.sh`
- Create: `scripts/build-llama.sh`
- Create: `scripts/detect-gpu.sh`

- [ ] **Step 1: Create get-token.sh**

Reads credentials from env vars or stdin — never CLI args (SEC-15). Supports `--flow client_credentials` and `--flow user_password` (section 8.2).

- [ ] **Step 2: Create search.sh and query.sh**

Convenience wrappers that call MCP endpoint via curl with JWT auth.

- [ ] **Step 3: Create download-model.sh**

Downloads GGUF embedding model via Hugging Face CLI (EMBED-08).

- [ ] **Step 4: Create eval.sh**

RAG evaluation script (EVAL-01 through EVAL-07):
- Reads eval JSON file
- For each entry: optional HyDE, search, LLM judge, structured verdict
- Reports pass rates, per-label rates, failed indices
- Authenticates via get-token.sh (EVAL-06)

- [ ] **Step 5: Create eval-stability.sh**

Runs eval.sh N times (default 25), reports min/max/avg pass rates (EVAL-05).

- [ ] **Step 6: Create remaining scripts**

install-postgres.sh, build-llama.sh, detect-gpu.sh — utility scripts per project structure.

- [ ] **Step 7: Make all scripts executable**

```bash
chmod +x scripts/*.sh
```

- [ ] **Step 8: Commit**

```bash
git add scripts/
git commit -m "Add all helper scripts: auth, search, eval, model download, and build utilities."
```

### Task 13.5: Example and Config Files

**Files:**
- Create: `cognito/config.json.example`
- Create: `data/evals/evals.json.example`

- [ ] **Step 1: Create cognito config example**

Example `aws-cognito` config referencing DB-09 (read-only MSSQL user requirement).

- [ ] **Step 2: Create eval example**

3-5 example eval entries with both "good" and "bad" labels (EVAL-08).

```json
[
  {"prompt": "How do I configure the database connection?", "label": "good", "notes": "Basic config question answerable from docs"},
  {"prompt": "What is the default chunk size for document ingestion?", "label": "good", "notes": "Should find 256 tokens in config docs"},
  {"prompt": "Does the server support Oracle database?", "label": "bad", "notes": "Oracle is not supported - should refuse false premise"},
  {"prompt": "How do I configure the Redis cache layer?", "label": "bad", "notes": "No Redis in this project - should refuse false premise"},
  {"prompt": "What authentication provider does the server use?", "label": "good", "notes": "AWS Cognito - well documented"}
]
```

- [ ] **Step 3: Commit**

```bash
git add cognito/config.json.example data/evals/evals.json.example
git commit -m "Add example configs for Cognito and evaluation framework."
```

---

## Phase 14: Integration and Final Assembly

### Task 14.1: Full Build Verification

- [ ] **Step 1: Run full build**

```bash
make build
```

Expected: Clean compilation, binary at `bin/mcp-server`.

- [ ] **Step 2: Run full unit test suite**

```bash
make test
```

Expected: ALL PASS.

- [ ] **Step 3: Run validate subcommand**

```bash
cp config.toml.example config.toml && chmod 600 config.toml
./bin/mcp-server validate --config config.toml
```

Expected: exits 0 (valid config).

- [ ] **Step 4: Run lint (if golangci-lint available)**

```bash
make lint 2>/dev/null || echo "lint skipped"
```

- [ ] **Step 5: Commit any fixes**

### Task 14.2: README.md

**Files:**
- Create: `README.md`

- [ ] **Step 1: Create README.md**

Per REQUIREMENTS.md section 12, include:
- Project summary and purpose
- Architecture overview (reference docs/DESIGN.md)
- First-time setup instructions (clone with submodules, prereqs, download model, configure, start)
- Fork workflow
- Build instructions (make targets)
- Test instructions (unit, integration, coverage)
- CLI subcommands (serve, ingest, validate, schema)
- Configuration reference (point to config.toml.example)
- Evaluation framework usage
- Security invariants for fork authors (section 7.3)

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Add README with setup, usage, fork workflow, and security invariants."
```

### Task 14.3: Startup Logging Verification

- [ ] **Step 1: Verify startup log includes all required fields (OBS-05)**

The `serve` command must log at startup:
- Server version
- Listen address
- Database engine
- Auth issuer URL
- Vector features enabled/disabled
- Container engine (if applicable)
- Guardrail status (L1/L2 with thresholds)
- HyDE status
- Reranker status

Verify by reading cmd/server/main.go and ensuring all fields are logged.

---

## Dependency Order Summary

The implementation follows the package dependency DAG strictly:

```
Phase 1:  Foundation (go.mod, .gitignore, config)
Phase 2:  Leaves (vecmath, engine)
Phase 3:  Database layer (interface, postgres, mssql)
Phase 4:  Auth (context, authorizer, cognito, middleware)
Phase 5:  Embed client
Phase 6:  Vector store
Phase 7:  Query store (safety, postgres, mssql)
Phase 8:  Search pipeline (guardrails, hyde, rerank, rrf, pipeline)
Phase 9:  Ingestion (chunker, walker, pipeline)
Phase 10: MCP tools (registry, search, query, ingest)
Phase 11: HTTP server
Phase 12: Main entry point + CLI
Phase 13: Build infrastructure (Makefile, containers, scripts)
Phase 14: Integration verification
```

Each phase depends only on completed phases above it. No circular dependencies.

## Key Requirement Cross-References

| Requirement Area | Tasks |
|-----------------|-------|
| MCP-01..07 | 10.1, 11.1, 12.1 |
| AUTH-01..11 | 4.1-4.4, 10.2-10.4 |
| DB-01..09 | 3.1-3.3 |
| VEC-01..10 | 5.1, 6.1, 8.4 |
| ING-01..15 | 9.1-9.3 |
| CHUNK-01..10 | 9.1 |
| GUARD-01..08 | 8.1 |
| ENH-01..11 | 8.2-8.4 |
| SQL-01..07 | 7.1-7.2 |
| CLI-01..06 | 12.1 |
| ENG-01..10 | 2.2 |
| EMBED-01..08 | 13.2 |
| EVAL-01..09 | 13.3 |
| CFG-01..05 | 1.3-1.4 |
| SEC-01..16 | Throughout (all phases) |
| OBS-01..07 | 11.1, 12.1 |
| PERF-01..05 | 8.4, 11.1 |
| TEST-01..07 | All test files |
| BUILD-01..05 | 13.1-13.2 |
| ERR-01..20 | Throughout (all phases) |
