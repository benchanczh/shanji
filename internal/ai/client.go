// Package ai is the LLM adapter layer. Provider-agnostic by design:
// endpoint, key and model tiers come from configuration, and any
// Anthropic-compatible endpoint works (Anthropic itself, or Kimi via
// https://api.moonshot.ai/anthropic).
//
// Per the architecture red line, the LLM only appears at the edges:
// recipe generation, intent parsing and translation. The planner
// never calls it.
package ai

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type Client struct {
	llm       anthropic.Client
	modelFast string // intent parsing, translation
	modelGen  string // recipe generation
}

// NewFromEnv builds a client from SHANJI_AI_* environment variables.
func NewFromEnv() (*Client, error) {
	key := os.Getenv("SHANJI_AI_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("SHANJI_AI_API_KEY is not set")
	}
	opts := []option.RequestOption{option.WithAPIKey(key)}
	if base := os.Getenv("SHANJI_AI_BASE_URL"); base != "" {
		opts = append(opts, option.WithBaseURL(base))
	}
	c := &Client{
		llm:       anthropic.NewClient(opts...),
		modelFast: envOr("SHANJI_AI_MODEL_FAST", "claude-haiku-4-5"),
		modelGen:  envOr("SHANJI_AI_MODEL_GEN", "claude-sonnet-4-6"),
	}
	return c, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// complete sends a single-turn prompt and returns the text response.
// JSON-mode prompting (rather than tool use) keeps us portable across
// Anthropic-compatible providers; the schema validation layer in
// seedjson is the actual correctness gate.
func (c *Client) complete(ctx context.Context, model, system, prompt string, maxTokens int64) (string, error) {
	msg, err := c.llm.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, block := range msg.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

// ExtractJSON pulls the first JSON object out of a model response,
// tolerating markdown code fences and surrounding prose.
func ExtractJSON(s string) string {
	if i := strings.Index(s, "```json"); i >= 0 {
		s = s[i+len("```json"):]
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
	} else if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}
