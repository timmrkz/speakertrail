// Package speech is the speech model Speaker Trail hears with on the Mac:
// which ones can be installed, fetching one, and taking it off again.
// Report a Problem's Dictate hears with it, see cmd/speakertrail-app/listen.go.
// Ported from FrameFairy's engine/speech.go and engine/remove_model.go.
//
// No model ships with the app. The speech model is 490 MB, which is not
// an update, and the app fetches it from whoever published it when Tim
// presses Download in Settings. Nothing about it is ours to redistribute.
//
// There is one model today and the list is written for more, the way
// FrameFairy's is: the moment there are two, which one becomes a choice.
package speech

import (
	"os"
	"path/filepath"
)

// Model is one recogniser that can be installed.
type Model struct {
	// Name is the folder it unpacks to, under the models folder, and the
	// name the app knows it by.
	Name string `json:"name"`
	// Title is what it is called in the app.
	Title string `json:"title"`
	// About is one line saying what it is for.
	About string `json:"about"`
	// Maker is who made it, the way a list of models says it.
	Maker string `json:"maker"`
	// Languages it covers, in words.
	Languages string `json:"languages"`
	// Download is the size of the archive in bytes, Unpacked what it costs
	// on disk once it is there. Both are said before anything starts,
	// because a download nobody agreed to is a download nobody wanted.
	Download int64 `json:"download"`
	Unpacked int64 `json:"unpacked"`
	// URL is where it comes from, whoever published it rather than us.
	URL string `json:"url"`
	// SHA256 of the archive. What arrives over a network is untrusted, and
	// a model that is not what it claims to be is not unpacked.
	SHA256 string `json:"-"`
	// Recommended marks the one the app offers when nobody has chosen.
	Recommended bool `json:"recommended"`
}

// Models is every model that can be installed, best first. The same as
// FrameFairy's, so a model fetched for one is the model of the other.
func Models() []Model {
	return []Model{{
		Name:        "sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8",
		Title:       "Parakeet TDT 0.6B v3",
		About:       "NVIDIA's recogniser, quantised. Fast on any machine and accurate on speech.",
		Maker:       "NVIDIA",
		Languages:   "25 European languages, German and English among them",
		Download:    487_170_055,
		Unpacked:    671_239_000,
		URL:         "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8.tar.bz2",
		SHA256:      "5793d0fd397c5778d2cf2126994d58e9d56b1be7c04d13c7a15bb1b4eafb16bf",
		Recommended: true,
	}}
}

// ByName finds one by the name it unpacks to.
func ByName(name string) (Model, bool) {
	for _, m := range Models() {
		if m.Name == name {
			return m, true
		}
	}
	return Model{}, false
}

// Dir is where the models live, in the app's folder: ~/Library/Application
// Support/Speaker Trail/models on a Mac. FrameFairy keeps its own in
// ~/.framefairy/models, and the two apps do not reach into each other's
// folders.
func Dir(appFolder string) string {
	return filepath.Join(appFolder, "models")
}

// Installed says whether this model is in dir and usable. A folder alone
// is not enough: a download interrupted half way leaves one, and Dictate
// would then fail on the first second of audio.
func (m Model) Installed(dir string) bool {
	return Ready(filepath.Join(dir, m.Name))
}

// Ready says whether the speech model in dir is there and usable: its
// tokens.txt, the file the recogniser opens first, is there and not empty.
func Ready(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "tokens.txt"))
	return err == nil && !info.IsDir() && info.Size() > 0
}

// InUse is the model Dictate hears with: the first installed one, best
// first, and false when none is.
func InUse(dir string) (Model, bool) {
	for _, m := range Models() {
		if m.Installed(dir) {
			return m, true
		}
	}
	return Model{}, false
}
