package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/search"
)

type searchParams struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// NewSearchTool creates the search_documents tool definition.
func NewSearchTool(pipeline *search.Pipeline) ToolDef {
	return ToolDef{
		Name:        "search_documents",
		Description: "Search the document corpus using semantic and full-text search. Returns relevant chunks with context.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "The search query"},
				"limit": {"type": "integer", "description": "Maximum number of results (1-20, default 5)", "minimum": 1, "maximum": 20, "default": 5}
			},
			"required": ["query"]
		}`),
		Handler: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var p searchParams
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid parameters: %w", err)
			}

			if p.Query == "" {
				return nil, fmt.Errorf("query is required")
			}

			if p.Limit <= 0 {
				p.Limit = 5
			}
			if p.Limit > 20 {
				p.Limit = 20
			}

			return pipeline.Search(ctx, p.Query, p.Limit)
		},
	}
}
