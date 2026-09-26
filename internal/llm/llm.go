// Package llm asks the language model on this machine, through Docker Model
// Runner's OpenAI-compatible API. It never calls a paid API.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// DefaultModel reads German and English pages well and fits a Mac with
// 32 GB of memory with room to spare.
const DefaultModel = "ai/gemma3:12b-q4_K_M"

// DefaultURL is Docker Model Runner seen from the Mac itself. Inside a
// container it is http://model-runner.docker.internal/engines/v1, which
// compose.yaml sets.
const DefaultURL = "http://localhost:12434/engines/v1"

// Client talks to one model. It asks one question at a time: several at
// once only share the same graphics chip and need more memory, which made
// the model fail on Tim's Mac.
type Client struct {
	URL   string
	Model string
	HTTP  *http.Client
	// Timeout caps one question. Default 3 minutes.
	Timeout time.Duration

	once  sync.Once
	slots chan struct{}
}

// FromEnv reads LLM_URL and LLM_MODEL, with the defaults above.
func FromEnv() *Client {
	c := &Client{URL: os.Getenv("LLM_URL"), Model: os.Getenv("LLM_MODEL")}
	if c.URL == "" {
		c.URL = DefaultURL
	}
	if c.Model == "" {
		c.Model = DefaultModel
	}
	return c
}

// FromEnvIfSet is FromEnv where LLM_URL is set, and nil elsewhere, so a
// server without a model never waits for one.
func FromEnvIfSet() *Client {
	if os.Getenv("LLM_URL") == "" {
		return nil
	}
	return FromEnv()
}

// ErrFailed means the model answered with an error of its own or took too
// long, as it does when the machine runs short of memory.
var ErrFailed = errors.New("the language model failed")

// Unavailable reports whether an error means the model cannot answer now,
// so the work waits for a later run instead of being retried at once.
func Unavailable(err error) bool {
	return errors.Is(err, ErrUnreachable) || errors.Is(err, ErrFailed)
}

// ErrUnreachable means no model answered at the address.
var ErrUnreachable = errors.New("no language model answers. Is Docker Model Runner on? Turn it on with: docker desktop enable model-runner")

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type request struct {
	Model          string    `json:"model"`
	Messages       []message `json:"messages"`
	Temperature    float64   `json:"temperature"`
	ResponseFormat any       `json:"response_format"`
}

type response struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// chatJSON sends one conversation and decodes the answer, which the schema
// forces into shape, into out.
func (c *Client) chatJSON(ctx context.Context, system, user, name string, schema, out any) (tokens int, err error) {
	c.once.Do(func() { c.slots = make(chan struct{}, 1) })
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body, err := json.Marshal(request{
		Model:       c.Model,
		Temperature: 0,
		Messages:    []message{{Role: "system", Content: system}, {Role: "user", Content: user}},
		ResponseFormat: map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": name, "strict": true, "schema": schema},
		},
	})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		if parent.Err() != nil {
			return 0, parent.Err()
		}
		if ctx.Err() != nil {
			return 0, fmt.Errorf("%w: no answer within %s", ErrFailed, timeout)
		}
		return 0, fmt.Errorf("%w (%v)", ErrUnreachable, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return 0, err
	}
	if res.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		if res.StatusCode == http.StatusNotFound && strings.Contains(strings.ToLower(msg), "model") {
			return 0, fmt.Errorf("the model %s is not on this machine yet. Get it with: docker model pull %s", c.Model, c.Model)
		}
		if res.StatusCode >= 500 {
			return 0, fmt.Errorf("%w, it answered %d: %s", ErrFailed, res.StatusCode, msg)
		}
		return 0, fmt.Errorf("the model answered %d: %s", res.StatusCode, msg)
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return 0, fmt.Errorf("the model's answer is not JSON: %w", err)
	}
	if len(r.Choices) == 0 {
		return 0, errors.New("the model gave no answer")
	}
	if err := json.Unmarshal([]byte(r.Choices[0].Message.Content), out); err != nil {
		return 0, fmt.Errorf("the model's answer does not have the asked shape: %w", err)
	}
	return r.Usage.PromptTokens + r.Usage.CompletionTokens, nil
}
