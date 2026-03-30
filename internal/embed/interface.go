package embed

import "context"

// Embedder generates vector embeddings from text.
type Embedder interface {
	// Embed generates embeddings for the given texts.
	// Returned embeddings are L2-normalized.
	Embed(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedWithPrefix generates embeddings with a specific prefix prepended to each text.
	EmbedWithPrefix(ctx context.Context, texts []string, prefix string) ([][]float32, error)
}
