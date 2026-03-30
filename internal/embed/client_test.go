package embed

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Embed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", req.Model)
		}

		resp := embeddingResponse{
			Data: make([]struct {
				Embedding []float32 `json:"embedding"`
			}, len(req.Input)),
		}
		for i := range req.Input {
			resp.Data[i].Embedding = []float32{3, 4} // will be normalized to [0.6, 0.8]
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-model", "query: ", "passage: ")
	embeddings, err := c.Embed(context.Background(), []string{"hello world"})
	if err != nil {
		t.Fatal(err)
	}

	if len(embeddings) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(embeddings))
	}

	// Verify L2 normalization
	vec := embeddings[0]
	var mag float64
	for _, v := range vec {
		mag += float64(v) * float64(v)
	}
	if math.Abs(mag-1.0) > 1e-5 {
		t.Errorf("expected normalized vector (mag ~1.0), got mag=%f", mag)
	}
}

func TestClient_EmbedWithPrefix(t *testing.T) {
	var receivedInput []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embeddingRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedInput = req.Input

		resp := embeddingResponse{
			Data: make([]struct {
				Embedding []float32 `json:"embedding"`
			}, len(req.Input)),
		}
		for i := range req.Input {
			resp.Data[i].Embedding = []float32{1, 0, 0}
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-model", "query: ", "passage: ")
	_, err := c.EmbedWithPrefix(context.Background(), []string{"hello"}, "passage: ")
	if err != nil {
		t.Fatal(err)
	}

	if len(receivedInput) != 1 || receivedInput[0] != "passage: hello" {
		t.Errorf("expected prefix applied, got %v", receivedInput)
	}
}

func TestClient_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-model", "", "")
	_, err := c.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error for 500 status")
	}
}

func TestClient_MismatchedCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := embeddingResponse{
			Data: make([]struct {
				Embedding []float32 `json:"embedding"`
			}, 1), // return 1 when 2 were requested
		}
		resp.Data[0].Embedding = []float32{1, 0}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	c := NewClient(server.URL, "test-model", "", "")
	_, err := c.Embed(context.Background(), []string{"hello", "world"})
	if err == nil {
		t.Fatal("expected error for mismatched count")
	}
}

func TestClient_Prefixes(t *testing.T) {
	c := NewClient("http://localhost", "model", "q: ", "p: ")
	if c.QueryPrefix() != "q: " {
		t.Errorf("expected 'q: ', got %q", c.QueryPrefix())
	}
	if c.PassagePrefix() != "p: " {
		t.Errorf("expected 'p: ', got %q", c.PassagePrefix())
	}
}
