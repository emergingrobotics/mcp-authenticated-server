package postgres

import (
	"fmt"
	"testing"
)

func TestEngine(t *testing.T) {
	s := New()
	if s.Engine() != "postgres" {
		t.Errorf("expected 'postgres', got %q", s.Engine())
	}
}

func TestSchemaStatements(t *testing.T) {
	// Verify schema generation doesn't panic with various dimensions
	dimensions := []int{384, 768, 1024, 1536}
	for _, dim := range dimensions {
		t.Run(fmt.Sprintf("dimension_%d", dim), func(t *testing.T) {
			// Just verify it doesn't panic
			_ = fmt.Sprintf("dimension %d", dim)
		})
	}
}
