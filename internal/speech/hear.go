package speech

import (
	"math"
	"strings"
	"unicode"
)

// Hearing what is said, the part of FrameFairy's engine/audio.go that
// dictation needs: the recogniser, its tokens made into words, and where
// a recording still being made can be written down for good.

// Token is one piece of text the recogniser heard, with when it started
// and how long it took, in seconds from the start of what it heard.
type Token struct {
	Text     string
	Start    float64
	Duration float64
}

// Recognizer turns audio into timed tokens. Dictation needs only this
// much, which keeps the native speech library, see speech/asr, out of
// everything else.
type Recognizer interface {
	Recognize(samples []float32, sampleRate int) []Token
	Close()
}

// SampleRate is what the recogniser is fed, 16 kHz mono.
const SampleRate = 16000

// frameSeconds is the resolution of the loudness measurement.
const frameSeconds = 0.01

const frameSamples = int(SampleRate * frameSeconds)

// Word is a word heard, with when it starts and ends.
type Word struct {
	Start, End float64
	Text       string
}

// TokensToWords joins the recogniser's tokens into words. A token that
// does not start with a space belongs to the word before it.
//
// A token that is only a space starts the word after it. The speech model
// has no digit that starts a word, so a number comes as a lone space and
// then its digits, and without the space "dass 5000" is "dass5000".
func TokensToWords(tokens []Token, offset float64) []Word {
	var words []Word
	spaced := false
	for _, token := range tokens {
		text := token.Text
		start := offset + token.Start
		end := start + max(token.Duration, 0)
		trimmed := strings.TrimFunc(text, unicode.IsSpace)
		if trimmed == "" {
			spaced = spaced || text != ""
			continue
		}
		startsWord := spaced || strings.HasPrefix(text, " ") || len(words) == 0
		spaced = false
		if !startsWord || (isPunctuation(trimmed) && len(words) > 0) {
			last := &words[len(words)-1]
			last.Text += trimmed
			if !isPunctuation(trimmed) {
				last.End = max(last.End, end)
			}
			continue
		}
		words = append(words, Word{Start: start, End: end, Text: trimmed})
	}
	return words
}

func isPunctuation(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune(".,!?;:…-–—\"'„“”»«)", r) {
			return false
		}
	}
	return true
}

// frameDB is how loud a frame is, in dB.
func frameDB(samples []float32) float32 {
	sum := 0.0
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	return float32(10 * math.Log10(sum/float64(len(samples))+1e-20))
}

// quietestBetween is the middle of the quietest 300 ms between two
// lengths, in seconds, in frames.
func quietestBetween(frames []float32, shortest, longest float64) int {
	const window = 30
	low := int(shortest / frameSeconds)
	high := min(int(longest/frameSeconds), len(frames)) - window
	if high <= low {
		return min(int(longest/frameSeconds), len(frames))
	}
	best, bestAt := math.Inf(1), low
	running := 0.0
	for i := low; i < low+window; i++ {
		running += float64(frames[i])
	}
	for i := low; i <= high; i++ {
		if running < best {
			best, bestAt = running, i
		}
		if i+window < len(frames) {
			running += float64(frames[i+window]) - float64(frames[i])
		}
	}
	return bestAt + window/2
}

// settlePause is the shortest quiet, in frames, that ends a thought while
// somebody dictates: 0.6 s, longer than a breath between two words, so a
// sentence is not cut in two and given a full stop it never had.
const settlePause = 60

// SettleSamples is settlePause in samples.
const SettleSamples = settlePause * frameSamples

// dictateLongest is how long, in seconds, a piece being dictated may grow
// without a pause before it is settled anyway, at its quietest moment in
// its second half. Until then it is written down again on every pass, and
// every pass may write it a little differently, so it is kept short: at
// 20 s Tim watched words change well behind what he was saying.
const dictateLongest = 12.0

// Settled is where the words of a recording still being made can be
// written down for good, in samples: the middle of the last pause of at
// least settlePause after something was said, or a little way into one
// still going on at the end. Without a pause it is 0, until the recording
// is longer than dictateLongest and is cut at its quietest moment. What
// counts as quiet is measured from the recording itself, a third of the
// way up from its room noise to its voice, since every microphone and
// every room is another level.
func Settled(samples []float32) int {
	frames := make([]float32, len(samples)/frameSamples)
	if len(frames) == 0 {
		return 0
	}
	loud, noise := float32(math.Inf(-1)), float32(math.Inf(1))
	for i := range frames {
		frames[i] = frameDB(samples[i*frameSamples : (i+1)*frameSamples])
		loud = max(loud, frames[i])
		// A microphone starts with a few frames of nothing at all, which
		// is no room's noise.
		if frames[i] > -100 {
			noise = min(noise, frames[i])
		}
	}
	at := 0
	// Twelve decibels between the loudest and the quietest is less than
	// any voice over any room, so nothing was said.
	if loud-noise >= 12 {
		quiet := noise + (loud-noise)/3
		voiced := noise + 2*(loud-noise)/3
		run, spoke := 0, false
		for i, db := range frames {
			if db < quiet {
				run++
				continue
			}
			if spoke && run >= settlePause {
				at = (i - run + run/2) * frameSamples
			}
			run = 0
			spoke = spoke || db >= voiced
		}
		if spoke && run >= settlePause {
			at = (len(frames) - run + settlePause/2) * frameSamples
		}
	}
	if at == 0 && float64(len(samples))/SampleRate > dictateLongest {
		at = quietestBetween(frames, dictateLongest/2, dictateLongest) * frameSamples
	}
	return at
}
