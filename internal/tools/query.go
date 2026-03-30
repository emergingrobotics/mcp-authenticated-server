package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/querystore"
)

type queryParams struct {
	Query  string        `json:"query"`
	Params []interface{} `json:"params"`
	Limit  int           `json:"limit"`
}

// NewQueryTool creates the query_data tool definition.
func NewQueryTool(store querystore.QueryStore, defaultLimit, maxLimit int, timeout time.Duration) ToolDef {
	return ToolDef{
		Name:        "query_data",
		Description: "Execute a read-only SQL query against the database and return structured results.",
		InputSchema: json.RawMessage(fmt.Sprintf(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "SQL SELECT query to execute"},
				"params": {"type": "array", "description": "Bind parameters for the query"},
				"limit": {"type": "integer", "description": "Maximum rows to return (default %d, max %d)"}
			},
			"required": ["query"]
		}`, defaultLimit, maxLimit)),
		Handler: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var p queryParams
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid parameters: %w", err)
			}

			if p.Query == "" {
				return nil, fmt.Errorf("query is required")
			}

			// SQL safety validation (SQL-06)
			if err := querystore.ValidateQuery(p.Query); err != nil {
				return nil, err
			}

			if p.Limit <= 0 {
				p.Limit = defaultLimit
			}
			if p.Limit > maxLimit {
				p.Limit = maxLimit
			}

			result, err := store.ExecuteReadOnly(ctx, p.Query, p.Params, p.Limit, timeout)
			if err != nil {
				// Log details server-side (SQL-07)
				slog.Warn("query execution error", "error", err)
				return nil, fmt.Errorf("query execution failed")
			}

			return result, nil
		},
	}
}
