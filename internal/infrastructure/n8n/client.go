package n8n

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/chat"
	"github.com/aussenseiter-VsRB/JHIC-BE/internal/domain/nexxa/match/content"
)

const nexxaHeaderName = "X-JHIC-Secret"

type Config struct {
	BaseURL      string
	ChatPath     string
	ChatUsername string
	ChatPassword string
	NexxaPath    string
	NexxaSecret  string
	Timeout      time.Duration
}

type Client struct {
	cfg Config
	hc  *http.Client
}

func NewClient(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 115 * time.Second
	}
	return &Client{cfg: cfg, hc: &http.Client{Timeout: cfg.Timeout}}
}

func (c *Client) Chat(ctx context.Context, chatInput, sessionID string) (*nexxa.ChatResponse, error) {
	payload, err := json.Marshal(chat.ChatRequest{ChatInput: chatInput, SessionID: sessionID})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", nexxa.ErrN8NUnavailable, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+c.cfg.ChatPath, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", nexxa.ErrN8NUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.ChatUsername != "" {
		req.SetBasicAuth(c.cfg.ChatUsername, c.cfg.ChatPassword)
	}

	var out nexxa.ChatResponse
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) NexxaMatch(ctx context.Context, answers []string) (string, error) {
	payload := map[string]string{}
	for i := 0; i < len(answers) && i < content.NexxaAnswerCount; i++ {
		payload[fmt.Sprintf("jawaban_%d", i+1)] = answers[i]
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: %v", nexxa.ErrN8NUnavailable, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+c.cfg.NexxaPath, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("%w: %v", nexxa.ErrN8NUnavailable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.NexxaSecret != "" {
		req.Header.Set(nexxaHeaderName, c.cfg.NexxaSecret)
	}

	raw, err := c.doRaw(req)
	if err != nil {
		return "", err
	}
	return raw, nil
}

func (c *Client) do(req *http.Request, out any) error {
	raw, err := c.doRaw(req)
	if err != nil {
		return err
	}

	body := []byte(raw)
	if err := json.Unmarshal(body, out); err == nil {
		return nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return fmt.Errorf("%w: invalid response: %v", nexxa.ErrN8NUnavailable, err)
	}
	if len(items) == 0 {
		return fmt.Errorf("%w: invalid response: empty items", nexxa.ErrN8NUnavailable)
	}
	if err := json.Unmarshal(items[0], out); err != nil {
		return fmt.Errorf("%w: invalid response: %v", nexxa.ErrN8NUnavailable, err)
	}
	return nil
}

func (c *Client) doRaw(req *http.Request) (string, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", nexxa.ErrN8NTimeout
		}
		return "", fmt.Errorf("%w: %v", nexxa.ErrN8NUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: status %d", nexxa.ErrN8NUnavailable, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%w: read response: %v", nexxa.ErrN8NUnavailable, err)
	}
	return string(body), nil
}