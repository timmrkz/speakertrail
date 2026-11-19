// Package language is the language model that drafts a first message on
// the Mac: which models can be fetched, fetching one, checking it before
// it is loaded, running it in llama-server and asking it. Ported from
// FrameFairy's engine, language.go, modelcheck.go, modelhost.go and
// local.go, where the same models choose clips.
//
// No model ships with the app. The smallest useful one is gigabytes, which
// is not an update, so the app fetches it from whoever published it when
// Tim presses Download in Settings. Nothing about it is ours to
// redistribute, so each licence is between Tim and its maker.
//
// A model runs from memory, so what decides whether a Mac can have one is
// not the download but what it takes to run. The list is judged by what
// the Mac holds, see FitsIn and Recommended.
package language

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Model is one model that can draft a first message.
type Model struct {
	// Name is the file it lands as, in the models folder.
	Name string `json:"name"`
	// Title is what it is called in the app, Maker who made it.
	Title string `json:"title"`
	Maker string `json:"maker"`
	// About is one line saying what it is.
	About string `json:"about"`
	// Download is how big the file is, in bytes, read off the real file
	// rather than guessed. It is said before anything starts, because a
	// download nobody agreed to is a download nobody wanted.
	Download int64 `json:"download"`
	// Needs is the memory it takes to write a first message, in bytes:
	// the weights, the cache the context lives in and llama.cpp's own
	// working memory. Worked out by NeedsAt at Context, never written by
	// hand.
	Needs int64 `json:"needs"`
	// Shape is what the model keeps in memory for every token of context.
	Shape Shape `json:"-"`
	// URL is where it comes from, whoever published it rather than us.
	URL string `json:"url"`
	// SHA256 of the file. What arrives over a network is untrusted, and a
	// model that is not what it claims to be is not kept.
	SHA256 string `json:"-"`
}

// Shape is what a model keeps in memory per token of context, read off
// the model's own config.json on Hugging Face rather than remembered.
type Shape struct {
	// Layers that look back over the whole context, and how many key and
	// value heads each has, how wide.
	FullLayers, FullKVHeads, FullHeadDim int
	// Layers that look back over only the last Window tokens.
	WindowLayers, WindowKVHeads, WindowHeadDim, Window int
}

// Context is the context llama-server is started with: what the prompt
// template, a whole profile of 8000 characters and the message take
// together, with room to spare. FrameFairy judges its models at 65536
// tokens, because a search reads a transcript. A first message reads one
// profile.
const Context = 8192

// windowSpare is how many cells llama.cpp keeps beyond the window in a
// sliding window cache: one batch, 512 by default.
const windowSpare = 512

// workingAllowance is llama.cpp's own working memory on top of the
// weights and the cache, a whole gigabyte as FrameFairy allows it.
const workingAllowance = 1 << 30

// checkpoints is how many copies of the window llama-server keeps, see
// FrameFairy's language.go. The server is told to keep no more.
const checkpoints = 3

func perToken(layers, heads, dim int) int64 { return int64(2 * layers * heads * dim * 2) }

// cacheBytes is the key and value cache for a context of ctx tokens, at
// two bytes a number, llama.cpp's default.
func (s Shape) cacheBytes(ctx int) int64 {
	cells := min(ctx, s.Window+windowSpare)
	return perToken(s.FullLayers, s.FullKVHeads, s.FullHeadDim)*int64(ctx) +
		perToken(s.WindowLayers, s.WindowKVHeads, s.WindowHeadDim)*int64(cells)
}

// checkpointBytes is what the checkpoints of the window take.
func (s Shape) checkpointBytes() int64 {
	return checkpoints * perToken(s.WindowLayers, s.WindowKVHeads, s.WindowHeadDim) * int64(s.Window)
}

// NeedsAt is the memory the model takes with a context of ctx tokens.
func (m Model) NeedsAt(ctx int) int64 {
	return m.Download + m.Shape.cacheBytes(ctx) + m.Shape.checkpointBytes() + workingAllowance
}

