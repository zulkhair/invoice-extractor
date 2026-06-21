// Package ollama wraps the official github.com/ollama/ollama/api client with a
// small Chat surface (text + vision), plus introspection for health and the
// pull-verification path. Kept thin so the wire details stay in one place — the
// api package's field names have drifted across releases (spec Task 1 warning).
package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

type Client struct {
	api     *api.Client
	timeout time.Duration
}

type ChatResult struct {
	Content   string
	Model     string
	Latency   time.Duration
	EvalCount int
}

func New(host string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("bad OLLAMA_HOST %q: %w", host, err)
	}
	return &Client{api: api.NewClient(u, http.DefaultClient), timeout: timeout}, nil
}

// Ping reports whether the Ollama server is reachable.
func (c *Client) Ping(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.api.List(ctx)
	return err == nil
}

// ListModels returns the names of locally available models.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	resp, err := c.api.List(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(resp.Models))
	for _, m := range resp.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// Chat runs a single-shot chat (no streaming exposed). images are raw PNG bytes.
func (c *Client) Chat(
	ctx context.Context,
	model, system, user string,
	images [][]byte,
	jsonFormat bool,
	temperature float64,
) (ChatResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	userMsg := api.Message{Role: "user", Content: user}
	if len(images) > 0 {
		imgs := make([]api.ImageData, len(images))
		for i, b := range images {
			imgs[i] = api.ImageData(b)
		}
		userMsg.Images = imgs
	}

	stream := false
	req := &api.ChatRequest{
		Model:    model,
		Messages: []api.Message{{Role: "system", Content: system}, userMsg},
		Stream:   &stream,
		Options:  map[string]any{"temperature": temperature},
	}
	if jsonFormat {
		req.Format = json.RawMessage(`"json"`)
	}

	var sb strings.Builder
	var evalCount int
	start := time.Now()
	err := c.api.Chat(ctx, req, func(r api.ChatResponse) error {
		sb.WriteString(r.Message.Content)
		if r.Done {
			evalCount = r.EvalCount
		}
		return nil
	})
	if err != nil {
		return ChatResult{}, fmt.Errorf("ollama chat (%s): %w", model, err)
	}
	return ChatResult{
		Content:   sb.String(),
		Model:     model,
		Latency:   time.Since(start),
		EvalCount: evalCount,
	}, nil
}
