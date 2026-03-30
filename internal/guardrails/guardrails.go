package guardrails

import "fmt"

// Guardrails implements the two-level guardrail system.
type Guardrails struct {
	topic    *TopicGuard // nil if Level 1 disabled
	minMatch float64     // 0 means Level 2 disabled
}

// New creates a Guardrails instance.
// Pass nil for topic to disable Level 1. Pass 0 for minMatch to disable Level 2.
func New(topic *TopicGuard, minMatchScore float64) *Guardrails {
	return &Guardrails{
		topic:    topic,
		minMatch: minMatchScore,
	}
}

// CheckTopicRelevance runs the Level 1 guardrail (pre-DB).
// Returns nil if the check passes or if Level 1 is disabled.
func (g *Guardrails) CheckTopicRelevance(queryEmbedding []float32) error {
	if g.topic == nil {
		return nil
	}
	_, ok := g.topic.Check(queryEmbedding)
	if !ok {
		return fmt.Errorf("off_topic: query does not appear to be related to the supported topic area")
	}
	return nil
}

// CheckMatchScore runs the Level 2 guardrail (post-retrieval).
// bestScore is the highest score from the merged/reranked results.
// Returns nil if the check passes or if Level 2 is disabled.
func (g *Guardrails) CheckMatchScore(bestScore float64) error {
	if g.minMatch <= 0 {
		return nil
	}
	if bestScore < g.minMatch {
		return fmt.Errorf("below_threshold: no content found that is sufficiently relevant to this query")
	}
	return nil
}

// TopicEnabled returns true if Level 1 is active.
func (g *Guardrails) TopicEnabled() bool {
	return g.topic != nil
}

// MatchEnabled returns true if Level 2 is active.
func (g *Guardrails) MatchEnabled() bool {
	return g.minMatch > 0
}
