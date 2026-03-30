package hyde

import "context"

// NoopGenerator returns the raw query unchanged. Used when HyDE is disabled.
type NoopGenerator struct{}

func (n *NoopGenerator) Generate(ctx context.Context, query string) (string, bool, error) {
	return query, false, nil
}
