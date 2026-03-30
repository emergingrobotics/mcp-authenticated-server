package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

func applySchema(ctx context.Context, db *sql.DB, dimension int) error {
	statements := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,

		`CREATE TABLE IF NOT EXISTS documents (
			id BIGSERIAL PRIMARY KEY,
			source_path TEXT UNIQUE NOT NULL,
			title TEXT,
			content TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			token_count INTEGER,
			created_at TIMESTAMPTZ DEFAULT NOW()
		)`,

		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS chunks (
			id BIGSERIAL PRIMARY KEY,
			document_id BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			chunk_index INTEGER NOT NULL,
			content TEXT NOT NULL,
			token_count INTEGER,
			heading_context TEXT,
			chunk_type TEXT,
			embedding vector(%d),
			content_fts tsvector GENERATED ALWAYS AS (to_tsvector('english', content)) STORED,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			UNIQUE(document_id, chunk_index)
		)`, dimension),

		`CREATE INDEX IF NOT EXISTS idx_chunks_content_fts ON chunks USING GIN (content_fts)`,

		`CREATE TABLE IF NOT EXISTS build_metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("schema apply: %w\nstatement: %s", err, stmt)
		}
	}

	return nil
}
