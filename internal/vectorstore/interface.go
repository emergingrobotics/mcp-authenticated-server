package vectorstore

import "context"

// Document represents an ingested file.
type Document struct {
	ID          int64
	SourcePath  string
	Title       string
	Content     string
	ContentHash string
	TokenCount  int
}

// Chunk represents a document chunk with its embedding.
type Chunk struct {
	DocumentID     int64
	ChunkIndex     int
	Content        string
	TokenCount     int
	HeadingContext string
	ChunkType      string
	Embedding      []float32
}

// ChunkResult is a search result from vector or text search.
type ChunkResult struct {
	Content        string  `json:"content"`
	HeadingContext string  `json:"heading_context"`
	ChunkType      string  `json:"chunk_type"`
	SourcePath     string  `json:"source_path"`
	Title          string  `json:"title"`
	Score          float64 `json:"score"`
	// Internal fields for deduplication
	DocumentID int64 `json:"-"`
	ChunkIndex int   `json:"-"`
}

// VectorStore abstracts vector storage and retrieval operations.
type VectorStore interface {
	InsertDocument(ctx context.Context, doc *Document) (int64, error)
	GetDocumentByPath(ctx context.Context, path string) (*Document, error)
	UpdateDocument(ctx context.Context, doc *Document) error
	DeleteChunksByDocumentID(ctx context.Context, docID int64) error
	InsertChunks(ctx context.Context, chunks []Chunk) error
	SearchKNN(ctx context.Context, embedding []float32, limit int) ([]ChunkResult, error)
	SearchFTS(ctx context.Context, query string, limit int) ([]ChunkResult, error)
	GetChunkCount(ctx context.Context) (int, error)
	DropAndRecreateTables(ctx context.Context, dimension int) error
	CreateIVFFlatIndex(ctx context.Context) error
	SetIVFFlatProbes(ctx context.Context, probes int) error
	WriteBuildMetadata(ctx context.Context, metadata map[string]string) error
}
