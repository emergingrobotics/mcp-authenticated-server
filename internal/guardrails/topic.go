package guardrails

import (
	"github.com/emergingrobotics/mcp-authenticated-server/internal/vecmath"
)

// TopicGuard implements Level 1 guardrail: topic relevance gate.
type TopicGuard struct {
	topicVector []float32
	minScore    float32
}

// NewTopicGuard creates a topic guard with a precomputed topic embedding.
// The topicEmbedding must already be L2-normalized.
func NewTopicGuard(topicEmbedding []float32, minScore float32) *TopicGuard {
	return &TopicGuard{
		topicVector: topicEmbedding,
		minScore:    minScore,
	}
}

// Check returns nil if the query embedding is sufficiently relevant to the topic.
func (g *TopicGuard) Check(queryEmbedding []float32) (float32, bool) {
	score := vecmath.DotProduct(queryEmbedding, g.topicVector)
	return score, score >= g.minScore
}
