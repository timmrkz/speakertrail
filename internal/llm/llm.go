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
	"time"
)

// DefaultModel reads German and English pages well and fits a Mac with
// 32 GB of memory with room to spare.
const DefaultModel = "ai/gemma3:12b-q4_K_M"

// DefaultURL is Docker Model Runner seen from the Mac itself. Inside a
// container it is http://model-runner.docker.internal/engines/v1, which
// compose.yaml sets.
const DefaultURL = "http://localhost:12434/engines/v1"

// Client talks to one model.
type Client struct {
	URL   string
	Model string
	HTTP  *http.Client
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
		// A large model loads for a while on its first question.
		hc = &http.Client{Timeout: 10 * time.Minute}
	}
	res, err := hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
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
