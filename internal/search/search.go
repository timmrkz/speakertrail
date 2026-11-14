// Package search asks Exa's index for LinkedIn profiles. The engine never
// opens LinkedIn itself, Exa answers from what it already holds.
package search

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var defaultClient = &http.Client{Timeout: 30 * time.Second}

// Exa is the search API at exa.ai. Its free plan needs no card.
type Exa struct {
	Key string
	// URL is the endpoint. Tests set it.
	URL    string
	Client *http.Client
	// RetryWait is how long People waits before it asks again after Exa
	// failed on its side. Tests set it.
	RetryWait time.Duration
}

// call sends a request and reads a JSON answer. An answer other than 200
// is an error that quotes the start of what the provider said.
func call(c *http.Client, req *http.Request, name string, out any) error {
	if c == nil {
		c = defaultClient
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 200 {
			// Cut at 200 bytes, and drop a character cut in half.
			msg = strings.ToValidUTF8(msg[:200], "")
		}
		return fmt.Errorf("%s answered %d: %s", name, resp.StatusCode, msg)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: the answer is not the expected JSON: %w", name, err)
	}
	return nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
