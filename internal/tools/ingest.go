package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/auth"
	"github.com/emergingrobotics/mcp-authenticated-server/internal/ingest"
)

type ingestParams struct {
	Directory string `json:"directory"`
	Drop      bool   `json:"drop"`
}

// NewIngestTool creates the ingest_documents tool definition.
func NewIngestTool(pipeline *ingest.Pipeline, dimension int) ToolDef {
	return ToolDef{
		Name:        "ingest_documents",
		Description: "Ingest documents from a directory into the vector store. Requires admin group authorization.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"directory": {"type": "string", "description": "Path to the directory to ingest"},
				"drop": {"type": "boolean", "description": "Drop and recreate tables before ingesting (default false)", "default": false}
			},
			"required": ["directory"]
		}`),
		Handler: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			var p ingestParams
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, fmt.Errorf("invalid parameters: %w", err)
			}

			if p.Directory == "" {
				return nil, fmt.Errorf("directory is required")
			}

			// Log destructive operations (OBS-07)
			if p.Drop {
				sub := auth.SubjectFromContext(ctx)
				slog.Warn("ingest with drop=true", "user", sub, "directory", p.Directory)
			}

			result, err := pipeline.Ingest(ctx, p.Directory, p.Drop, dimension)
			if err != nil {
				return nil, err
			}

			return result, nil
		},
	}
}
