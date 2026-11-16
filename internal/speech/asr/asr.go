// Package asr runs NVIDIA's Parakeet speech recogniser on the Mac,
// through sherpa-onnx, for Dictate in Report a Problem. It is the only
// package that needs the speech library, which keeps the rest of the app
// plain Go. Ported from FrameFairy's asr/asr.go.
package asr

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/v2/sherpa_onnx"

	"github.com/timmrkz/speakertrail/internal/speech"
)

// ErrNoModel is a speech model folder that lacks a file the model needs.
var ErrNoModel = errors.New("no speech model")

// Files the model folder has to contain.
var Files = []string{"encoder.int8.onnx", "decoder.int8.onnx", "joiner.int8.onnx", "tokens.txt"}

// Check reports what is missing from a model folder, if anything.
func Check(dir string) error {
	var missing []string
	for _, name := range Files {
		if info, err := os.Stat(filepath.Join(dir, name)); err != nil || info.IsDir() {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: the speech model folder %s is missing %v", ErrNoModel, dir, missing)
	}
	return nil
}

// Recognizer wraps one loaded model.
type Recognizer struct {
	impl *sherpa.OfflineRecognizer
}

// OpenOne loads one copy of the model in dir, with as many threads as
// FrameFairy gives each of its copies on this Mac, see Mix. Dictation is
// a few sentences at a time.
func OpenOne(dir string) (speech.Recognizer, error) {
	_, threads := Mix(ThisMachine())
	r, err := OpenWith(dir, threads)
	if err != nil {
		// A nil *Recognizer in the interface would not read as nil.
		return nil, err
	}
	return r, nil
}

// Machine is what the threads are sized from.
type Machine struct {
	// Cores is every core, and Fast the performance cores where the system
	// tells them apart, zero where it does not.
	Cores, Fast int
	// Memory in bytes, zero when the system does not say.
	Memory int64
}

// ThisMachine is the machine the app is running on.
func ThisMachine() Machine {
	return Machine{Cores: runtime.NumCPU(), Fast: fastCores(), Memory: memory()}
}

// copyMemory is the memory one copy of the model is allowed, a copy for
// every 8 GB.
const copyMemory = 8 << 30

// maxCopies is the most copies measured to help.
const maxCopies = 4

// Mix is how many copies of the model hear side by side on a machine, and
// with how many threads each, as FrameFairy measured it with its speech
// benchmark: the performance cores in all, several copies rather than one,
// one thread a copy under 6 cores, and a copy for every 8 GB, at most four.
// Speaker Trail loads one copy, with the threads of one.
func Mix(m Machine) (copies, threads int) {
	total := m.Fast
	if total <= 0 {
		total = m.Cores
	}
	total = max(total, 1)
	threads = 1
	if total >= 6 {
		threads = 2
	}
	copies = min(max(total/threads, 1), maxCopies)
	if m.Memory > 0 {
		// To the nearest 8 GB: a machine of 16 GB tells Linux a little
		// less than that, and it is still a machine of 16 GB.
		copies = min(copies, max(int((m.Memory+copyMemory/2)/copyMemory), 1))
	}
	threads = max(threads, total/copies)
	return copies, threads
}

// OpenWith loads the model in dir with that many threads, on the
// processor.
func OpenWith(dir string, threads int) (*Recognizer, error) {
	if err := Check(dir); err != nil {
		return nil, err
	}
	if threads <= 0 {
		threads = min(runtime.NumCPU(), 8)
	}
	config := sherpa.OfflineRecognizerConfig{}
	config.FeatConfig.SampleRate = speech.SampleRate
	config.FeatConfig.FeatureDim = 80
	config.ModelConfig.Transducer.Encoder = filepath.Join(dir, "encoder.int8.onnx")
	config.ModelConfig.Transducer.Decoder = filepath.Join(dir, "decoder.int8.onnx")
	config.ModelConfig.Transducer.Joiner = filepath.Join(dir, "joiner.int8.onnx")
	config.ModelConfig.Tokens = filepath.Join(dir, "tokens.txt")
	config.ModelConfig.ModelType = "nemo_transducer"
	config.ModelConfig.NumThreads = threads
	config.ModelConfig.Provider = "cpu"
	config.DecodingMethod = "greedy_search"
	impl := sherpa.NewOfflineRecognizer(&config)
	if impl == nil {
		return nil, fmt.Errorf("the speech model in %s could not be loaded", dir)
	}
	return &Recognizer{impl: impl}, nil
}

// Recognize transcribes one piece of 16 kHz mono audio. Times are seconds
// from the start of the samples.
func (r *Recognizer) Recognize(samples []float32, sampleRate int) []speech.Token {
	if len(samples) == 0 {
		return nil
	}
	stream := sherpa.NewOfflineStream(r.impl)
	defer sherpa.DeleteOfflineStream(stream)
	stream.AcceptWaveform(sampleRate, samples)
	r.impl.Decode(stream)
	result := stream.GetResult()
	tokens := make([]speech.Token, 0, len(result.Tokens))
	for i, text := range result.Tokens {
		token := speech.Token{Text: text}
		if i < len(result.Timestamps) {
			token.Start = float64(result.Timestamps[i])
		}
		if i < len(result.Durations) {
			token.Duration = float64(result.Durations[i])
		}
		tokens = append(tokens, token)
	}
	return tokens
}

// Close frees the model.
func (r *Recognizer) Close() {
	if r.impl != nil {
		sherpa.DeleteOfflineRecognizer(r.impl)
		r.impl = nil
	}
}
