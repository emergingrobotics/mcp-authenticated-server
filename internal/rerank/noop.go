package rerank

import "context"

// NoopReranker does nothing. Used when reranking is disabled.
type NoopReranker struct{}

func (n *NoopReranker) Rerank(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
	return nil, nil
}
