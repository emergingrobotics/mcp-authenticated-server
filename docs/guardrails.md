# Guardrails

## Current Implementation

The system has a two-level guardrail system implemented in `internal/guardrails/`. Both levels are configurable via `[guardrails]` in `config.toml`, reloadable via SIGHUP, and zero-overhead when disabled.

### Level 1 -- Topic Relevance Gate (Pre-Database)

- Embeds a configured `corpus_topic` string at startup and L2-normalizes it
- Computes cosine similarity (dot product of L2-normalized vectors) between query embedding and topic vector
- Rejects the query if similarity is below `min_topic_score` (default 0.25)
- Runs before any database queries, preventing wasted compute on off-topic queries
- Error returned to client: `"off_topic: query does not appear to be related to the supported topic area"`

**Pipeline location**: Step 7 in `internal/search/pipeline.go`, after embedding and L2 normalization.

### Level 2 -- Match Score Gate (Post-Retrieval)

- Checks the highest-scoring result after RRF merge and optional reranking
- Rejects if best score is below `min_match_score`
- Prevents low-confidence results from reaching the client
- Score is logged at debug level server-side but not exposed to the client
- Error returned to client: `"below_threshold: no content found that is sufficiently relevant to this query"`

**Pipeline location**: Step 11 in `internal/search/pipeline.go`, after RRF merge and reranking.

### Configuration

```toml
[guardrails]
corpus_topic = ""              # Corpus topic description (empty = Level 1 disabled)
min_topic_score = 0.25         # Level 1 threshold [0.0, 1.0]
min_match_score = 0.0          # Level 2 threshold [0.0, 1.0] (0 = disabled)
```

**Validation rules**:
- Both scores must be in range `[0.0, 1.0]`
- If `corpus_topic` is non-empty, `embed.enabled` must be `true`

### Requirements Traceability

GUARD-01 through GUARD-08 in `REQUIREMENTS.md` cover:
- GUARD-01/02: Level 1 topic relevance gate and pre-DB execution
- GUARD-03/04: Level 2 match score gate and post-retrieval execution
- GUARD-05: Score range validation
- GUARD-06: Zero overhead when disabled
- GUARD-07: Startup logging
- GUARD-08: Cross-field validation (corpus_topic requires embed.enabled)

---

## Proposed Additions

The design doc (`docs/DESIGN.md`, line 123) identifies `guardrails/guardrails.go` as the extension point for Level 3+ checks. The following additions address risks the current two levels do not cover.

### Pre-Retrieval Guards (alongside Level 1)

#### Prompt Injection Detection

Classify the query for injection patterns before embedding. This prevents adversarial queries from reaching the retrieval pipeline.

**Detection layers** (in order of cost):

1. **Pattern matching** -- Regex rules for known injection signatures:
   - Instruction override attempts: `"ignore previous instructions"`, `"system prompt:"`, `"you are now"`
   - Encoded payloads: base64-encoded instructions, unicode homoglyphs, zero-width characters
   - Role-play attempts: `"pretend you are"`, `"act as"`, `"roleplay"`
   - Delimiter injection: markdown fences, XML tags, JSON/YAML structures embedded in natural language

2. **LLM classifier** (optional, higher cost) -- Send the query to a lightweight model with a classification prompt. Returns a confidence score; reject above a configurable threshold. This catches novel injection patterns that regex misses.

**Configuration**:
```toml
[guardrails]
injection_detection = false        # Enable prompt injection detection
injection_patterns_file = ""       # Path to custom regex pattern file (empty = built-in defaults)
injection_llm_enabled = false      # Enable LLM-based classifier (requires ANTHROPIC_API_KEY)
injection_llm_threshold = 0.8      # LLM classifier rejection threshold [0.0, 1.0]
```

**Error**: `"injection_detected: query appears to contain an injection attempt"`

#### Query Length and Complexity Bounds

Reject queries that are too short (single character), too long (token bomb), or contain excessive special characters. Simple, cheap, and catches obvious abuse.

**Configuration**:
```toml
[guardrails]
min_query_length = 3               # Minimum query character length (0 = disabled)
max_query_length = 2000            # Maximum query character length (0 = disabled)
max_special_char_ratio = 0.5       # Max ratio of non-alphanumeric characters [0.0, 1.0]
```

**Error**: `"invalid_query: query does not meet length or complexity requirements"`

#### Rate-Based Abuse Detection

Track per-user query patterns to detect systematic corpus extraction or denial-of-service attempts.

**Detection signals**:
- Rapid-fire identical queries from the same user
- Monotonically increasing offset/limit patterns suggesting enumeration
- Query volume exceeding a per-user sliding window threshold

**Configuration**:
```toml
[guardrails]
rate_limit_enabled = false         # Enable per-user rate limiting
rate_limit_window = "60s"          # Sliding window duration
rate_limit_max_queries = 30        # Max queries per window per user
rate_limit_dedup_window = "5s"     # Window for detecting duplicate queries
```

