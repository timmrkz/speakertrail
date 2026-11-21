package language

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/timmrkz/speakertrail/internal/fetch"
)

// The test program is the stand-in llama-server too: started with
// FAKE_LLAMA_SERVER set, it serves /health and a streamed answer on the
// port it is given, to the key in its environment, and says it is loaded
// the way llama-server does.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_LLAMA_SERVER") == "1" {
		fakeServer()
		return
	}
	os.Exit(m.Run())
}

func fakeServer() {
	fs := flag.NewFlagSet("llama-server", flag.ContinueOnError)
	port := fs.String("port", "", "")
	model := fs.String("m", "", "")
	fs.String("host", "", "")
	fs.String("c", "", "")
	fs.String("ngl", "", "")
	fs.String("np", "", "")
	fs.String("b", "", "")
	fs.String("ctx-checkpoints", "", "")
	fs.String("cache-ram", "", "")
	fs.Bool("no-slots", false, "")
	fs.Bool("no-cache-idle-slots", false, "")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if strings.HasSuffix(*model, "broken.gguf") {
		fmt.Println("llama_model_load: error loading model: this is no model")
		os.Exit(1)
	}
	key := os.Getenv("LLAMA_API_KEY")
	l, err := net.Listen("tcp", "127.0.0.1:"+*port)
	if err != nil {
		os.Exit(3)
	}
	fmt.Println("main: model loaded")
	_ = http.Serve(l, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/health":
			fmt.Fprint(w, `{"status":"ok"}`)
		case "/v1/chat/completions":
			stream(w, "Hallo", " Mara", "!")
		}
	}))
}

// stream answers the way llama-server streams an answer.
func stream(w http.ResponseWriter, pieces ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, p := range pieces {
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", p)
	}
	fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	fmt.Fprint(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":812,\"completion_tokens\":3}}\n\n")
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func TestRecommended(t *testing.T) {
	models := Models()
	for _, tc := range []struct {
		gb   int64
		want string
	}{{32, "Gemma 4 26B A4B"}, {24, "Gemma 4 26B A4B"}, {20, "Qwen3 14B"}, {16, "Ministral 3 8B"}, {4, ""}, {0, "Ministral 3 8B"}} {
		m, ok := Recommended(models, tc.gb<<30)
		if (tc.want == "") == ok || (ok && m.Title != tc.want) {
			t.Errorf("%d GB: %s %v, want %q", tc.gb, m.Title, ok, tc.want)
		}
	}
	for _, m := range models {
		if m.Needs <= m.Download || m.SHA256 == "" || !strings.HasPrefix(m.URL, "https://huggingface.co/") {
			t.Errorf("%+v", m)
		}
	}
}

// fakeModel is a file that begins the way a model does, with its sum.
func fakeModel(body string) (Model, []byte) {
	data := []byte("GGUF" + body)
	sum := sha256.Sum256(data)
	return Model{Name: "probe.gguf", Title: "Probe", Download: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}, data
}

func TestInstallCheckAndRemove(t *testing.T) {
	m, data := fakeModel(strings.Repeat("weights", 1000))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data) }))
	defer srv.Close()
	m.URL = srv.URL + "/probe.gguf"
	dir := t.TempDir()
	if err := (Installer{}).Install(t.Context(), m, dir); err != nil {
		t.Fatal(err)
	}
	if !m.Installed(dir) {
		t.Fatal("not installed")
	}
	if got, ok := InUse([]Model{{Name: "other.gguf"}, m}, dir); !ok || got.Name != m.Name {
		t.Errorf("in use %v %v", got.Name, ok)
	}
	if err := Check(m, dir); err != nil {
		t.Fatal(err)
	}
	// The file changed after it arrived is not loaded.
	if err := os.WriteFile(filepath.Join(dir, m.Name), append([]byte("GGUF"), make([]byte, len(data)-4)...), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	os.Chtimes(filepath.Join(dir, m.Name), later, later)
	if err := Check(m, dir); err == nil || !strings.Contains(err.Error(), "not the file it was") {
		t.Errorf("a changed model was let through: %v", err)
	}
	if err := Remove(m, dir); err != nil || m.Installed(dir) {
		t.Errorf("remove %v", err)
	}
}

// What is not the model, a wrong file or a page saying no, is not kept,
// and nothing of it stays to carry on from.
func TestInstallRefusesWhatIsNotTheModel(t *testing.T) {
	m, _ := fakeModel("weights")
	for _, body := range []string{"GGUFother", "<html>no</html>"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		m.URL = srv.URL
		m.Download = int64(len(body))
		dir := t.TempDir()
		if err := (Installer{}).Install(t.Context(), m, dir); err == nil {
			t.Errorf("%q was installed", body)
		}
		if _, err := os.Stat(filepath.Join(dir, m.Name+".part")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%q left its part behind", body)
		}
		srv.Close()
	}
}

