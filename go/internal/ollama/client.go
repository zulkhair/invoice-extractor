// Package ollama wraps the official github.com/ollama/ollama/api client with a
// small Chat + Generate surface (text + vision + OCR), plus introspection for health
// and the pull-verification path. Kept thin so the wire details stay in one place —
// the api package's field names have drifted across releases.
package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

// Options are the per-call runtime knobs shared by Chat and Generate.
type Options struct {
	Temperature float64
	NumCtx      int    // >0 to send num_ctx
	NumGPU      int    // <0 = let Ollama decide (omit num_gpu)
	NumPredict  int    // >0 to cap output tokens (OCR repetition guard)
	KeepAlive   string // "", "-1"/"0" (numeric seconds), or a duration like "30m"
	JSONFormat  bool   // request format:"json"
}

func (o Options) toMap() map[string]any {
	m := map[string]any{"temperature": o.Temperature}
	if o.NumCtx > 0 {
		m["num_ctx"] = o.NumCtx
	}
	if o.NumGPU >= 0 {
		m["num_gpu"] = o.NumGPU
	}
	if o.NumPredict > 0 {
		m["num_predict"] = o.NumPredict
	}
	return m
}

// keepAlive parses "-1"/"0"/digits as seconds and "30m"-style strings as a duration.
// A negative duration tells Ollama to keep the model resident forever.
func keepAlive(s string) *api.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		return &api.Duration{Duration: time.Duration(n) * time.Second}
	}
	if d, err := time.ParseDuration(s); err == nil {
		return &api.Duration{Duration: d}
	}
	return nil
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
	opts Options,
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
		Model:     model,
		Messages:  []api.Message{{Role: "system", Content: system}, userMsg},
		Stream:    &stream,
		Options:   opts.toMap(),
		KeepAlive: keepAlive(opts.KeepAlive),
	}
	if opts.JSONFormat {
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

// Generate runs a single-shot /api/generate. Used for OCR transcription: free-text
// output (not JSON), which GLM-OCR and similar models read more reliably via generate
// than via chat.
func (c *Client) Generate(
	ctx context.Context,
	model, prompt string,
	images [][]byte,
	opts Options,
) (ChatResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	stream := false
	req := &api.GenerateRequest{
		Model:     model,
		Prompt:    prompt,
		Stream:    &stream,
		Options:   opts.toMap(),
		KeepAlive: keepAlive(opts.KeepAlive),
	}
	if len(images) > 0 {
		imgs := make([]api.ImageData, len(images))
		for i, b := range images {
			imgs[i] = api.ImageData(b)
		}
		req.Images = imgs
	}
	if opts.JSONFormat {
		req.Format = json.RawMessage(`"json"`)
	}

	var sb strings.Builder
	var evalCount int
	start := time.Now()
	err := c.api.Generate(ctx, req, func(r api.GenerateResponse) error {
		sb.WriteString(r.Response)
		if r.Done {
			evalCount = r.EvalCount
		}
		return nil
	})
	if err != nil {
		return ChatResult{}, fmt.Errorf("ollama generate (%s): %w", model, err)
	}
	return ChatResult{
		Content:   sb.String(),
		Model:     model,
		Latency:   time.Since(start),
		EvalCount: evalCount,
	}, nil
}
