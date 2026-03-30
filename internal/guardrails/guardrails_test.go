package guardrails

import (
	"testing"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/vecmath"
)

func TestTopicGuard_Relevant(t *testing.T) {
	topic := []float32{1, 0, 0}
	vecmath.L2Normalize(topic)
	g := NewTopicGuard(topic, 0.5)

	query := []float32{0.9, 0.1, 0}
	vecmath.L2Normalize(query)

	score, ok := g.Check(query)
	if !ok {
		t.Errorf("expected relevant (score=%f), got not relevant", score)
	}
}

func TestTopicGuard_Irrelevant(t *testing.T) {
	topic := []float32{1, 0, 0}
	vecmath.L2Normalize(topic)
	g := NewTopicGuard(topic, 0.5)

	query := []float32{0, 1, 0}
	vecmath.L2Normalize(query)

	score, ok := g.Check(query)
	if ok {
		t.Errorf("expected irrelevant (score=%f), got relevant", score)
	}
}

func TestGuardrails_Level1_Disabled(t *testing.T) {
	g := New(nil, 0)
	if err := g.CheckTopicRelevance([]float32{1, 0, 0}); err != nil {
		t.Errorf("expected no error when L1 disabled, got: %v", err)
	}
}

func TestGuardrails_Level1_Reject(t *testing.T) {
	topic := []float32{1, 0, 0}
	vecmath.L2Normalize(topic)
	tg := NewTopicGuard(topic, 0.9)
	g := New(tg, 0)

	query := []float32{0, 1, 0} // orthogonal
	vecmath.L2Normalize(query)

	if err := g.CheckTopicRelevance(query); err == nil {
		t.Error("expected off_topic error")
	}
}

func TestGuardrails_Level1_Pass(t *testing.T) {
	topic := []float32{1, 0, 0}
	vecmath.L2Normalize(topic)
	tg := NewTopicGuard(topic, 0.5)
	g := New(tg, 0)

	query := []float32{0.9, 0.1, 0}
	vecmath.L2Normalize(query)

	if err := g.CheckTopicRelevance(query); err != nil {
		t.Errorf("expected pass, got: %v", err)
	}
}

func TestGuardrails_Level2_Disabled(t *testing.T) {
	g := New(nil, 0)
	if err := g.CheckMatchScore(0.01); err != nil {
		t.Errorf("expected no error when L2 disabled, got: %v", err)
	}
}

func TestGuardrails_Level2_Reject(t *testing.T) {
	g := New(nil, 0.5)
	if err := g.CheckMatchScore(0.1); err == nil {
		t.Error("expected below_threshold error")
	}
}

func TestGuardrails_Level2_Pass(t *testing.T) {
	g := New(nil, 0.5)
	if err := g.CheckMatchScore(0.8); err != nil {
		t.Errorf("expected pass, got: %v", err)
	}
}

func TestGuardrails_BothDisabled_ZeroOverhead(t *testing.T) {
	g := New(nil, 0)
	if g.TopicEnabled() {
		t.Error("expected TopicEnabled=false")
	}
	if g.MatchEnabled() {
		t.Error("expected MatchEnabled=false")
	}
}

func TestGuardrails_BothEnabled(t *testing.T) {
	topic := []float32{1, 0}
	vecmath.L2Normalize(topic)
	tg := NewTopicGuard(topic, 0.25)
	g := New(tg, 0.1)
	if !g.TopicEnabled() {
		t.Error("expected TopicEnabled=true")
	}
	if !g.MatchEnabled() {
		t.Error("expected MatchEnabled=true")
	}
}