// On the real disk: llama-server is another program, the test's own in
// the stand-in's part, and it reads the model's file.
func TestStartAndAsk(t *testing.T) {
	t.Setenv("FAKE_LLAMA_SERVER", "1")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(t.TempDir(), "probe.gguf")
	os.WriteFile(model, []byte("GGUF"), 0o644)
	s, err := Start(t.Context(), self, model, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	var heard []string
	a, err := Ask(t.Context(), nil, s, []Message{{Role: "user", Content: "Schreib Mara."}}, func(so string) { heard = append(heard, so) })
	if err != nil {
		t.Fatal(err)
	}
	if a.Text != "Hallo Mara!" || a.PromptTokens != 812 || a.Written != 3 || a.Finish != "stop" {
		t.Errorf("answer %+v", a)
	}
	if strings.Join(heard, "|") != "Hallo|Hallo Mara|Hallo Mara!" {
		t.Errorf("heard %q", heard)
	}
	// Without its key the server answers nobody.
	res, err := http.Get(s.URL + "/health")
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		t.Errorf("health without the key: %v %v", res, err)
	}
	s.Stop()
	if _, err := http.Get(s.URL + "/health"); err == nil {
		t.Error("the server is still there after Stop")
	}
	broken := filepath.Join(filepath.Dir(model), "broken.gguf")
	os.WriteFile(broken, []byte("GGUF"), 0o644)
	if _, err := Start(t.Context(), self, broken, nil, nil); err == nil || !strings.Contains(err.Error(), "this is no model") {
		t.Errorf("a server that stopped while loading: %v", err)
	}
}

func TestReadStream(t *testing.T) {
	for _, tc := range []struct{ body, want, err string }{
		{"data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\ndata: [DONE]\n", "Hi", ""},
		{"data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n", "", "stopped before it was finished"},
		{"data: {\"error\":{\"message\":\"out of memory\"}}\n", "", "out of memory"},
		{"data: nonsense\n", "", "not JSON"},
	} {
		a, err := readStream(strings.NewReader(tc.body), func() {}, nil)
		if a.Text != tc.want || (tc.err == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tc.err)) {
			t.Errorf("%q: %q %v", tc.body, a.Text, err)
		}
	}
}

// The host from several goroutines at once: one load for one model, every
// holder gets the same server, and the server stops only once the last
// let go and the wait ran out, or the app quits.
func TestHostFromSeveralGoroutines(t *testing.T) {
	var launched, stopped atomic.Int32
	var mu sync.Mutex
	var gone []func()
	h := &Host{
		Launch: func(ctx context.Context, model string) (*Server, error) {
			launched.Add(1)
			return &Server{URL: model, stop: func() { stopped.Add(1) }}, nil
		},
		Idle: func(_ time.Duration, f func()) func() bool {
			mu.Lock()
			gone = append(gone, f)
			mu.Unlock()
			return func() bool { return true }
		},
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			s, release, err := h.Hold(t.Context(), "/models/a.gguf")
			if err != nil || s.URL != "/models/a.gguf" {
				t.Errorf("%v %v", s, err)
				return
			}
			h.Loaded("/models/a.gguf")
			release(Keep)
			release(Keep)
		})
	}
	wg.Wait()
	if launched.Load() != 1 || stopped.Load() != 0 || !h.Loaded("/models/a.gguf") {
		t.Fatalf("%d launched, %d stopped", launched.Load(), stopped.Load())
	}
	// The wait runs out, and the model goes.
	mu.Lock()
	last := gone[len(gone)-1]
	mu.Unlock()
	last()
	if stopped.Load() != 1 || h.Loaded("/models/a.gguf") {
		t.Errorf("%d stopped after the wait", stopped.Load())
	}
	// Another model takes the place of one nobody holds.
	_, r1, _ := h.Hold(t.Context(), "/models/a.gguf")
	if _, _, err := h.Hold(t.Context(), "/models/b.gguf"); !errors.Is(err, errBusy) {
		t.Errorf("a second model beside one in use: %v", err)
	}
	r1(Keep)
	if _, r2, err := h.Hold(t.Context(), "/models/b.gguf"); err != nil {
		t.Fatal(err)
	} else {
		r2(Keep)
	}
	if !h.Drop("/models/b.gguf") || h.Loaded("/models/b.gguf") {
		t.Error("a model nobody holds was not dropped")
	}
	h.Close()
	if _, _, err := h.Hold(t.Context(), "/models/a.gguf"); !errors.Is(err, ErrClosed) {
		t.Errorf("after Close: %v", err)
	}
}

