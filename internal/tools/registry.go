package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/emergingrobotics/mcp-authenticated-server/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolHandler is the function signature for MCP tool handlers.
type ToolHandler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// ToolDef defines a tool to be registered.
type ToolDef struct {
	Name        string
	Description string
	InputSchema json.RawMessage // JSON Schema as raw JSON
	Handler     ToolHandler
}

// Register adds a tool to the MCP server with authorization and logging.
func Register(server *mcp.Server, authorizer auth.Authorizer, def ToolDef) {
	server.AddTool(
		&mcp.Tool{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: def.InputSchema,
		},
		func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()
			sub := auth.SubjectFromContext(ctx)

			// Per-tool authorization
			if err := authorizer.Authorize(ctx, def.Name); err != nil {
				slog.Info("tool call denied",
					"tool", def.Name,
					"user", sub,
					"error", err,
				)
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("forbidden: %v", err)},
					},
					IsError: true,
				}, nil
			}

			// Call handler with raw arguments
			result, err := def.Handler(ctx, request.Params.Arguments)

			duration := time.Since(start)

			if err != nil {
				slog.Info("tool call failed",
					"tool", def.Name,
					"user", sub,
					"duration", duration,
					"error", err,
				)
				return &mcp.CallToolResult{
					Content: []mcp.Content{
						&mcp.TextContent{Text: err.Error()},
					},
					IsError: true,
				}, nil
			}

			slog.Info("tool call success",
				"tool", def.Name,
				"user", sub,
				"duration", duration,
			)

			resultJSON, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("marshaling result: %w", err)
			}

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: string(resultJSON)},
				},
			}, nil
		},
	)
}
