package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/vecmath"
)

// ErrUnavailable indicates the embedding server cannot be reached (connection refused, DNS failure, timeout).
// Distinct from a server-side error (HTTP 500), which means the server is reachable but rejected the request.
var ErrUnavailable = errors.New("embedding server unavailable")

const (
	maxResponseBody = 4 << 20 // 4 MiB (SEC-04)
	clientTimeout   = 30 * time.Second
)

// Client is an OpenAI-compatible embedding HTTP client.
type Client struct {
	host          string
	model         string
	queryPrefix   string
	passagePrefix string
	httpClient    *http.Client
}

// NewClient creates an embedding client.
func NewClient(host, model, queryPrefix, passagePrefix string) *Client {
	return &Client{
		host:          host,
		model:         model,
		queryPrefix:   queryPrefix,
		passagePrefix: passagePrefix,
		httpClient: &http.Client{
			Timeout: clientTimeout,
		},
	}
}

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return c.EmbedWithPrefix(ctx, texts, "")
}

func (c *Client) EmbedWithPrefix(ctx context.Context, texts []string, prefix string) ([][]float32, error) {
	inputs := make([]string, len(texts))
	for i, t := range texts {
		inputs[i] = prefix + t
	}

	reqBody := embeddingRequest{
		Model: c.model,
		Input: inputs,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling embed request: %w", err)
	}

	url := c.host + "/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Connection-level failure: server unreachable (ERR-16: halt ingest)
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("reading embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Server returned an error (e.g., input too large) -- this is a per-request
		// failure, not a connectivity issue. Callers should skip the file, not halt.
		return nil, fmt.Errorf("embed server returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var embedResp embeddingResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("parsing embed response: %w", err)
	}

	if len(embedResp.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(embedResp.Data))
	}

	embeddings := make([][]float32, len(embedResp.Data))
	for i, d := range embedResp.Data {
		vec := d.Embedding
		vecmath.L2Normalize(vec) // VEC-06
		embeddings[i] = vec
	}

	return embeddings, nil
}

// QueryPrefix returns the configured query prefix.
func (c *Client) QueryPrefix() string {
	return c.queryPrefix
}

// PassagePrefix returns the configured passage prefix.
func (c *Client) PassagePrefix() string {
	return c.passagePrefix
}
