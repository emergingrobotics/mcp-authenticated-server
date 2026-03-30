package rerank

import "context"

// RerankResult is a single reranked document.
type RerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// Reranker scores documents against a query using a cross-encoder model.
type Reranker interface {
	// Rerank returns scored results. Returns nil on error (caller falls back to RRF).
	Rerank(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error)
}
