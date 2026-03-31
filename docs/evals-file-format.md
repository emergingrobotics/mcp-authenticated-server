# Eval File Format

## Overview

Eval files are JSON arrays of evaluation cases used to measure RAG retrieval and answer quality. Each case poses a question against the ingested document corpus and declares whether the question is answerable (`good`), contains fabricated details (`bad`), or is outside the corpus domain (`off_topic`). The eval harness (`scripts/eval.sh`) runs each case through the MCP server's `search_documents` tool. For `good` and `bad` labels, an LLM judge evaluates the result. For `off_topic` labels, the LLM is asked to answer the question using the search results and its reply is printed for human review -- this shows whether the system correctly declines to answer or hallucinates from unrelated results.

## File Location

Eval files live under `data/<corpus>/evals/evals.json`, where `<corpus>` is the name of the document collection:

```
data/
├── mystery-books/evals/evals.json     # 100 evals (50 good, 50 bad)
├── sf-books/evals/evals.json          # 100 evals (50 good, 50 bad)
├── support/evals/evals.json           #  40 evals (28 good, 12 bad)
└── techdocs/evals/evals.json          # 200 evals (100 good, 100 bad)
```

## Schema

The file is a JSON array of objects. Each object has exactly four fields:

```json
[
  {
    "prompt": "What BrightScript object and method do you call to perform an async HTTP GET?",
    "label": "good",
    "notes": "roUrlTransfer with AsyncGetToFile(destination).",
    "file": "08-fetching-remote-content"
  }
]
```

### Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `prompt` | string | yes | The natural language question to ask the RAG system. Typically 40-150 characters. |
| `label` | string | yes | One of `"good"`, `"bad"`, or `"off_topic"`. Determines how the answer is judged. |
| `notes` | string | yes | Explains what a correct answer must contain (for `good`) or what is fabricated (for `bad`). Used by the LLM judge to evaluate the answer. |
| `file` | string | yes | The source document filename (without extension) that the eval targets. Used for traceability — identifies which document in the corpus the question was derived from. |

### Label semantics

**`good`** — The question is answerable from the corpus. The prompt asks about real content that exists in the ingested documents. A passing answer correctly addresses the question using retrieved content.

- The `notes` field describes what a correct answer should cover.
- The eval fails if the RAG system cannot retrieve relevant chunks or if the generated answer does not address the question.

**`bad`** — The question contains fabricated details that are not in the corpus. The prompt references plausible-sounding but nonexistent methods, APIs, characters, events, or configuration options.

- The `notes` field explains what is fabricated and why the question is unanswerable.
- The eval passes only if the answer refuses to confirm the false premise. An answer that invents a plausible-sounding response (hallucination) fails.

**`off_topic`** — The question is outside the corpus domain entirely. Instead of pass/fail judging, the LLM is asked to answer the question using the search results and its reply is printed to stdout for human review.

- The `notes` field describes why the question is off-topic and what domain it belongs to.
- Off-topic evals are excluded from the pass/fail count and pass rate calculation.
- The printed reply shows whether the system correctly refuses to answer or hallucinates from unrelated search results. This is useful for tuning Level 1 (topic relevance) guardrails and evaluating how the system handles queries it should decline.

## Design principles

### Balanced labels

Eval files should contain a roughly equal mix of `good` and `bad` cases. This prevents bias: a system that always answers would score well on `good` evals but poorly on `bad`, and vice versa. The overall pass rate reflects both retrieval accuracy and hallucination resistance.

### `bad` evals must be plausible

Fabricated questions should be hard to distinguish from real ones without access to the corpus. They should use correct terminology, realistic naming conventions, and plausible API patterns from the domain. A `bad` eval that asks about "the foobar widget" is too easy to reject; one that asks about "the `SetProxyBypassList()` method on `roUrlTransfer`" requires the system to actually check the corpus.

### `notes` guide the judge

The `notes` field is not shown to the RAG system — it is only used by the LLM judge that evaluates the answer. For `good` evals, notes should be specific enough that the judge can verify the answer is substantively correct, not just vaguely related. For `bad` evals, notes should clearly identify the fabricated element so the judge can detect whether the answer accepted or rejected the false premise.

### `file` enables traceability

The `file` field traces each eval back to its source document. This is useful for diagnosing retrieval failures: if all failures come from the same file, the document may be poorly chunked or underrepresented in the index. The value should match the filename stem (without path or extension) of a document in the corpus.

## Validation

An eval file is valid when:

1. It is a JSON array (not an object or scalar).
2. Every element has all four fields: `prompt`, `label`, `notes`, `file`.
3. Every `label` is one of `"good"`, `"bad"`, or `"off_topic"`.
4. Every `prompt` is a non-empty string.
5. Every `notes` is a non-empty string.
6. The array contains at least one element.

## Running evals

```sh
# Run all evals in a file
make eval EVAL_FILE=./data/techdocs/evals/evals.json

# With verbose output (shows answers)
make eval EVAL_FILE=./data/techdocs/evals/evals.json ARGS="--verbose"

# With HyDE query expansion
make eval EVAL_FILE=./data/techdocs/evals/evals.json ARGS="--expand-query"
```

The summary reports overall pass rate and a per-label breakdown:

```
---
Results: 190/200 passed, 10/200 failed (pass rate: 95.0%)
  good label:      90/100 (90.0%)
  bad label:       100/100 (100.0%)

Failed eval indices: 25 30 36 39 43 62 72 73 80 85
```

When `off_topic` evals are present, they are excluded from the pass/fail totals:

```
---
Results: 90/100 passed, 10/100 failed (pass rate: 90.0%)
  good label:      45/50 (90.0%)
  bad label:       45/50 (90.0%)
  off_topic label: 25 (not judged, replies printed above)

Failed eval indices: 3 17 22 31 40 55 62 78 89 94
```
