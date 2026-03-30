package vectorstore

import "testing"

func TestNewPostgresStore(t *testing.T) {
	// Unit test: verify construction doesn't panic
	s := NewPostgresStore(nil, 768)
	if s.dimension != 768 {
		t.Errorf("expected dimension 768, got %d", s.dimension)
	}
}
