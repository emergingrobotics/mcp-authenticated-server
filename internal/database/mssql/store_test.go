package mssql

import "testing"

func TestEngine(t *testing.T) {
	s := New()
	if s.Engine() != "mssql" {
		t.Errorf("expected 'mssql', got %q", s.Engine())
	}
}
