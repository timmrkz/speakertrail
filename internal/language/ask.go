package language

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Asking the model, streamed, so the draft grows on screen as the model
// writes it. FrameFairy's askLocal and readLocalStream, for a plain answer.

// Message is one turn of a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Answer is what the model wrote and what it cost.
type Answer struct {
	Text string
	// PromptTokens is what it read, Written what it wrote.
	PromptTokens, Written int
	// Finish is why it stopped: stop, or length when it ran into
	// MaxTokens.
	Finish string
}

// MaxTokens is the most a first message may take. A first message is a
// few sentences, a few hundred tokens.
const MaxTokens = 1024

// ErrStalled is a server that stopped answering in the middle of an ask.
// The draft lets go of it at once, see Host.Hold.
var ErrStalled = errors.New("the language model stopped answering")

// stall is how long the server may send nothing before the ask is given
// up on. It speaks all the time while it works, so this is a server that
// hangs. As FrameFairy's localStall.
const stall = 2 * time.Minute

// Ask asks the server for an answer to messages. heard is told the text
// written so far, each time more arrives. The ask ends with ctx.
func Ask(ctx context.Context, client *http.Client, s *Server, messages []Message, heard func(string)) (Answer, error) {
	body, err := json.Marshal(map[string]any{
		"messages":   messages,
		"max_tokens": MaxTokens,
		// No thinking before the message. Gemma 4 thinks for minutes
		// when it is let, and a few sentences need none.
		"reasoning_budget_tokens": 0,
		"stream":                  true,
		"stream_options":          map[string]any{"include_usage": true},
	})
	if err != nil {
		return Answer{}, err
	}
	asking, stop := context.WithCancel(ctx)
	defer stop()
	var stalled atomic.Bool
	watch := time.AfterFunc(stall, func() {
		stalled.Store(true)
		stop()
	})
	defer watch.Stop()
	gaveUp := func(err error) error {
		if stalled.Load() {
			return fmt.Errorf("%w: nothing came from it for %s", ErrStalled, stall)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	req, err := http.NewRequestWithContext(asking, http.MethodPost, s.URL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Key)
	res, err := orDefault(client).Do(req)
	if err != nil {
		return Answer{}, gaveUp(fmt.Errorf("the language model did not answer: %w", err))
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return Answer{}, fmt.Errorf("the language model reported an error: %s", refusal(res.StatusCode, raw))
	}
	a, err := readStream(res.Body, func() { watch.Reset(stall) }, heard)
	if err != nil {
		return Answer{}, gaveUp(err)
	}
	if strings.TrimSpace(a.Text) == "" {
		return Answer{}, errors.New("the language model wrote nothing")
	}
	return a, nil
}

// refusal is what the server said when it answered status with raw.
func refusal(status int, raw []byte) string {
	var said struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &said) == nil && said.Error != nil {
		return clip(said.Error.Message, 400)
	}
	return fmt.Sprintf("status %d", status)
}

// clip cuts text to at most n letters.
func clip(text string, n int) string {
	r := []rune(text)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return text
}

// chunk is one piece of a streamed answer.
type chunk struct {
	Choices []struct {
		Delta struct {
			Content *string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// readStream reads the server's events, a line "data: {...}" each, until
// "data: [DONE]". came is told whenever anything arrives, heard the text
// so far whenever it grows. What the model writes is untrusted: it is
// kept as text and shown as text.
func readStream(r io.Reader, came func(), heard func(string)) (Answer, error) {
	var a Answer
	var text strings.Builder
	finished := false
	lines := bufio.NewScanner(r)
	lines.Buffer(make([]byte, 64<<10), 1<<20)
	for lines.Scan() {
		came()
		line := lines.Text()
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			finished = true
			break
		}
		var c chunk
		if err := json.Unmarshal([]byte(data), &c); err != nil {
			return Answer{}, errors.New("the language model sent a piece of its answer that is not JSON")
		}
		if c.Error != nil {
			return Answer{}, fmt.Errorf("the language model reported an error: %s", clip(c.Error.Message, 400))
		}
		grew := false
		for _, choice := range c.Choices {
			if choice.Delta.Content != nil && *choice.Delta.Content != "" {
				text.WriteString(*choice.Delta.Content)
				grew = true
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				a.Finish = *choice.FinishReason
			}
		}
		if c.Usage != nil {
			a.PromptTokens, a.Written = c.Usage.PromptTokens, c.Usage.CompletionTokens
		}
		if grew && heard != nil {
			heard(text.String())
		}
	}
	if err := lines.Err(); err != nil {
		return Answer{}, err
	}
	// The last word is the end marker or a reason to stop. Neither means
	// the connection went before the answer was done.
	if !finished && a.Finish == "" {
		return Answer{}, errors.New("the language model's answer stopped before it was finished")
	}
	a.Text = text.String()
	return a, nil
}
