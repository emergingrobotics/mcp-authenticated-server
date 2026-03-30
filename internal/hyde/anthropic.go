package hyde

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const defaultSystemPrompt = "You are a documentation assistant. Write a 2-3 sentence answer to the following question as if it appeared verbatim in the documentation. Be specific and include exact values, constants, or commands if relevant. Do not hedge or qualify."

const maxTokens int64 = 256

// AnthropicGenerator uses Claude to expand queries via HyDE.
type AnthropicGenerator struct {
	client       anthropic.Client
	model        string
	systemPrompt string
}

// NewAnthropicGenerator creates a HyDE generator using the Anthropic API.
// Returns a NoopGenerator if ANTHROPIC_API_KEY is not set (ENH-05).
func NewAnthropicGenerator(model, systemPrompt, baseURL string) Generator {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		slog.Warn("ANTHROPIC_API_KEY not set, HyDE will use raw query passthrough")
		return &NoopGenerator{}
	}

	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	client := anthropic.NewClient(opts...)

	return &AnthropicGenerator{
		client:       client,
		model:        model,
		systemPrompt: systemPrompt,
	}
}

func (g *AnthropicGenerator) Generate(ctx context.Context, query string) (string, bool, error) {
	resp, err := g.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     g.model,
		MaxTokens: maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: g.systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(fmt.Sprintf("Question: %s", query)),
			),
		},
	})
	if err != nil {
		// Fall back to raw query on failure (ERR-06)
		slog.Warn("HyDE generation failed, falling back to raw query", "error", err)
		return query, false, nil
	}

	if len(resp.Content) == 0 {
		slog.Warn("HyDE returned empty content, falling back to raw query")
		return query, false, nil
	}

	hypothesis := ""
	for _, block := range resp.Content {
		if block.Type == "text" {
			hypothesis += block.Text
		}
	}

	if hypothesis == "" {
		return query, false, nil
	}

	return hypothesis, true, nil
}
