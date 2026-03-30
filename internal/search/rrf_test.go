package search

import (
	"math"
	"testing"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/vectorstore"
)

func makeResult(docID int64, chunkIndex int) vectorstore.ChunkResult {
	return vectorstore.ChunkResult{
		DocumentID: docID,
		ChunkIndex: chunkIndex,
		Content:    "test content",
	}
}

func TestMergeRRF_BothArms(t *testing.T) {
	k := 60
	knn := []vectorstore.ChunkResult{
		makeResult(1, 0), // rank 1
		makeResult(2, 0), // rank 2
	}
	fts := []vectorstore.ChunkResult{
		makeResult(2, 0), // rank 1
		makeResult(3, 0), // rank 2
	}

	results := MergeRRF(knn, fts, k)

	// Doc 2 in KNN at rank 2 (index 1): 1/(60+2)
	// Doc 2 in FTS at rank 1 (index 0): 1/(60+1)
	// Doc 2 total: 1/62 + 1/61
	// Doc 1 only in KNN at rank 1: 1/(60+1) = 1/61
	// Doc 3 only in FTS at rank 2: 1/(60+2) = 1/62

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Doc 2 should be highest (appears in both arms)
	if results[0].DocumentID != 2 {
		t.Errorf("expected doc 2 first, got doc %d", results[0].DocumentID)
	}

	expectedDoc2Score := 1.0/float64(k+2) + 1.0/float64(k+1) // rank 2 in KNN + rank 1 in FTS
	if math.Abs(results[0].Score-expectedDoc2Score) > 1e-10 {
		t.Errorf("expected doc 2 score %f, got %f", expectedDoc2Score, results[0].Score)
	}
}

func TestMergeRRF_SingleArm(t *testing.T) {
	knn := []vectorstore.ChunkResult{
		makeResult(1, 0),
	}
	fts := []vectorstore.ChunkResult{}

	results := MergeRRF(knn, fts, 60)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	expected := 1.0 / float64(60+1)
	if math.Abs(results[0].Score-expected) > 1e-10 {
		t.Errorf("expected score %f, got %f", expected, results[0].Score)
	}
}

func TestMergeRRF_EmptyLists(t *testing.T) {
	results := MergeRRF(nil, nil, 60)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestMergeRRF_ConfigurableK(t *testing.T) {
	knn := []vectorstore.ChunkResult{makeResult(1, 0)}
	fts := []vectorstore.ChunkResult{}

	results10 := MergeRRF(knn, fts, 10)
	results100 := MergeRRF(knn, fts, 100)

	// Lower k gives higher scores for top-ranked items
	if results10[0].Score <= results100[0].Score {
		t.Error("expected lower k to produce higher scores")
	}
}

func TestMergeRRF_SortedDescending(t *testing.T) {
	knn := []vectorstore.ChunkResult{
		makeResult(1, 0), // rank 1
		makeResult(2, 0), // rank 2
		makeResult(3, 0), // rank 3
	}
	fts := []vectorstore.ChunkResult{
		makeResult(3, 0), // rank 1 in FTS
	}

	results := MergeRRF(knn, fts, 60)

	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Error("results not sorted descending")
		}
	}
}
