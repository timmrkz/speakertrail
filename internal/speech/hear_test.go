package speech

import (
	"context"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Ported from FrameFairy's engine/hear_test.go, words_test.go and
// speechhost_test.go.

// dictated is a recording of a hum over room noise, with the hum stopping
// for each of the quiet stretches given, in seconds.
func dictated(seconds float64, quiet ...[2]float64) []float32 {
	samples := make([]float32, int(seconds*SampleRate))
	for i := range samples {
		at := float64(i) / SampleRate
		// Room noise, about 50 dB below the voice.
		samples[i] = float32(0.001 * math.Sin(2*math.Pi*3001*at))
		still := false
		for _, q := range quiet {
			still = still || (at >= q[0] && at < q[1])
		}
		if !still {
			samples[i] += float32(0.3 * math.Sin(2*math.Pi*220*at))
		}
	}
	return samples
}

// While somebody dictates, what they said before a pause is written down
// for good and what follows is still being heard. The pause has to be a
// real one, long enough that a sentence has ended, and has to come after
// something was said. Without one, a piece is settled before it grows
// past what the speech model takes in one go.
func TestSettledAtAPause(t *testing.T) {
	near := func(t *testing.T, got int, want float64) {
		t.Helper()
		if math.Abs(float64(got)/SampleRate-want) > 0.15 {
			t.Errorf("settled at %.2f s, want about %.2f s", float64(got)/SampleRate, want)
		}
	}
	// A pause from 3 s to 4 s settles in its middle.
	near(t, Settled(dictated(6, [2]float64{3, 4})), 3.5)
	// The last of two pauses.
	near(t, Settled(dictated(9, [2]float64{2, 3}, [2]float64{5, 6})), 5.5)
	// A pause still going on at the end settles a little way into it.
	near(t, Settled(dictated(4, [2]float64{3, 4})), 3.3)
	// A breath between two words is no pause.
	if at := Settled(dictated(6, [2]float64{3, 3.3})); at != 0 {
		t.Errorf("a breath settled at %.2f s", float64(at)/SampleRate)
	}
	// Quiet before anything was said settles nothing, and neither does
	// room noise alone.
	if at := Settled(dictated(6, [2]float64{0, 2})); at != 0 {
		t.Errorf("quiet before the first word settled at %.2f s", float64(at)/SampleRate)
	}
	if at := Settled(dictated(6, [2]float64{0, 6})); at != 0 {
		t.Errorf("room noise settled at %.2f s", float64(at)/SampleRate)
	}
	if at := Settled(nil); at != 0 {
		t.Errorf("nothing settled at %d", at)
	}
	// Talking on without a pause is settled at the quietest moment before
	// the piece is longer than the speech model takes.
	long := dictated(dictateLongest+1, [2]float64{dictateLongest - 3, dictateLongest - 2.8})
	near(t, Settled(long), dictateLongest-2.9)
	if at := Settled(dictated(dictateLongest - 1)); at != 0 {
		t.Errorf("%.0f s without a pause settled at %.2f s", dictateLongest-1, float64(at)/SampleRate)
	}
}

func TestTokensToWords(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
	tokens := []Token{
		{" Al", 0.00, 0.24}, {"les", 0.24, 0.16}, {" hat", 0.40, 0.24},
		{" En", 0.80, 0.16}, {"de", 0.96, 0.16}, {",", 1.12, 0.16},
		{" nur", 1.28, 0.16}, {" .", 1.44, 0.08},
	}
	words := TokensToWords(tokens, 10)
	want := []Word{{Start: 10.00, End: 10.40, Text: "Alles"}, {Start: 10.40, End: 10.64, Text: "hat"},
		{Start: 10.80, End: 11.12, Text: "Ende,"}, {Start: 11.28, End: 11.44, Text: "nur."}}
	if len(words) != len(want) {
		t.Fatalf("got %v", words)
	}
	for i := range want {
		if words[i].Text != want[i].Text || !near(words[i].Start, want[i].Start) || !near(words[i].End, want[i].End) {
			t.Errorf("word %d = %+v, want %+v", i, words[i], want[i])
		}
	}
	// A number comes as a lone space and then its digits, and the space
	// is what keeps it apart from the word before it.
	numbers := []Token{{" dass", 0, 0.2}, {" ", 0.2, 0.08}, {"5", 0.28, 0.08},
		{"0", 0.36, 0.08}, {"0", 0.44, 0.08}, {" Euro", 0.52, 0.2}, {".", 0.72, 0.04},
		{" ", 0.8, 0.08}, {"1", 0.88, 0.08}, {"4", 0.96, 0.08}}
	got := TokensToWords(numbers, 0)
	texts := make([]string, len(got))
	for i, w := range got {
		texts[i] = w.Text
	}
	if s := strings.Join(texts, " "); s != "dass 500 Euro. 14" {
		t.Errorf("got %q", s)
	}
	if !near(got[1].Start, 0.28) || !near(got[1].End, 0.52) {
		t.Errorf("500 is timed %+v", got[1])
	}
	// A chunk that starts without a leading space still starts a word.
	if got := TokensToWords([]Token{{"Hallo", 0, 0.2}}, 0); len(got) != 1 || got[0].Text != "Hallo" {
		t.Errorf("got %v", got)
	}
}

// countedSpeech is a speech model that counts how often it is loaded and
// closed.
type countedSpeech struct {
	loads, closes atomic.Int32
}

func (c *countedSpeech) open(string) (Recognizer, error) {
	c.loads.Add(1)
	return &countedRecognizer{c: c}, nil
}

type countedRecognizer struct {
	c      *countedSpeech
	closed atomic.Bool
}

func (r *countedRecognizer) Recognize([]float32, int) []Token { return nil }

func (r *countedRecognizer) Close() {
	if r.closed.Swap(true) {
		panic("a speech model was closed twice")
	}
	r.c.closes.Add(1)
}

// hearOnce takes the speech model, as a dictation does, and lets it go.
func hearOnce(t *testing.T, h *Host, dir string) {
	t.Helper()
	rec, release, err := h.Speech(context.Background(), dir)
	if err != nil {
		t.Error(err)
		return
	}
	rec.Recognize(nil, SampleRate)
	release()
}

// The speech model kept is loaded once, and every dictation after uses
// it. Not kept, it goes when the dictation ends.
func TestTheSpeechModelKeptIsLoadedOnce(t *testing.T) {
	var c countedSpeech
	h := &Host{Open: c.open}
	hearOnce(t, h, "speech")
	hearOnce(t, h, "speech")
	if got := c.loads.Load(); got != 2 {
		t.Fatalf("not kept, two dictations loaded the model %d times, want 2", got)
	}
	h.Keep("speech")
	for range 3 {
		hearOnce(t, h, "speech")
	}
	if got := c.loads.Load(); got != 3 {
		t.Errorf("kept, three more dictations loaded it %d times in all, want 3", got)
	}
	if got := c.closes.Load(); got != 2 {
		t.Errorf("closed %d times while kept, want 2, the two not kept", got)
	}
	h.Close()
	if got := c.closes.Load(); got != 3 {
		t.Errorf("closed %d times once the app quit, want 3", got)
	}
	h.Keep("speech")
	hearOnce(t, h, "speech")
	if loads, closes := c.loads.Load(), c.closes.Load(); loads != closes {
		t.Errorf("a model was kept after the app quit: %d loads, %d closes", loads, closes)
	}
}

// A model given up while a dictation holds it stays until that dictation
// is done.
func TestTheSpeechModelGivenUpWaitsForTheDictation(t *testing.T) {
	var c countedSpeech
	h := &Host{Open: c.open}
	h.Keep("one")
	rec, release, err := h.Speech(context.Background(), "one")
	if err != nil {
		t.Fatal(err)
	}
	if !h.Held() {
		t.Error("the model is not held while a dictation has it")
	}
	h.KeepNone()
	if c.closes.Load() != 0 {
		t.Fatal("the model was closed under a dictation")
	}
	rec.Recognize(nil, SampleRate)
	release()
	release()
	if got := c.closes.Load(); got != 1 {
		t.Fatalf("closed %d times after the dictation, want 1", got)
	}
	if h.Held() {
		t.Error("held after it was let go of")
	}
}

// Dictations, keeps and give-ups from several goroutines at once leave
// every model loaded closed once nobody holds one and none is kept, and
// none closed under a dictation, which the stand-in panics on.
func TestTheSpeechModelIsSharedAtOnce(t *testing.T) {
	var c countedSpeech
	h := &Host{Open: c.open}
	var wg sync.WaitGroup
	for i := range 40 {
		wg.Go(func() {
			switch i % 5 {
			case 0:
				h.Keep("speech")
			case 1:
				h.KeepNone()
			case 2:
				hearOnce(t, h, "other")
			default:
				hearOnce(t, h, "speech")
			}
		})
	}
	wg.Wait()
	h.Close()
	if loads, closes := c.loads.Load(), c.closes.Load(); loads != closes {
		t.Errorf("%d loads and %d closes once nobody dictates and the app quit", loads, closes)
	}
}

// One speech model is in memory at a time, never two. A dictation of
// another model waits for the one in use to be let go of.
func TestOneSpeechModelIsLoadedAtATime(t *testing.T) {
	var c countedSpeech
	h := &Host{Open: c.open}
	_, release, err := h.Speech(context.Background(), "old")
	if err != nil {
		t.Fatal(err)
	}
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := h.Speech(gone, "new"); err == nil {
		t.Error("a dictation of another model did not wait for the one in use")
	}
	if got := c.loads.Load(); got != 1 {
		t.Fatalf("%d models loaded while one is in use, want 1", got)
	}
	release()
	hearOnce(t, h, "new")
	if loads, closes := c.loads.Load(), c.closes.Load(); loads != 2 || closes != 2 {
		t.Errorf("%d loads and %d closes, want 2 and 2", loads, closes)
	}
}
