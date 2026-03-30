package hyde

import "context"

// Generator expands a user query into a hypothetical document passage.
type Generator interface {
	// Generate returns an expanded query or hypothesis.
	// isPassage is true when the result should use passage_prefix for embedding;
	// false when it should use query_prefix (raw query or fallback).
	Generate(ctx context.Context, query string) (hypothesis string, isPassage bool, err error)
}
