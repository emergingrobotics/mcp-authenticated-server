package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	maxResponseBody = 4 << 20 // 4 MiB (ENH-10)
	clientTimeout   = 30 * time.Second
)

// Client calls an HTTP /rerank endpoint.
type Client struct {
	host       string
	httpClient *http.Client
}

// NewClient creates a reranker HTTP client.
func NewClient(host string) *Client {
	return &Client{
		host: host,
		httpClient: &http.Client{
			Timeout: clientTimeout,
		},
	}
}

type rerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

type rerankResponse struct {
	Results []RerankResult `json:"results"`
}

func (c *Client) Rerank(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
	reqBody := rerankRequest{
		Query:     query,
		Documents: documents,
		TopN:      topN,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling rerank request: %w", err)
	}

	url := c.host + "/rerank"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("reranker request failed, falling back to RRF", "error", err)
		return nil, nil // fall back (ERR-07)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		slog.Warn("reading reranker response failed", "error", err)
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		slog.Warn("reranker returned non-200", "status", resp.StatusCode)
		return nil, nil
	}

	var rerankResp rerankResponse
	if err := json.Unmarshal(respBody, &rerankResp); err != nil {
		slog.Warn("parsing reranker response failed", "error", err)
		return nil, nil
	}

	return rerankResp.Results, nil
}
