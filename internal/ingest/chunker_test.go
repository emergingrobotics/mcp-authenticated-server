package ingest

import (
	"testing"
)

func TestMarkdownChunker_HeadingStack(t *testing.T) {
	content := `# Installation

Some intro text that is long enough to be at least fifty characters for the minimum chunk size requirement.

## Linux

Linux specific instructions that are also long enough to meet the fifty character minimum chunk size requirement.

### Docker

Docker setup instructions that are also long enough to meet the fifty character minimum chunk size requirement.

## macOS

macOS specific instructions that are also long enough to meet the fifty character minimum chunk size requirement.
`
	chunker := &MarkdownChunker{}
	title, chunks, err := chunker.ChunkFile("test.md", []byte(content), 50)
	if err != nil {
		t.Fatal(err)
	}

	if title != "Installation" {
		t.Errorf("expected title 'Installation', got %q", title)
	}

	// Verify heading contexts
	foundContexts := make(map[string]bool)
	for _, c := range chunks {
		if c.HeadingContext != "" {
			foundContexts[c.HeadingContext] = true
		}
	}

	if !foundContexts["Installation"] {
		t.Error("expected heading context 'Installation'")
	}
	if !foundContexts["Installation > Linux"] {
		t.Error("expected heading context 'Installation > Linux'")
	}
}

func TestMarkdownChunker_CodeBlocks(t *testing.T) {
	content := "# Test\n\n```go\nfunc main() {\n    fmt.Println(\"hello world this is a long enough code block\")\n}\n```\n"
	chunker := &MarkdownChunker{}
	_, chunks, err := chunker.ChunkFile("test.md", []byte(content), 256)
	if err != nil {
		t.Fatal(err)
	}

	foundCode := false
	for _, c := range chunks {
		if c.ChunkType == "code" {
			foundCode = true
		}
	}
	if !foundCode {
		t.Error("expected at least one code chunk")
	}
}

func TestMarkdownChunker_Tables(t *testing.T) {
	content := "# Test\n\n| Col1 | Col2 | Col3 | Col4 | Col5 | Col6 |\n|------|------|------|------|------|------|\n| val1 | val2 | val3 | val4 | val5 | val6 |\n| val7 | val8 | val9 | valA | valB | valC |\n"
	chunker := &MarkdownChunker{}
	_, chunks, err := chunker.ChunkFile("test.md", []byte(content), 256)
	if err != nil {
		t.Fatal(err)
	}

	foundTable := false
	for _, c := range chunks {
		if c.ChunkType == "table" {
			foundTable = true
		}
	}
	if !foundTable {
		t.Error("expected at least one table chunk")
	}
}

func TestMarkdownChunker_Lists(t *testing.T) {
	content := "# Test\n\n- First item that is long enough for the minimum chunk requirement of fifty characters\n- Second item that is also long enough for the minimum chunk requirement of fifty characters\n* Third item that is also long enough for the minimum chunk requirement of fifty chars\n"
	chunker := &MarkdownChunker{}
	_, chunks, err := chunker.ChunkFile("test.md", []byte(content), 256)
	if err != nil {
		t.Fatal(err)
	}

	foundList := false
	for _, c := range chunks {
		if c.ChunkType == "list" {
			foundList = true
		}
	}
	if !foundList {
		t.Error("expected at least one list chunk")
	}
}

func TestMarkdownChunker_YAMLFrontMatter(t *testing.T) {
	content := "---\ntitle: Test Doc\ndate: 2024-01-01\n---\n\n# Test\n\nSome content here that is long enough to meet the fifty character minimum chunk size requirement for proper testing.\n"
	chunker := &MarkdownChunker{}
	_, chunks, err := chunker.ChunkFile("test.md", []byte(content), 256)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range chunks {
		if contains(c.Content, "title: Test Doc") || contains(c.Content, "date: 2024") {
			t.Error("YAML front matter should be stripped")
		}
	}
}

func TestMarkdownChunker_TitleFromFilename(t *testing.T) {
	content := "Some content without any headings that is long enough to meet the fifty character minimum chunk size.\n"
	chunker := &MarkdownChunker{}
	title, _, err := chunker.ChunkFile("my-document.md", []byte(content), 256)
	if err != nil {
		t.Fatal(err)
	}
	if title != "my-document" {
		t.Errorf("expected title 'my-document', got %q", title)
	}
}

func TestMarkdownChunker_MinimumChunkLength(t *testing.T) {
	content := "# Test\n\nShort.\n"
	chunker := &MarkdownChunker{}
	_, chunks, err := chunker.ChunkFile("test.md", []byte(content), 256)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range chunks {
		if len(c.Content) < 50 {
			t.Errorf("chunk below 50 chars: %q (%d chars)", c.Content, len(c.Content))
		}
	}
}

func TestBuildEmbedText(t *testing.T) {
	tests := []struct {
		prefix, filename, heading, content, expected string
	}{
		{
			"passage: ", "README.md", "Installation > Docker",
			"Run docker compose up",
			"passage: README.md: Installation > Docker\n\nRun docker compose up",
		},
		{
			"", "notes.md", "",
			"Some notes",
			"notes.md\n\nSome notes",
		},
	}

	for _, tt := range tests {
		got := BuildEmbedText(tt.prefix, tt.filename, tt.heading, tt.content)
		if got != tt.expected {
			t.Errorf("BuildEmbedText(%q, %q, %q, %q) = %q, want %q",
				tt.prefix, tt.filename, tt.heading, tt.content, got, tt.expected)
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	// 1024 chars / 4 = 256 tokens
	text := make([]byte, 1024)
	for i := range text {
		text[i] = 'a'
	}
	got := estimateTokens(string(text))
	if got != 256 {
		t.Errorf("expected 256 tokens, got %d", got)
	}
}

func TestHeadingLevel(t *testing.T) {
	tests := []struct {
		line  string
		level int
	}{
		{"# H1", 1},
		{"## H2", 2},
		{"### H3", 3},
		{"#### H4", 4},
		{"##### H5", 5},
		{"###### H6", 6},
		{"####### H7", 0}, // too deep
		{"Not a heading", 0},
		{"#NoSpace", 0},
	}

	for _, tt := range tests {
		got := headingLevel(tt.line)
		if got != tt.level {
			t.Errorf("headingLevel(%q) = %d, want %d", tt.line, got, tt.level)
		}
	}
}

func TestIsListLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"- item", true},
		{"* item", true},
		{"1. item", true},
		{"10. item", true},
		{"not a list", false},
		{"--- separator", false},
	}

	for _, tt := range tests {
		got := isListLine(tt.line)
		if got != tt.want {
			t.Errorf("isListLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