// Models is every model that can be installed, largest first. The same
// four as FrameFairy's, from the same makers, with the same files and
// checksums, so a model is what its maker released.
func Models() []Model {
	const hf = "https://huggingface.co/"
	models := []Model{{
		Name:  "gemma-4-26B_q4_0-it.gguf",
		Title: "Gemma 4 26B A4B",
		Maker: "Google",
		About: "Twenty six billion parameters with four of them used per token, " +
			"so it runs like a far smaller model. The largest of these.",
		Download: 14_439_363_584,
		URL:      hf + "google/gemma-4-26B-A4B-it-qat-q4_0-gguf/resolve/main/gemma-4-26B_q4_0-it.gguf",
		SHA256:   "3eca3b8f6d7baf218a7dd6bba5fb59a56ee25fe2d567b6f5f589b4f697eca51d",
		Shape:    Shape{FullLayers: 5, FullKVHeads: 2, FullHeadDim: 512, WindowLayers: 25, WindowKVHeads: 8, WindowHeadDim: 256, Window: 1024},
	}, {
		Name:     "Qwen3-14B-Q4_K_M.gguf",
		Title:    "Qwen3 14B",
		Maker:    "Alibaba",
		About:    "Fourteen billion parameters, quantised to four bits.",
		Download: 9_001_752_960,
		URL:      hf + "Qwen/Qwen3-14B-GGUF/resolve/main/Qwen3-14B-Q4_K_M.gguf",
		SHA256:   "500a8806e85ee9c83f3ae08420295592451379b4f8cf2d0f41c15dffeb6b81f0",
		Shape:    Shape{FullLayers: 40, FullKVHeads: 8, FullHeadDim: 128},
	}, {
		Name:  "gemma-4-12b-it-qat-q4_0.gguf",
		Title: "Gemma 4 12B",
		Maker: "Google",
		About: "Twelve billion parameters, trained to be quantised to four bits " +
			"rather than cut down to them afterwards.",
		Download: 6_975_879_296,
		URL:      hf + "google/gemma-4-12B-it-qat-q4_0-gguf/resolve/main/gemma-4-12b-it-qat-q4_0.gguf",
		SHA256:   "93567e57a8fe10b23569b9d9ec38cd005deedf71e29477c421a4b83f418a538b",
		Shape:    Shape{FullLayers: 8, FullKVHeads: 1, FullHeadDim: 512, WindowLayers: 40, WindowKVHeads: 8, WindowHeadDim: 256, Window: 1024},
	}, {
		Name:  "Ministral-3-8B-Instruct-2512-Q4_K_M.gguf",
		Title: "Ministral 3 8B",
		Maker: "Mistral",
		About: "Eight billion parameters, quantised to four bits. The smallest " +
			"of these, for a machine with less to spare.",
		Download: 5_198_911_904,
		URL:      hf + "mistralai/Ministral-3-8B-Instruct-2512-GGUF/resolve/main/Ministral-3-8B-Instruct-2512-Q4_K_M.gguf",
		SHA256:   "33e7a72cf5e6e2cfc2f2847075acc013d68bba023e35310cef86b5cf8fdca761",
		Shape:    Shape{FullLayers: 34, FullKVHeads: 8, FullHeadDim: 128},
	}}
	for i := range models {
		models[i].Needs = models[i].NeedsAt(Context)
	}
	return models
}

// ByName finds one of models by the file it lands as.
func ByName(models []Model, name string) (Model, bool) {
	for _, m := range models {
		if m.Name == name {
			return m, true
		}
	}
	return Model{}, false
}

// Dir is where the models live, in the app's folder: ~/Library/Application
// Support/Speaker Trail/models on a Mac, beside the speech model.
// FrameFairy keeps its own in ~/.framefairy/models, and the two apps do
// not reach into each other's folders.
func Dir(appFolder string) string {
	return filepath.Join(appFolder, "models")
}

// Installed says whether this model is in dir and is a model. A file of
// the right name is not enough: it has to begin the way a model does. A
// download that stopped never gets this far, because it lands under a
// part name and is only moved once it is whole.
func (m Model) Installed(dir string) bool {
	path := filepath.Join(dir, m.Name)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return isGGUF(path)
}

// isGGUF says whether a file begins the way a model does. Four bytes, and
// they are the only thing that tells a model from an error page saved
// under a model's name.
func isGGUF(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	magic := make([]byte, 4)
	if _, err := file.Read(magic); err != nil {
		return false
	}
	return string(magic) == "GGUF"
}

// InUse is the model that drafts: the largest of models installed in dir,
// and false when none is. The one place that says which, for drafting and
// for Settings alike.
func InUse(models []Model, dir string) (Model, bool) {
	for _, m := range models {
		if m.Installed(dir) {
			return m, true
		}
	}
	return Model{}, false
}

// Fit is what a machine can do with a model.
type Fit string

const (
	// FitsWell means it runs with room to spare.
	FitsWell Fit = "fits"
	// FitsTight means it runs and the machine has little left.
	FitsTight Fit = "tight"
	// TooBig means it does not fit in memory at all. Running it anyway
	// means swapping, which is minutes an answer rather than seconds.
	TooBig Fit = "too big"
	// FitUnknown means the machine did not say how much memory it has.
	FitUnknown Fit = "unknown"
)

// memoryHeadroom is how much a machine has to have left over for a model
// to be comfortable: the system, the app, a browser and whatever else is
// open.
const memoryHeadroom = 8 << 30

// FitsIn says what a machine with this much memory can do with the model.
func (m Model) FitsIn(total int64) Fit {
	switch {
	case total <= 0:
		return FitUnknown
	case m.Needs+memoryHeadroom <= total:
		return FitsWell
	case m.Needs <= total:
		return FitsTight
	default:
		return TooBig
	}
}

// Recommended is the model to offer a machine with this much memory: the
// largest it holds comfortably, or else the largest it holds at all. A
// machine that will not say is offered the smallest, the one most likely
// to run. It answers false when nothing fits.
func Recommended(models []Model, total int64) (Model, bool) {
	if len(models) == 0 {
		return Model{}, false
	}
	if total <= 0 {
		smallest := models[0]
		for _, m := range models {
			if m.Download < smallest.Download {
				smallest = m
			}
		}
		return smallest, true
	}
	for _, want := range []Fit{FitsWell, FitsTight} {
		var best Model
		found := false
		for _, m := range models {
			if m.FitsIn(total) == want && (!found || m.Download > best.Download) {
				best, found = m, true
			}
		}
		if found {
			return best, true
		}
	}
	return Model{}, false
}

// MachineMemory is how much memory this machine has, in bytes, or zero
// when it will not say.
func MachineMemory() int64 {
	switch runtime.GOOS {
	case "darwin":
		return systemMemory()
	case "linux":
		return meminfo("/proc/meminfo", "MemTotal:")
	}
	return 0
}

// meminfo reads one field, like MemTotal:, out of a meminfo file, which
// is in kibibytes.
func meminfo(path, field string) int64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	lines := bufio.NewScanner(file)
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		if len(fields) < 2 || fields[0] != field {
			continue
		}
		n, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return n * 1024
	}
	return 0
}
