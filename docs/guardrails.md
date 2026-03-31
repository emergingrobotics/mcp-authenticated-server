# Guardrails

## Configuration

All guardrails are configured in the `[guardrails]` section of `config.toml`. Both levels are disabled by default and have zero runtime overhead when disabled. Changes are reloadable via SIGHUP without restarting the server.

### Level 1 -- Topic Relevance Gate

Blocks off-topic queries before they hit the database. Set `corpus_topic` to a short description of what your corpus contains:

```toml
[guardrails]
corpus_topic = "Kubernetes cluster administration and troubleshooting"
min_topic_score = 0.25
```

`corpus_topic` is a plain-text description of the corpus subject matter. At startup, the server embeds this text and uses it as a reference vector. Incoming queries are compared against it via cosine similarity. If the score is below `min_topic_score`, the query is rejected immediately.

- Set `corpus_topic = ""` (the default) to disable Level 1 entirely.
- `min_topic_score` must be in `[0.0, 1.0]`. The default `0.25` is a reasonable starting point. Lower values are more permissive; raise it if you see irrelevant queries getting through.
- Requires `embed.enabled = true` in your config. The server will refuse to start if `corpus_topic` is set without embeddings enabled.

**Client error on rejection**: `"off_topic: query does not appear to be related to the supported topic area"`

### Level 2 -- Match Score Gate

Blocks low-confidence results after retrieval and reranking. Set `min_match_score` to a non-zero threshold:

```toml
[]
min_match_score = 0.15
```

After KNN + full-text search results are merged via RRF (and optionally reranked), the server checks the highest-scoring result. If it falls below `min_match_score`, the entire result set is rejected.

- Set `min_match_score = 0.0` (the default) to disable Level 2 entirely.
- Must be in `[0.0, 1.0]`. Start low (0.10-0.20) and increase based on eval results. Setting this too high will cause valid queries to be rejected.
- The actual score is logged at debug level server-side but is not exposed to the client.

**Client error on rejection**: `"below_threshold: no content found that is sufficiently relevant to this query"`

### Full example

Both levels enabled together:

```toml
[guardrails]
corpus_topic = "Victorian-era mystery novels and detective fiction"
min_topic_score = 0.25
min_match_score = 0.15
```

### Tuning tips

- Run evals (`make eval`) with guardrails disabled first to establish a baseline pass rate.
- Enable Level 1, run evals again, and check whether any "good" queries are being rejected as off-topic. If so, broaden `corpus_topic` or lower `min_topic_score`.
- Enable Level 2, run evals again, and check whether any "good" queries are being rejected as below-threshold. Lower `min_match_score` if needed.
- The "bad" eval label tests whether the system correctly rejects fabricated premises. Guardrails should help these pass, not hurt them.

### Validation rules

- Both scores must be in `[0.0, 1.0]` -- the server will refuse to start otherwise.
- If `corpus_topic` is non-empty, `embed.enabled` must be `true`.

### Requirements traceability

GUARD-01 through GUARD-08 in `REQUIREMENTS.md`.

---

## How It Works

### Architecture

The guardrail system is implemented in `internal/guardrails/` with three files:

- `guardrails.go` -- `Guardrails` struct with `CheckTopicRelevance()` and `CheckMatchScore()` methods
- `topic.go` -- `TopicGuard` struct that holds the pre-computed corpus topic embedding and computes cosine similarity
- `guardrails_test.go` -- unit tests covering all threshold boundaries and disabled paths

```go
type Guardrails struct {
    topic    *TopicGuard  // nil when Level 1 is disabled
    minMatch float64      // 0 means Level 2 is disabled
}
```

Both check methods follow a zero-overhead pattern: if the guard is disabled (nil pointer or zero threshold), the method returns nil immediately with no computation.

### Search pipeline integration

Guardrails run at two points in the search pipeline (`internal/search/pipeline.go`):

```
Query
  |
  v
Embed query (with optional HyDE expansion)
  |
  v
L2-normalize embedding
  |
  v
[Level 1] CheckTopicRelevance(queryEmbedding)  --> reject if off-topic
  |
  v
Parallel KNN + Full-text search
  |
  v
Reciprocal Rank Fusion (RRF) merge
  |
  v
Optional cross-encoder reranking
  |
  v
[Level 2] CheckMatchScore(bestScore)  --> reject if below threshold
  |
  v
Truncate to requested limit
  |
  v
Return results
```

Level 1 runs before any database queries. This is intentional: off-topic queries are rejected without consuming database resources.

Level 2 runs after RRF merge and optional reranking. If a reranker is enabled, the reranker scores are used for the threshold check, not the raw RRF scores. This means the `min_match_score` threshold should be tuned relative to whatever scoring stage is last in your pipeline.

### Level 1 internals

At server startup (`cmd/server/main.go`):

1. The `corpus_topic` text is sent to the embedding server.
2. The resulting vector is L2-normalized and stored in the `TopicGuard`.
3. If embedding fails, the server exits immediately (does not run with partial guardrails).

At query time:

1. The query embedding (already L2-normalized by the pipeline) is passed to `CheckTopicRelevance()`.
2. Cosine similarity is computed as the dot product of the two L2-normalized vectors (using `internal/vecmath/`).
3. If the similarity is below `min_topic_score`, an error is returned.

The topic embedding is computed once at startup and reused for all queries. The per-query cost is a single dot product over the embedding dimension (768 floats by default).

### Level 2 internals

After RRF merge (and optional reranking), the pipeline passes the highest score from the merged result set to `CheckMatchScore()`. If the score is below `min_match_score`, an error is returned.

The score is logged at debug level for diagnostics:

```
level=DEBUG msg="Level 2 guardrail rejection" best_score=0.0823
```

### Error propagation

Both guardrail rejections return Go errors that propagate through the search pipeline to the tool handler (`internal/tools/registry.go`). The tool handler logs the error at info level with the tool name, user, and duration, then returns it as the MCP tool result text. The client sees only the error message prefix (`off_topic:` or `below_threshold:`), not internal scores.

### Extending with new guards

The design doc (`docs/DESIGN.md`) identifies `guardrails/guardrails.go` as the extension point for Level 3+ checks. To add a new guard:

1. Add a field to the `Guardrails` struct (nil pointer = disabled, zero overhead).
2. Add a `Check*()` method following the nil-check-first pattern.
3. Add config fields to `GuardrailsConfig` in `internal/config/config.go` with validation in `Validate()`.
4. Wire it up in `cmd/server/main.go` at startup.
5. Call the check method at the appropriate pipeline step in `internal/search/pipeline.go`:
   - Pre-retrieval (after Level 1, before database queries) for input guards.
   - Post-retrieval (after Level 2, before truncation) for output guards.

Proposed additions, ranked by impact:

1. **Prompt injection detection** -- Regex pattern matching and optional LLM classifier to block adversarial queries before embedding. Pre-retrieval.
2. **PII filtering** -- Scan result text for sensitive patterns (SSN, credit cards, API keys). Redact or block. Post-retrieval.
3. **Query length/complexity bounds** -- Reject single-character, excessively long, or high-special-character queries. Pre-retrieval.
4. **Rate-based abuse detection** -- Per-user sliding window counters to prevent corpus extraction. Pre-retrieval.
5. **Content coherence check** -- Cross-encoder or LLM judge to verify results actually answer the query. Post-retrieval.
6. **Result diversity guard** -- Detect and deduplicate near-identical results via pairwise cosine similarity. Post-retrieval.
