package hyde

import (
	"context"
	"testing"
)

func TestNoopGenerator(t *testing.T) {
	g := &NoopGenerator{}
	hypothesis, isPassage, err := g.Generate(context.Background(), "what is the default port?")
	if err != nil {
		t.Fatal(err)
	}
	if hypothesis != "what is the default port?" {
		t.Errorf("expected raw query, got %q", hypothesis)
	}
	if isPassage {
		t.Error("expected isPassage=false for noop")
	}
}

func TestNewAnthropicGenerator_NoAPIKey(t *testing.T) {
	// Ensure ANTHROPIC_API_KEY is not set
	t.Setenv("ANTHROPIC_API_KEY", "")

	gen := NewAnthropicGenerator("claude-haiku-4-5-20251001", "", "")
	// Should return a NoopGenerator
	if _, ok := gen.(*NoopGenerator); !ok {
		t.Errorf("expected NoopGenerator when API key is missing, got %T", gen)
	}
}

func TestNewAnthropicGenerator_WithAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-not-real")

	gen := NewAnthropicGenerator("claude-haiku-4-5-20251001", "", "")
	if _, ok := gen.(*AnthropicGenerator); !ok {
		t.Errorf("expected AnthropicGenerator when API key is set, got %T", gen)
	}
}
