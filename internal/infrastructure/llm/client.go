package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
)

type Config struct {
	BaseURL    string
	APIKey     string
	Model      string
	Timeout    time.Duration
	MaxTokens  int
	EmbedURL   string
	EmbedModel string
}

type Client struct {
	cfg Config
	hc  *http.Client
}

func NewClient(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 115 * time.Second
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 1500
	}
	if cfg.EmbedURL == "" {
		cfg.EmbedURL = cfg.BaseURL
	}
	return &Client{cfg: cfg, hc: &http.Client{Timeout: cfg.Timeout}}
}

func (c *Client) Complete(ctx context.Context, messages []nexxa.Message, jsonMode bool) (string, error) {
	payload := map[string]any{
		"model":      c.cfg.Model,
		"messages":   messages,
		"max_tokens": c.cfg.MaxTokens,
	}
	if jsonMode {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: %v", nexxa.ErrUpstreamUnavailable, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(c.cfg.BaseURL, "/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("%w: %v", nexxa.ErrUpstreamUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	raw, err := c.doRaw(req)
	if err != nil {
		return "", err
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("%w: invalid response: %v", nexxa.ErrUpstreamUnavailable, err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("%w: invalid response: empty choices", nexxa.ErrUpstreamUnavailable)
	}
	return out.Choices[0].Message.Content, nil
}

func (c *Client) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	payload := map[string]any{
		"model": c.cfg.EmbedModel,
		"input": inputs,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", nexxa.ErrUpstreamUnavailable, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(c.cfg.EmbedURL, "/embeddings"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", nexxa.ErrUpstreamUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	raw, err := c.doRaw(req)
	if err != nil {
		return nil, err
	}

	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%w: invalid response: %v", nexxa.ErrUpstreamUnavailable, err)
	}
	vecs := make([][]float32, 0, len(out.Data))
	for i, d := range out.Data {
		if len(d.Embedding) == 0 {
			return nil, fmt.Errorf("%w: invalid response: empty embedding %d", nexxa.ErrUpstreamUnavailable, i)
		}
		vecs = append(vecs, d.Embedding)
	}
	return vecs, nil
}

func (c *Client) doRaw(req *http.Request) ([]byte, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			return nil, nexxa.ErrUpstreamTimeout
		}
		return nil, fmt.Errorf("%w: %v", nexxa.ErrUpstreamUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", nexxa.ErrUpstreamUnavailable, resp.StatusCode)
	}

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("%w: read response: %v", nexxa.ErrUpstreamUnavailable, err)
	}
	return buf.Bytes(), nil
}

func joinURL(base, path string) string {
	return strings.TrimRight(base, "/") + path
}

func isTimeout(err error) bool {
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}