**Error**: `"rate_limited: too many requests"`

**Implementation note**: Requires an in-memory sliding window counter keyed by user ID (from JWT `sub` claim). A `sync.Map` of token-bucket or sliding-window counters, cleaned up periodically.

### Post-Retrieval Guards (alongside Level 2)

#### PII and Sensitive Data Filter

Scan result text for sensitive patterns before returning to the client. This addresses compliance requirements (GDPR, HIPAA, PCI-DSS) when the corpus may contain sensitive data that should not be surfaced.

**Pattern categories**:
- Social Security Numbers: `\b\d{3}-\d{2}-\d{4}\b`
- Credit card numbers: Luhn-validated 13-19 digit sequences
- Email addresses: RFC 5322 patterns
- Phone numbers: Common national formats
- API keys/tokens: High-entropy strings matching known provider formats (AWS, GitHub, Stripe)
- Custom patterns: User-defined regex list for domain-specific sensitive data

**Behavior options**:
- **Block**: Reject the entire result set if any match is found
- **Redact**: Replace matched patterns with `[REDACTED]` and return the sanitized results

**Configuration**:
```toml
[guardrails]
pii_filter_enabled = false         # Enable PII/sensitive data filtering
pii_filter_mode = "redact"         # "block" or "redact"
pii_patterns_file = ""             # Path to custom pattern file (empty = built-in defaults)
pii_categories = ["ssn", "credit_card", "email", "api_key"]  # Which built-in categories to enable
```

**Error (block mode)**: `"pii_detected: results contain sensitive data that cannot be returned"`

#### Content Coherence Check

Verify that returned results actually answer the query rather than being spurious high-similarity matches. This catches the failure mode where embedding similarity is high but semantic relevance is low.

**Mechanism**: Use a lightweight cross-encoder or LLM judge to score `(query, top_result)` pairs. If the best coherence score is below a threshold, reject the result set. This is similar to the existing reranker but operates as a binary pass/fail gate rather than a ranking signal.

**Configuration**:
```toml
[guardrails]
coherence_check_enabled = false    # Enable content coherence check
coherence_threshold = 0.5          # Minimum coherence score [0.0, 1.0]
coherence_top_k = 1                # Number of top results to check
```

**Error**: `"low_coherence: results do not appear to answer the query"`

**Implementation note**: This is the most expensive proposed guard. Consider applying it only to the top-1 result to limit latency impact. Can reuse the existing reranker infrastructure if available.

#### Result Diversity Guard

Detect when all top-K results are near-duplicates (high pairwise cosine similarity). Return deduplicated results or flag the condition.

**Mechanism**: Compute pairwise cosine similarity among the top-K result embeddings. If the minimum pairwise similarity exceeds a threshold, the results are near-identical.

**Behavior options**:
- **Deduplicate**: Remove near-duplicate results, returning only distinct passages
- **Warn**: Return results with a metadata flag indicating low diversity

**Configuration**:
```toml
[guardrails]
diversity_guard_enabled = false    # Enable result diversity guard
diversity_threshold = 0.95         # Pairwise similarity above this = duplicate [0.0, 1.0]
diversity_mode = "dedup"           # "dedup" or "warn"
```

---

## Implementation Pattern

Each new guard follows the existing zero-overhead pattern in `internal/guardrails/guardrails.go`:

```go
type Guardrails struct {
    topic       *TopicGuard
    minMatch    float64
    piiFilter   *PIIGuard        // Level 3
    injection   *InjectionGuard  // Level 3
}

func (g *Guardrails) CheckPII(resultTexts []string) error {
    if g.piiFilter == nil {
        return nil
    }
    // scan and reject/redact
}

func (g *Guardrails) CheckInjection(query string) error {
    if g.injection == nil {
        return nil
    }
    // pattern match and/or LLM classify
}
```

**Pipeline integration** in `internal/search/pipeline.go`:
- Pre-retrieval guards: after step 7 (Level 1 topic check), before database queries
- Post-retrieval guards: after step 11 (Level 2 score check), before truncation and return

**Configuration**: Add fields to `GuardrailsConfig` in `internal/config/config.go` with validation in `config.Validate()`.

**Testing**: Each guard needs unit tests covering:
- Disabled path returns nil immediately
- Boundary values at threshold
- Known positive and negative cases
- Error message format

---

## Priority

Ranked by impact:

1. **Prompt injection detection** -- Security. Protects against adversarial misuse of the retrieval pipeline.
2. **PII filtering** -- Compliance. Prevents accidental exposure of sensitive data from the corpus.
3. **Query length/complexity bounds** -- Robustness. Cheap to implement, catches obvious abuse.
4. **Rate-based abuse detection** -- Availability. Prevents corpus extraction and resource exhaustion.
5. **Content coherence check** -- Quality. Catches false-positive retrieval results.
6. **Result diversity guard** -- Quality. Addresses corpus redundancy issues.
