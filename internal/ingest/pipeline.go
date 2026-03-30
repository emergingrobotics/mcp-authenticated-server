package ingest

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/embed"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/vectorstore"
)

// IngestResult holds the results of an ingestion run.
type IngestResult struct {
	DocumentsProcessed int     `json:"documents_processed"`
	ChunksCreated      int     `json:"chunks_created"`
	Errors             int     `json:"errors"`
	DurationSeconds    float64 `json:"duration_seconds"`
}

// PipelineConfig holds ingestion configuration.
type PipelineConfig struct {
	ChunkSize         int
	BatchSize         int
	MaxFileSize       int64
	AllowedDirs       []string
	AllowedExtensions []string
	ExcludedDirs      []string
	PassagePrefix     string
}

// Pipeline orchestrates document ingestion.
type Pipeline struct {
	vectorStore vectorstore.VectorStore
	embedder    embed.Embedder
	chunker     Chunker
	config      PipelineConfig
}

// NewPipeline creates an ingestion pipeline.
func NewPipeline(vs vectorstore.VectorStore, emb embed.Embedder, chunker Chunker, cfg PipelineConfig) *Pipeline {
	return &Pipeline{
		vectorStore: vs,
		embedder:    emb,
		chunker:     chunker,
		config:      cfg,
	}
}

// Ingest processes all eligible files in a directory.
func (p *Pipeline) Ingest(ctx context.Context, dir string, drop bool, dimension int) (*IngestResult, error) {
	start := time.Now()

	// Validate directory
	if err := ValidateDirectory(dir, p.config.AllowedDirs); err != nil {
		return nil, err
	}

	// Drop and recreate if requested
	if drop {
		slog.Warn("dropping and recreating tables for ingest")
		if err := p.vectorStore.DropAndRecreateTables(ctx, dimension); err != nil {
			return nil, fmt.Errorf("drop tables: %w", err)
		}
	}

	// Walk directory
	entries, err := Walk(dir, WalkOptions{
		AllowedDirs:       p.config.AllowedDirs,
		AllowedExtensions: p.config.AllowedExtensions,
		ExcludedDirs:      p.config.ExcludedDirs,
		MaxFileSize:       p.config.MaxFileSize,
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	if len(entries) == 0 {
		slog.Warn("no eligible files found in directory", "dir", dir)
		return &IngestResult{DurationSeconds: time.Since(start).Seconds()}, nil
	}

	result := &IngestResult{}

	for _, entry := range entries {
		if err := p.processFile(ctx, entry, result); err != nil {
			// ERR-16: embed server unreachable (connection refused, DNS, timeout) -> halt immediately
			if errors.Is(err, embed.ErrUnavailable) {
				slog.Error("embed server unreachable, halting ingest", "error", err)
				result.DurationSeconds = time.Since(start).Seconds()
				return result, err
			}
			// ERR-15: per-file error (including embed 500s) -> warn, skip, continue
			slog.Warn("per-file error, skipping", "path", entry.Path, "error", err)
			result.Errors++
		}
	}

	// Create IVFFlat index if needed
	if drop {
		count, err := p.vectorStore.GetChunkCount(ctx)
		if err == nil && count >= 100 {
			if err := p.vectorStore.CreateIVFFlatIndex(ctx); err != nil {
				slog.Warn("failed to create IVFFlat index", "error", err)
			}
		}
	}

	// Write build metadata
	metadata := map[string]string{
		"total_documents": fmt.Sprintf("%d", result.DocumentsProcessed),
		"total_chunks":    fmt.Sprintf("%d", result.ChunksCreated),
		"duration":        fmt.Sprintf("%.2fs", time.Since(start).Seconds()),
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}
	p.vectorStore.WriteBuildMetadata(ctx, metadata)

	result.DurationSeconds = time.Since(start).Seconds()
	return result, nil
}

func (p *Pipeline) processFile(ctx context.Context, entry FileEntry, result *IngestResult) error {
	// Compute content hash (ING-08)
	hash := sha256.Sum256(entry.Content)
	contentHash := fmt.Sprintf("%x", hash[:8]) // first 16 hex chars

	// Check for existing document
	existing, err := p.vectorStore.GetDocumentByPath(ctx, entry.Path)
	if err != nil {
		return fmt.Errorf("checking existing document: %w", err)
	}

	if existing != nil && existing.ContentHash == contentHash {
		// Unchanged — skip (ING-08)
		return nil
	}

	// Chunk the file
	title, chunks, err := p.chunker.ChunkFile(entry.Path, entry.Content, p.config.ChunkSize)
	if err != nil {
		return fmt.Errorf("chunking: %w", err)
	}

	doc := &vectorstore.Document{
		SourcePath:  entry.Path,
		Title:       title,
		Content:     string(entry.Content),
		ContentHash: contentHash,
		TokenCount:  estimateTokens(string(entry.Content)),
	}

	if existing != nil {
		// Update existing document
		doc.ID = existing.ID
		if err := p.vectorStore.UpdateDocument(ctx, doc); err != nil {
			return fmt.Errorf("updating document: %w", err)
		}
		if err := p.vectorStore.DeleteChunksByDocumentID(ctx, doc.ID); err != nil {
			return fmt.Errorf("deleting old chunks: %w", err)
		}
	} else {
		// Insert new document
		id, err := p.vectorStore.InsertDocument(ctx, doc)
		if err != nil {
			return fmt.Errorf("inserting document: %w", err)
		}
		doc.ID = id
	}

	result.DocumentsProcessed++

	if len(chunks) == 0 {
		slog.Warn("file produced zero chunks", "path", entry.Path)
		return nil
	}

	// Embed chunks in batches (ING-07)
	filename := filepath.Base(entry.Path)
	for batchStart := 0; batchStart < len(chunks); batchStart += p.config.BatchSize {
		batchEnd := batchStart + p.config.BatchSize
		if batchEnd > len(chunks) {
			batchEnd = len(chunks)
		}
		batch := chunks[batchStart:batchEnd]

		// Build embed texts
		texts := make([]string, len(batch))
		for i, c := range batch {
			texts[i] = BuildEmbedText(p.config.PassagePrefix, filename, c.HeadingContext, c.Content)
		}

		embeddings, err := p.embedder.Embed(ctx, texts)
		if err != nil {
			return fmt.Errorf("embedding batch: %w", err)
		}

		// Convert to vectorstore chunks
		vsChunks := make([]vectorstore.Chunk, len(batch))
		for i, c := range batch {
			vsChunks[i] = vectorstore.Chunk{
				DocumentID:     doc.ID,
				ChunkIndex:     batchStart + i,
				Content:        c.Content,
				TokenCount:     c.TokenCount,
				HeadingContext: c.HeadingContext,
				ChunkType:      c.ChunkType,
				Embedding:      embeddings[i],
			}
		}

		if err := p.vectorStore.InsertChunks(ctx, vsChunks); err != nil {
			return fmt.Errorf("inserting chunks: %w", err)
		}

		result.ChunksCreated += len(vsChunks)
	}

	return nil
}
