package search

import (
	"sort"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/vectorstore"
)

// RankedResult is a search result with an RRF-merged score.
type RankedResult struct {
	vectorstore.ChunkResult
	Score float64
}

// MergeRRF merges KNN and FTS results using Reciprocal Rank Fusion.
// score(d) = sum(1/(k + rank_i(d))) for each arm where d appears.
// Rank is 1-indexed.
func MergeRRF(knnResults, ftsResults []vectorstore.ChunkResult, kConstant int) []RankedResult {
	type key struct {
		DocumentID int64
		ChunkIndex int
	}

	scores := make(map[key]*RankedResult)

	for rank, r := range knnResults {
		k := key{r.DocumentID, r.ChunkIndex}
		if _, ok := scores[k]; !ok {
			scores[k] = &RankedResult{ChunkResult: r}
		}
		scores[k].Score += 1.0 / float64(kConstant+rank+1)
	}

	for rank, r := range ftsResults {
		k := key{r.DocumentID, r.ChunkIndex}
		if _, ok := scores[k]; !ok {
			scores[k] = &RankedResult{ChunkResult: r}
		}
		scores[k].Score += 1.0 / float64(kConstant+rank+1)
	}

	results := make([]RankedResult, 0, len(scores))
	for _, r := range scores {
		results = append(results, *r)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}
