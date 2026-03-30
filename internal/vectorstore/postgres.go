package vectorstore

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	pgvector "github.com/pgvector/pgvector-go"
)

// PostgresStore implements VectorStore using PostgreSQL + pgvector.
type PostgresStore struct {
	db        *sql.DB
	dimension int
}

// NewPostgresStore creates a new PostgreSQL vector store.
func NewPostgresStore(db *sql.DB, dimension int) *PostgresStore {
	return &PostgresStore{db: db, dimension: dimension}
}

func (s *PostgresStore) InsertDocument(ctx context.Context, doc *Document) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO documents (source_path, title, content, content_hash, token_count)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		doc.SourcePath, doc.Title, doc.Content, doc.ContentHash, doc.TokenCount,
	).Scan(&id)
	return id, err
}

func (s *PostgresStore) GetDocumentByPath(ctx context.Context, path string) (*Document, error) {
	doc := &Document{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, source_path, title, content, content_hash, token_count
		 FROM documents WHERE source_path = $1`, path,
	).Scan(&doc.ID, &doc.SourcePath, &doc.Title, &doc.Content, &doc.ContentHash, &doc.TokenCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return doc, err
}

func (s *PostgresStore) UpdateDocument(ctx context.Context, doc *Document) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE documents SET title = $1, content = $2, content_hash = $3, token_count = $4
		 WHERE id = $5`,
		doc.Title, doc.Content, doc.ContentHash, doc.TokenCount, doc.ID,
	)
	return err
}

func (s *PostgresStore) DeleteChunksByDocumentID(ctx context.Context, docID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM chunks WHERE document_id = $1`, docID,
	)
	return err
}

func (s *PostgresStore) InsertChunks(ctx context.Context, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	// Batch insert with parameterized query
	valueStrings := make([]string, 0, len(chunks))
	valueArgs := make([]interface{}, 0, len(chunks)*7)

	for i, c := range chunks {
		base := i * 7
		valueStrings = append(valueStrings, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7,
		))
		valueArgs = append(valueArgs,
			c.DocumentID, c.ChunkIndex, c.Content, c.TokenCount,
			c.HeadingContext, c.ChunkType, pgvector.NewVector(c.Embedding),
		)
	}

	query := fmt.Sprintf(
		`INSERT INTO chunks (document_id, chunk_index, content, token_count, heading_context, chunk_type, embedding)
		 VALUES %s`,
		strings.Join(valueStrings, ", "),
	)

	_, err := s.db.ExecContext(ctx, query, valueArgs...)
	return err
}

func (s *PostgresStore) SearchKNN(ctx context.Context, embedding []float32, limit int) ([]ChunkResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.content, c.heading_context, c.chunk_type, d.source_path, d.title,
		        c.embedding <=> $1 AS distance, c.document_id, c.chunk_index
		 FROM chunks c
		 JOIN documents d ON c.document_id = d.id
		 ORDER BY c.embedding <=> $1
		 LIMIT $2`,
		pgvector.NewVector(embedding), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("KNN search: %w", err)
	}
	defer rows.Close()

	var results []ChunkResult
	for rows.Next() {
		var r ChunkResult
		var distance float64
		if err := rows.Scan(&r.Content, &r.HeadingContext, &r.ChunkType,
			&r.SourcePath, &r.Title, &distance, &r.DocumentID, &r.ChunkIndex); err != nil {
			return nil, fmt.Errorf("scanning KNN result: %w", err)
		}
		// Convert cosine distance to similarity score
		r.Score = 1 - distance
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *PostgresStore) SearchFTS(ctx context.Context, query string, limit int) ([]ChunkResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.content, c.heading_context, c.chunk_type, d.source_path, d.title,
		        ts_rank(c.content_fts, plainto_tsquery('english', $1)) AS score,
		        c.document_id, c.chunk_index
		 FROM chunks c
		 JOIN documents d ON c.document_id = d.id
		 WHERE c.content_fts @@ plainto_tsquery('english', $1)
		 ORDER BY score DESC
		 LIMIT $2`,
		query, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("FTS search: %w", err)
	}
	defer rows.Close()

	var results []ChunkResult
	for rows.Next() {
		var r ChunkResult
		if err := rows.Scan(&r.Content, &r.HeadingContext, &r.ChunkType,
			&r.SourcePath, &r.Title, &r.Score, &r.DocumentID, &r.ChunkIndex); err != nil {
			return nil, fmt.Errorf("scanning FTS result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *PostgresStore) GetChunkCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunks`).Scan(&count)
	return count, err
}

func (s *PostgresStore) DropAndRecreateTables(ctx context.Context, dimension int) error {
	statements := []string{
		`DROP TABLE IF EXISTS chunks CASCADE`,
		`DROP TABLE IF EXISTS documents CASCADE`,
		`DROP TABLE IF EXISTS build_metadata CASCADE`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("drop tables: %w", err)
		}
	}
	// Re-apply schema handled by caller via database.Store.ApplySchema
	return nil
}

func (s *PostgresStore) CreateIVFFlatIndex(ctx context.Context) error {
	count, err := s.GetChunkCount(ctx)
	if err != nil {
		return err
	}
	if count < 100 {
		return nil // skip index creation for small datasets
	}

	lists := int(math.Max(10, math.Floor(math.Sqrt(float64(count)))))
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS idx_chunks_embedding ON chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = %d)`,
		lists,
	))
	return err
}

func (s *PostgresStore) SetIVFFlatProbes(ctx context.Context, probes int) error {
	// Only set probes if the IVFFlat index exists
	var indexExists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = 'idx_chunks_embedding')`,
	).Scan(&indexExists)
	if err != nil {
		return err
	}
	if !indexExists {
		return nil
	}
	// Safe: %d only formats integers; SET does not support $1 parameters in PostgreSQL.
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`SET ivfflat.probes = %d`, probes))
	return err
}

func (s *PostgresStore) WriteBuildMetadata(ctx context.Context, metadata map[string]string) error {
	for k, v := range metadata {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO build_metadata (key, value) VALUES ($1, $2)
			 ON CONFLICT (key) DO UPDATE SET value = $2`,
			k, v,
		)
		if err != nil {
			return fmt.Errorf("writing build metadata %q: %w", k, err)
		}
	}
	return nil
}
