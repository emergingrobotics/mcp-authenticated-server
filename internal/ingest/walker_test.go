package ingest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalk_BasicFiltering(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Hello"), 0644)
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "image.png"), []byte("not text"), 0644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden"), 0644)
	os.WriteFile(filepath.Join(dir, ".env.example"), []byte("VAR=val"), 0644)

	entries, err := Walk(dir, WalkOptions{

		AllowedExtensions: []string{".md", ".go"},
		ExcludedDirs:      []string{},
		MaxFileSize:       1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[filepath.Base(e.Path)] = true
	}

	if !names["readme.md"] {
		t.Error("expected readme.md")
	}
	if !names["code.go"] {
		t.Error("expected code.go")
	}
	if names["image.png"] {
		t.Error("expected image.png to be filtered out")
	}
	if names[".hidden"] {
		t.Error("expected .hidden to be filtered out")
	}
}

func TestWalk_ExcludedDirs(t *testing.T) {
	dir := t.TempDir()

	nodeDir := filepath.Join(dir, "node_modules")
	os.MkdirAll(nodeDir, 0755)
	os.WriteFile(filepath.Join(nodeDir, "pkg.js"), []byte("code"), 0644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("main"), 0644)

	entries, err := Walk(dir, WalkOptions{

		AllowedExtensions: []string{".js"},
		ExcludedDirs:      []string{"node_modules"},
		MaxFileSize:       1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if filepath.Base(e.Path) == "pkg.js" {
			t.Error("expected node_modules/pkg.js to be excluded")
		}
	}
}

func TestWalk_MaxFileSize(t *testing.T) {
	dir := t.TempDir()

	small := make([]byte, 100)
	big := make([]byte, 2000)
	os.WriteFile(filepath.Join(dir, "small.md"), small, 0644)
	os.WriteFile(filepath.Join(dir, "big.md"), big, 0644)

	entries, err := Walk(dir, WalkOptions{

		AllowedExtensions: []string{".md"},
		ExcludedDirs:      []string{},
		MaxFileSize:       1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 file (small), got %d", len(entries))
	}
}

func TestWalk_NonExistentDir(t *testing.T) {
	_, err := Walk("/does/not/exist", WalkOptions{})
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestWalk_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	entries, err := Walk(dir, WalkOptions{

		AllowedExtensions: []string{".md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty dir, got %d", len(entries))
	}
}

func TestMatchesRagignore(t *testing.T) {
	patterns := []string{"*.log", "temp/**", "secret.txt"}

	tests := []struct {
		path    string
		matched bool
	}{
		{"output.log", true},
		{"temp/file.txt", true},
		{"secret.txt", true},
		{"readme.md", false},
		{"src/main.go", false},
	}

	for _, tt := range tests {
		got := MatchesRagignore(tt.path, patterns)
		if got != tt.matched {
			t.Errorf("MatchesRagignore(%q) = %v, want %v", tt.path, got, tt.matched)
		}
	}
}

func TestLoadRagignore(t *testing.T) {
	dir := t.TempDir()
	ragignore := "# comment\n*.log\n\ntemp/**\n"
	os.WriteFile(filepath.Join(dir, ".ragignore"), []byte(ragignore), 0644)

	patterns, err := LoadRagignore(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d: %v", len(patterns), patterns)
	}
}

func TestLoadRagignore_NoFile(t *testing.T) {
	dir := t.TempDir()
	patterns, err := LoadRagignore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if patterns != nil {
		t.Errorf("expected nil patterns when no .ragignore, got %v", patterns)
	}
}
