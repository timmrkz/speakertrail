package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/timmrkz/speakertrail/internal/extract"
)

// fakeModel answers like Docker Model Runner. answer gets the user message
// and returns the people to put in the reply.
func fakeModel(t *testing.T, answer func(user string) []Person) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/engines/v1/chat/completions" || r.Method != http.MethodPost {
			http.Error(w, "wrong path "+r.URL.Path, http.StatusNotFound)
			return
		}
		var req struct {
			Model          string    `json:"model"`
			Temperature    float64   `json:"temperature"`
			Messages       []message `json:"messages"`
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.Model != "ai/test-model" || req.ResponseFormat.Type != "json_schema" || req.Temperature != 0 || len(req.Messages) != 2 {
			t.Errorf("request %+v", req)
		}
		content, _ := json.Marshal(map[string]any{"people": answer(req.Messages[1].Content)})
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": string(content)}}},
			"usage":   map[string]any{"prompt_tokens": 100, "completion_tokens": 20},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func client(srv *httptest.Server) *Client {
	return &Client{URL: srv.URL + "/engines/v1", Model: "ai/test-model"}
}

// All names here are invented.
const page = `Founders Breakfast #42
Durch den Morgen führt Karla Kontrolle.
Startplatz, Köln, 7. Oktober 2026
Diesmal erzählt Dr. Lena Musterfrau, Gründerin der Beispiel GmbH, wie aus ihrer Backstube eine Kette wurde.
Moderation: Jonas Beispielmann
Fragen an info@example.org`

func TestPeopleKeepsOnlyNamesThePageContains(t *testing.T) {
	srv, _ := fakeModel(t, func(string) []Person {
		return []Person{
			{Name: "Lena Musterfrau", Role: "speaker", Affiliation: "Beispiel GmbH"},
			{Name: "Jonas Beispielmann", Role: "moderator", Affiliation: "Musterbank AG"},
			{Name: "Erika Erfunden", Role: "speaker", Affiliation: ""},
			{Name: "Lena", Role: "speaker"},
			{Name: "info@example.org", Role: "host"},
			{Name: "lena  musterfrau", Role: "speaker"},
			{Name: "Karla Kontrolle", Role: "host", Affiliation: "Karla Kontrolle"},
		}
	})
	res, err := client(srv).People(t.Context(), "Founders Breakfast", page)
	if err != nil {
		t.Fatal(err)
	}
	want := []Person{
		{Name: "Lena Musterfrau", Role: "speaker", Affiliation: "Beispiel GmbH"},
		// The page does not name his organisation, so it is left out.
		{Name: "Jonas Beispielmann", Role: "moderator", Affiliation: ""},
		// Her own name is not her organisation.
		{Name: "Karla Kontrolle", Role: "host", Affiliation: ""},
	}
	if len(res.People) != len(want) {
		t.Fatalf("people %+v", res.People)
	}
	for i := range want {
		if res.People[i] != want[i] {
			t.Errorf("person %d: %+v, want %+v", i, res.People[i], want[i])
		}
	}
	var dropped []string
	for _, p := range res.Dropped {
		dropped = append(dropped, p.Name)
	}
	if strings.Join(dropped, "|") != "Erika Erfunden|Lena|info@example.org" {
		t.Errorf("dropped %q", dropped)
	}
	if res.Parts != 1 || res.Tokens != 120 || res.Unread != 0 {
		t.Errorf("parts %d, tokens %d, unread %d", res.Parts, res.Tokens, res.Unread)
	}
}

func TestFullName(t *testing.T) {
	// All names are invented.
	for name, want := range map[string]bool{
		"Lena Musterfrau":        true,
		"Dr. Lena Musterfrau":    true,
		"Anna-Lena von Beispiel": true,
		"Kofi AB Probemann":      true,
		"Jürgen Weß":             true,
		"Lena":                   false,
		"Dr. Lena":               false,
		"Lena M.":                false,
		"Lena M":                 false,
		"Beispiel digital":       false,
		"Musterfirma GmbH":       false,
		"Foodclub NRW e.V.":      false,
		"Gare du Beispiel GmbH":  false,
		"Lena 2":                 false,
		"lena@example.org":       false,
	} {
		if got := fullName(name, extract.NormaliseName(name)); got != want {
			t.Errorf("fullName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestPeopleReadsALongPageInParts(t *testing.T) {
	var b strings.Builder
	for i := range 3000 {
		b.WriteString("Zeile mit Programm und Beschreibung ")
		if i == 10 {
			b.WriteString("Keynote: Mara Beispielfrau")
		}
		if i == 300 {
			b.WriteString("Panel mit Mara Beispielfrau und Tom Testmann")
		}
		b.WriteString("\n")
	}
	text := b.String()
	srv, calls := fakeModel(t, func(user string) []Person {
		var out []Person
		for _, n := range []string{"Mara Beispielfrau", "Tom Testmann"} {
			if strings.Contains(user, n) {
				out = append(out, Person{Name: n, Role: "speaker"})
			}
		}
		return out
	})
	res, err := client(srv).People(t.Context(), "Lange Seite", text)
	if err != nil {
		t.Fatal(err)
	}
	if int(calls.Load()) != maxParts || res.Parts != maxParts {
		t.Errorf("%d calls, %d parts, want %d", calls.Load(), res.Parts, maxParts)
	}
	if res.Unread == 0 {
		t.Error("the end of the page was not read, and the result must say so")
	}
	if len(res.People) != 2 {
		t.Errorf("people %+v, want each name once", res.People)
	}
}

func TestSplitNeverCutsAFullPartMidCharacter(t *testing.T) {
	line := strings.Repeat("ä", 5000)
	parts, unread := split(line, 1001, 10)
	total := 0
	for _, p := range parts {
		if !strings.HasPrefix(p, "ä") && p != "\n" {
			t.Fatalf("a part starts in the middle of a character: %q", p[:4])
		}
		total += len(p)
	}
	if unread != 0 || total < len(line) {
		t.Errorf("read %d of %d bytes, %d unread", total, len(line), unread)
	}
}

func TestPeopleSaysWhenNoModelAnswers(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, err := (&Client{URL: url, Model: "ai/test-model"}).People(t.Context(), "", page)
	if !errors.Is(err, ErrUnreachable) {
		t.Errorf("error %v, want ErrUnreachable", err)
	}
}

func TestPeopleSaysWhenTheModelIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := client(srv).People(t.Context(), "", page)
	if err == nil || !strings.Contains(err.Error(), "docker model pull ai/test-model") {
		t.Errorf("error %v, want the pull command", err)
	}
}

// The client holds no state between questions, so pages can be read at the
// same time.
func TestPeopleFromSeveralGoroutines(t *testing.T) {
	srv, calls := fakeModel(t, func(string) []Person {
		return []Person{{Name: "Lena Musterfrau", Role: "speaker"}}
	})
	c := client(srv)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			res, err := c.People(context.Background(), "", page)
			if err != nil || len(res.People) != 1 {
				t.Errorf("%+v %v", res, err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 8 {
		t.Errorf("%d calls", calls.Load())
	}
}