// Quitting while a model loads stops the load, and no server is left.
func TestCloseWhileLoading(t *testing.T) {
	started := make(chan struct{})
	h := &Host{Launch: func(ctx context.Context, model string) (*Server, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	errs := make(chan error)
	go func() {
		_, _, err := h.Hold(context.Background(), "/models/a.gguf")
		errs <- err
	}()
	<-started
	h.Close()
	if err := <-errs; err == nil {
		t.Error("the load went on after Close")
	}
}

// A server that stopped answering is let go of at once, not kept.
func TestAStalledServerGoesAtOnce(t *testing.T) {
	var stopped atomic.Int32
	h := &Host{
		Launch: func(ctx context.Context, model string) (*Server, error) {
			return &Server{URL: model, stop: func() { stopped.Add(1) }}, nil
		},
		Idle: func(time.Duration, func()) func() bool { return func() bool { return true } },
	}
	defer h.Close()
	_, release, err := h.Hold(t.Context(), "/models/a.gguf")
	if err != nil {
		t.Fatal(err)
	}
	release(0)
	if stopped.Load() != 1 || h.Loaded("/models/a.gguf") {
		t.Errorf("%d stopped, loaded %v", stopped.Load(), h.Loaded("/models/a.gguf"))
	}
}

func TestRoomForAModel(t *testing.T) {
	const gb = 1_000_000_000
	if err := roomFor(17*gb, 20*gb, nil); err != nil {
		t.Errorf("fits: %v", err)
	}
	if err := roomFor(17*gb, 0, nil); err != nil {
		t.Errorf("the system did not say: %v", err)
	}
	err := roomFor(17*gb, 12*gb, nil)
	if !errors.Is(err, ErrNoRoom) || !strings.Contains(err.Error(), "needs about 17.0 GB and 12.0 GB are free") {
		t.Errorf("too little: %v", err)
	}
	if err := roomFor(17*gb, 12*gb, []string{"4242"}); err == nil || !strings.Contains(err.Error(), "process 4242") {
		t.Errorf("another server: %v", err)
	}
}

func TestVMStatFree(t *testing.T) {
	const text = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               12000.
Pages active:                            500000.
Pages wired down:                        300000.
Pages occupied by compressor:            100000.
`
	if got, want := vmStatFree(text, 32<<30), int64(32<<30)-400000*16384; got != want {
		t.Errorf("%d, want %d", got, want)
	}
	if vmStatFree("nonsense", 32<<30) != 0 || vmStatFree(text, 0) != 0 {
		t.Error("what is not vm_stat's was read")
	}
}

func TestLlamaServers(t *testing.T) {
	got := llamaServers("  1 /sbin/launchd\n 4242 /Users/x/FrameFairy.app/Contents/MacOS/llama-server\n 77 llama-server-other\n")
	if strings.Join(got, ",") != "4242" {
		t.Errorf("%q", got)
	}
}

// A server whose app has gone is stopped when the app starts again. One
// whose app still runs, and a number that is no longer that server, are
// left alone.
func TestLeftoverServers(t *testing.T) {
	t.Setenv("FAKE_LLAMA_SERVER", "1")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	model := filepath.Join(dir, "probe.gguf")
	os.WriteFile(model, []byte("GGUF"), 0o644)
	left, err := Start(t.Context(), self, model, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer left.Stop()
	kept, err := Start(t.Context(), self, model, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer kept.Stop()
	// A process that has ended, the app that crashed.
	gone := exec.Command(self, "-test.run=^$")
	gone.Env = append(os.Environ(), "FAKE_LLAMA_SERVER=")
	if err := gone.Run(); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(dir, "notes")
	noteServer(notes, serverNote{PID: left.PID, Port: left.Port, Model: model, Owner: gone.Process.Pid})
	noteServer(notes, serverNote{PID: kept.PID, Port: kept.Port, Model: model})
	// The number is a live process, but not that server.
	noteServer(notes, serverNote{PID: os.Getpid(), Port: 1, Model: model, Owner: gone.Process.Pid})
	var asked []int
	stopped := stopLeftovers(notes, func(p *os.Process) error {
		asked = append(asked, p.Pid)
		return p.Signal(os.Interrupt)
	})
	if len(stopped) != 1 || stopped[0] != left.PID || len(asked) != 1 {
		t.Errorf("stopped %v, asked %v, want %d", stopped, asked, left.PID)
	}
	if _, err := http.Get(left.URL + "/health"); err == nil {
		t.Error("the leftover still answers")
	}
	if _, err := os.Stat(noteFile(notes, kept.PID)); err != nil {
		t.Error("the note of a server in use went")
	}
	if _, err := os.Stat(noteFile(notes, left.PID)); !errors.Is(err, os.ErrNotExist) {
		t.Error("the note of the leftover stayed")
	}
}

// A model FrameFairy has whole is taken from its folder, not downloaded,
// and its folder is left as it was. One that is not the model is not.
func TestInstallTakesFrameFairysCopy(t *testing.T) {
	m, data := fakeModel(strings.Repeat("weights", 1000))
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked++; w.Write(data) }))
	defer srv.Close()
	m.URL = srv.URL + "/probe.gguf"
	theirs := t.TempDir()
	os.WriteFile(filepath.Join(theirs, m.Name), data, 0o644)
	dir := t.TempDir()
	if err := (Installer{Elsewhere: []string{theirs}}).Install(t.Context(), m, dir); err != nil {
		t.Fatal(err)
	}
	if asked != 0 || !m.Installed(dir) || Check(m, dir) != nil || !m.Shared(dir) {
		t.Errorf("%d downloads, installed %v, shared %v", asked, m.Installed(dir), m.Shared(dir))
	}
	if err := Remove(m, dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(theirs, m.Name)); string(got) != string(data) {
		t.Error("FrameFairy's copy went with ours")
	}
	// Changed there, so it is not the model: downloaded instead.
	os.WriteFile(filepath.Join(theirs, m.Name), append([]byte("GGUF"), make([]byte, len(data)-4)...), 0o644)
	if err := (Installer{Elsewhere: []string{theirs}}).Install(t.Context(), m, dir); err != nil || asked != 1 || Check(m, dir) != nil {
		t.Errorf("a wrong copy: %v, %d downloads", err, asked)
	}
}

// Checking FrameFairy's copy reads all of it, 14.4 GB for Gemma 4 26B, so
// it says how far it is and about how long is left, the way a download
// does, and Cancel stops it. Tim saw only a beam for a few seconds and
// doubted anything had come, 21 November 2026.
func TestCheckingFrameFairysCopySaysHowFar(t *testing.T) {
	m, data := fakeModel(strings.Repeat("weights", 300_000))
	m.URL = "http://127.0.0.1:1/never"
	theirs := t.TempDir()
	os.WriteFile(filepath.Join(theirs, m.Name), data, 0o644)
	clock := time.Unix(0, 0)
	var said []fetch.Progress
	in := Installer{Elsewhere: []string{theirs},
		Now: func() time.Time { clock = clock.Add(time.Second); return clock },
		Report: func(p *fetch.Progress) {
			if p != nil {
				said = append(said, *p)
			}
		}}
	dir := t.TempDir()
	if err := in.Install(t.Context(), m, dir); err != nil || !m.Installed(dir) {
		t.Fatalf("not taken: %v", err)
	}
	var far []fetch.Progress
	for _, p := range said {
		if p.Text == "copying" && p.Fraction > 0 && p.Fraction < 1 && p.Remaining > 0 {
			far = append(far, p)
		}
	}
	if len(far) < 2 || far[0].Fraction >= far[len(far)-1].Fraction {
		t.Errorf("the check did not say how far it was: %+v", said)
	}

	// Cancelled from another goroutine while it checks, as Cancel in
	// Settings is: nothing is taken and nothing downloads.
	ctx, cancel := context.WithCancel(t.Context())
	dir = t.TempDir()
	checking := make(chan struct{})
	cancelled := make(chan struct{})
	var once sync.Once
	in.Report = func(p *fetch.Progress) {
		if p != nil && p.Fraction > 0 {
			once.Do(func() {
				close(checking)
				<-cancelled
			})
		}
	}
	go func() {
		<-checking
		cancel()
		close(cancelled)
	}()
	if err := in.Install(ctx, m, dir); !errors.Is(err, fetch.ErrCancelled) || m.Installed(dir) {
		t.Errorf("Cancel during the check: %v, installed %v", err, m.Installed(dir))
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("Cancel left %d files behind", len(entries))
	}
}

// A download that arrived whole and was stopped before it was moved into
// place is read through again to check it, saying how far it is, and is
// not fetched again.
func TestAWholeDownloadIsCheckedNotFetched(t *testing.T) {
	m, data := fakeModel(strings.Repeat("weights", 300_000))
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked++; w.Write(data) }))
	defer srv.Close()
	m.URL = srv.URL + "/probe.gguf"
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, m.Name+".part"), data, 0o644)
	clock := time.Unix(0, 0)
	checked := false
	in := Installer{Now: func() time.Time { clock = clock.Add(time.Second); return clock },
		Report: func(p *fetch.Progress) { checked = checked || (p != nil && p.Text == "checking" && p.Fraction > 0) }}
	if err := in.Install(t.Context(), m, dir); err != nil || !m.Installed(dir) || asked != 0 || !checked {
		t.Errorf("%v, installed %v, %d downloads, said it checked %v", err, m.Installed(dir), asked, checked)
	}
}
