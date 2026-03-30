package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Rerank(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rerank" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var req rerankRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := rerankResponse{
			Results: []RerankResult{
				{Index: 1, RelevanceScore: 0.95},
				{Index: 0, RelevanceScore: 0.80},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	results, err := c.Rerank(context.Background(), "test query", []string{"doc1", "doc2"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].RelevanceScore != 0.95 {
		t.Errorf("expected score 0.95, got %f", results[0].RelevanceScore)
	}
}

func TestClient_Rerank_Error_FallsBack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(server.URL)
	results, err := c.Rerank(context.Background(), "test", []string{"doc"}, 1)
	if err != nil {
		t.Fatal("expected no error (fallback)")
	}
	if results != nil {
		t.Error("expected nil results (fallback to RRF)")
	}
}

func TestNoopReranker(t *testing.T) {
	n := &NoopReranker{}
	results, err := n.Rerank(context.Background(), "test", []string{"doc"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Error("expected nil results from noop")
	}
}
