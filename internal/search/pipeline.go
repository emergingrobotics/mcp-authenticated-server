package search

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/embed"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/guardrails"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/hyde"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/rerank"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/vectorstore"
)

// SearchResult is the response from the search pipeline.
type SearchResult struct {
	Results         []vectorstore.ChunkResult `json:"results"`
	TotalChunksInDB int                       `json:"total_chunks_in_db"`
}

// PipelineConfig holds search pipeline tuning parameters.
type PipelineConfig struct {
	Probes            int
	RetrievalPoolSize int
	RRFConstant       int
	QueryPrefix       string
	PassagePrefix     string
}

// Pipeline orchestrates the full search flow per REQUIREMENTS section 14.
type Pipeline struct {
	embedder    embed.Embedder
	vectorStore vectorstore.VectorStore
	guard       *guardrails.Guardrails
	hydeGen     hyde.Generator
	reranker    rerank.Reranker
	config      PipelineConfig
}

// NewPipeline creates a search pipeline.
func NewPipeline(
	embedder embed.Embedder,
	vs vectorstore.VectorStore,
	guard *guardrails.Guardrails,
	hydeGen hyde.Generator,
	reranker rerank.Reranker,
	cfg PipelineConfig,
) *Pipeline {
	return &Pipeline{
		embedder:    embedder,
		vectorStore: vs,
		guard:       guard,
		hydeGen:     hydeGen,
		reranker:    reranker,
		config:      cfg,
	}
}

// Search executes the full search pipeline.
func (p *Pipeline) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	// Check if DB has any chunks
	totalChunks, err := p.vectorStore.GetChunkCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting chunk count: %w", err)
	}
	if totalChunks == 0 {
		return &SearchResult{
			Results:         []vectorstore.ChunkResult{},
			TotalChunksInDB: 0,
		}, nil
	}

	// Step 4: HyDE expansion
	embedText := query
	prefix := p.config.QueryPrefix
	if p.hydeGen != nil {
		hypothesis, isPassage, err := p.hydeGen.Generate(ctx, query)
		if err != nil {
			slog.Warn("HyDE generation failed", "error", err)
		} else if isPassage {
			embedText = hypothesis
			prefix = p.config.PassagePrefix
		}
	}

	// Step 5: Embed
	embeddings, err := p.embedder.EmbedWithPrefix(ctx, []string{embedText}, prefix)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}
	queryEmbedding := embeddings[0]
	// Step 6: L2 normalization done by embed client

	// Step 7: Level 1 guardrail (pre-DB)
	if err := p.guard.CheckTopicRelevance(queryEmbedding); err != nil {
		return nil, err
	}

	// Step 8: Parallel retrieval
	poolSize := p.config.RetrievalPoolSize

	// Set IVFFlat probes if index exists
	p.vectorStore.SetIVFFlatProbes(ctx, p.config.Probes)

	var knnResults, ftsResults []vectorstore.ChunkResult
	var knnErr, ftsErr error
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		knnResults, knnErr = p.vectorStore.SearchKNN(ctx, queryEmbedding, poolSize)
	}()
	go func() {
		defer wg.Done()
		ftsResults, ftsErr = p.vectorStore.SearchFTS(ctx, query, poolSize)
	}()
	wg.Wait()

	if knnErr != nil {
		return nil, fmt.Errorf("KNN search failed: %w", knnErr)
	}
	if ftsErr != nil {
		return nil, fmt.Errorf("FTS search failed: %w", ftsErr)
	}

	// Step 9: RRF merge
	merged := MergeRRF(knnResults, ftsResults, p.config.RRFConstant)

	if len(merged) == 0 {
		return &SearchResult{
			Results:         []vectorstore.ChunkResult{},
			TotalChunksInDB: totalChunks,
		}, nil
	}

	// Step 10: Reranking
	if p.reranker != nil {
		documents := make([]string, len(merged))
		for i, r := range merged {
			docText := r.SourcePath
			if r.HeadingContext != "" {
				docText += ": " + r.HeadingContext
			}
			docText += "\n\n" + r.Content
			documents[i] = docText
		}

		rerankResults, err := p.reranker.Rerank(ctx, query, documents, len(documents))
		if err != nil {
			slog.Warn("reranker error, keeping RRF scores", "error", err)
		} else if rerankResults != nil {
			// Replace scores with reranker scores
			scoreMap := make(map[int]float64)
			for _, rr := range rerankResults {
				scoreMap[rr.Index] = rr.RelevanceScore
			}
			for i := range merged {
				if score, ok := scoreMap[i]; ok {
					merged[i].Score = score
				} else {
					merged[i].Score = 0
				}
			}
			// Re-sort by reranker scores
			sortByScore(merged)
		}
	}

	// Step 11: Level 2 guardrail (post-retrieval)
	if err := p.guard.CheckMatchScore(merged[0].Score); err != nil {
		slog.Debug("Level 2 guardrail rejection", "best_score", merged[0].Score)
		return nil, err
	}

	// Step 12: Truncate to limit
	if len(merged) > limit {
		merged = merged[:limit]
	}

	results := make([]vectorstore.ChunkResult, len(merged))
	for i, r := range merged {
		r.ChunkResult.Score = r.Score
		results[i] = r.ChunkResult
	}

	return &SearchResult{
		Results:         results,
		TotalChunksInDB: totalChunks,
	}, nil
}

func sortByScore(results []RankedResult) {
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score > results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
}
