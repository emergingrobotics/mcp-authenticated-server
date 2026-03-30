package ingest

import (
	"path/filepath"
	"strings"
)

// Chunk represents a document chunk with metadata.
type Chunk struct {
	Content        string
	HeadingContext string
	ChunkType      string // "paragraph", "code", "table", "list"
	TokenCount     int
}

// Chunker splits document content into chunks.
type Chunker interface {
	ChunkFile(path string, content []byte, chunkSize int) (title string, chunks []Chunk, err error)
}

// MarkdownChunker implements structure-aware markdown chunking.
type MarkdownChunker struct{}

func (c *MarkdownChunker) ChunkFile(path string, content []byte, chunkSize int) (string, []Chunk, error) {
	lines := strings.Split(string(content), "\n")
	var chunks []Chunk
	var title string

	// Heading stack (CHUNK-01)
	headings := make([]string, 7) // index 1-6
	currentBreadcrumb := ""

	var accum strings.Builder
	accumType := "paragraph"
	inCodeBlock := false
	var codeAccum strings.Builder
	inYAMLFront := false
	yamlDone := false
	inTable := false
	var tableAccum strings.Builder

	flush := func(chunkType string) {
		text := strings.TrimSpace(accum.String())
		if text == "" || len(text) < 50 {
			if text != "" && len(text) < 50 {
				// Too short for standalone chunk, keep accumulating
				return
			}
			accum.Reset()
			return
		}
		chunks = append(chunks, Chunk{
			Content:        text,
			HeadingContext: currentBreadcrumb,
			ChunkType:      chunkType,
			TokenCount:     estimateTokens(text),
		})
		accum.Reset()
	}

	flushTable := func() {
		text := strings.TrimSpace(tableAccum.String())
		if text == "" {
			return
		}
		chunks = append(chunks, Chunk{
			Content:        text,
			HeadingContext: currentBreadcrumb,
			ChunkType:      "table",
			TokenCount:     estimateTokens(text),
		})
		tableAccum.Reset()
	}

	buildBreadcrumb := func() string {
		var parts []string
		for i := 1; i <= 6; i++ {
			if headings[i] != "" {
				parts = append(parts, headings[i])
			}
		}
		return strings.Join(parts, " > ")
	}

	for i, line := range lines {
		// YAML front matter (CHUNK-07)
		if i == 0 && strings.TrimSpace(line) == "---" && !yamlDone {
			inYAMLFront = true
			continue
		}
		if inYAMLFront {
			if strings.TrimSpace(line) == "---" {
				inYAMLFront = false
				yamlDone = true
			}
			continue
		}
		yamlDone = true

		// Code blocks (CHUNK-03)
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inCodeBlock {
				// End of code block
				codeAccum.WriteString(line)
				text := strings.TrimSpace(codeAccum.String())
				if len(text) >= 50 {
					chunks = append(chunks, Chunk{
						Content:        text,
						HeadingContext: currentBreadcrumb,
						ChunkType:      "code",
						TokenCount:     estimateTokens(text),
					})
				}
				codeAccum.Reset()
				inCodeBlock = false
				continue
			}
			// Start of code block - flush accumulated text first
			flush(accumType)
			accumType = "paragraph"
			inCodeBlock = true
			codeAccum.WriteString(line)
			codeAccum.WriteString("\n")
			continue
		}
		if inCodeBlock {
			codeAccum.WriteString(line)
			codeAccum.WriteString("\n")
			continue
		}

		// Tables (CHUNK-04)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "|") {
			if !inTable {
				flush(accumType)
				accumType = "paragraph"
				inTable = true
			}
			tableAccum.WriteString(line)
			tableAccum.WriteString("\n")
			continue
		}
		if inTable {
			flushTable()
			inTable = false
		}

		// Headings (CHUNK-01)
		if level := headingLevel(trimmed); level > 0 {
			// Flush accumulated text with PREVIOUS breadcrumb
			flush(accumType)
			accumType = "paragraph"

			headingText := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			headings[level] = headingText

			// Truncate deeper levels (CHUNK-01b)
			for j := level + 1; j <= 6; j++ {
				headings[j] = ""
			}

			currentBreadcrumb = buildBreadcrumb()

			// Title: first H1 (CHUNK-08)
			if level == 1 && title == "" {
				title = headingText
			}
			continue
		}

		// Lists (CHUNK-05)
		if isListLine(trimmed) {
			if accumType != "list" {
				flush(accumType)
				accumType = "list"
			}
			accum.WriteString(line)
			accum.WriteString("\n")

			if estimateTokens(accum.String()) >= chunkSize {
				flush("list")
				accumType = "list"
			}
			continue
		}

		// Paragraphs (CHUNK-06)
		if accumType == "list" {
			flush("list")
			accumType = "paragraph"
		}

		if trimmed == "" {
			// Empty line can trigger flush if we have enough content
			if estimateTokens(accum.String()) > 0 {
				// Don't flush on empty line alone, just add newline for spacing
				accum.WriteString("\n")
			}
			continue
		}

		accum.WriteString(line)
		accum.WriteString("\n")

		if estimateTokens(accum.String()) >= chunkSize {
			flush("paragraph")
			accumType = "paragraph"
		}
	}

	// Flush remaining
	if inCodeBlock {
		text := strings.TrimSpace(codeAccum.String())
		if len(text) >= 50 {
			chunks = append(chunks, Chunk{
				Content:        text,
				HeadingContext: currentBreadcrumb,
				ChunkType:      "code",
				TokenCount:     estimateTokens(text),
			})
		}
	}
	if inTable {
		flushTable()
	}
	flush(accumType)

	// Title fallback to filename stem (CHUNK-08)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	return title, chunks, nil
}

// BuildEmbedText constructs the text for embedding per CHUNK-09.
func BuildEmbedText(prefix, filename, headingContext, content string) string {
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteString(filename)
	if headingContext != "" {
		sb.WriteString(": ")
		sb.WriteString(headingContext)
	}
	sb.WriteString("\n\n")
	sb.WriteString(content)
	return sb.String()
}

func headingLevel(line string) int {
	level := 0
	for _, ch := range line {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	if level >= 1 && level <= 6 && len(line) > level && line[level] == ' ' {
		return level
	}
	return 0
}

func isListLine(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return true
	}
	// Numbered list: N.
	for i, ch := range line {
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '.' && i > 0 && i < len(line)-1 && line[i+1] == ' ' {
			return true
		}
		break
	}
	return false
}

// estimateTokens approximates token count as len/4 (ING-06).
func estimateTokens(text string) int {
	return len(text) / 4
}
