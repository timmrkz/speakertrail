package language

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Running the model in llama.cpp's server, the way FrameFairy runs it: a
// program of its own beside the app, started with the model, asked over
// HTTP on the loopback, and stopped again. See FrameFairy's local.go.

// ServerName is the program's name. It lies beside the app's own program,
// in Contents/MacOS of the bundle, built by make, see scripts/build-llama.sh.
const ServerName = "llama-server"

// FindServer is the llama-server the app runs: the one named in
// SPEAKERTRAIL_LLAMA_SERVER, or else the one beside the app's program.
// Nothing is taken from the search path, so what runs is what was built
// and shipped.
func FindServer() (string, error) {
	if named := os.Getenv("SPEAKERTRAIL_LLAMA_SERVER"); named != "" {
		if _, err := exec.LookPath(named); err != nil {
			return "", fmt.Errorf("SPEAKERTRAIL_LLAMA_SERVER names %s, which cannot be run", named)
		}
		return named, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	beside := filepath.Join(filepath.Dir(self), ServerName)
	if info, err := os.Stat(beside); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
		return "", errors.New("llama-server is not beside the app, so no model can run. Build the app again with make")
	}
	return beside, nil
}

// Server is a llama-server running one model.
type Server struct {
	// URL is where it answers, on the loopback.
	URL string
	// Key is what it answers to, see newKey.
	Key string
	// PID is its process, Port where it listens.
	PID, Port int
	stop      func()
}

// Stop stops the server and returns once it has gone.
func (s *Server) Stop() {
	if s != nil && s.stop != nil {
		s.stop()
	}
}

// A llama-server the app starts answers only to the app. Left to itself it
// answers any program on the Mac and any web page open in a browser, since
// it allows every origin, and it lists what it is working on, which is a
// person's profile. So each server gets a key of its own, made when it
// starts and handed over in its environment rather than on its command
// line, where every program on the Mac could read it, and the list of
// what it works on is switched off. As in FrameFairy.
func newKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// promptBatch is how many tokens of the prompt llama-server reads before
// it says how far it has come, see FrameFairy's local.go.
const promptBatch = 512

// stopGrace is how long llama-server has to go by itself once it is asked
// to. Idle, it goes well inside it. After it, it is killed: it has nothing
// to save.
const stopGrace = 500 * time.Millisecond

// loadLimit is how long a model may take to load. Gemma 4 26B loads in 9
// to 24 seconds on an M2 Max, as FrameFairy measured.
const loadLimit = 10 * time.Minute

// readyMarks are the lines llama-server writes once it may be ready. Each
// is followed by a look at its health, which decides.
var readyMarks = []string{"model loaded", "server is listening"}

// saidStoppedLoading is the log's line for a server that stopped while it
// loaded. A test reads the notes as it is said.
const saidStoppedLoading = "llama-server stopped while loading the model"

// Start runs llama-server with the model at path and waits until the model
// is loaded. log hears the server's last lines when it fails, and how long
// the load took when it does not. Nothing it says while it works is
// logged, since it is about a person's profile.
//
// The server is written down in notes the moment it starts, and the note
// goes once it has stopped, see leftover.go. Written only once the model
// had loaded, as it was at first, a server whose app ended while it
// loaded was in no note, and no later start of the app stopped it.
// FrameFairy writes it down at the same moment.
func Start(ctx context.Context, server, model, notes string, client *http.Client, log *slog.Logger) (*Server, error) {
	if _, err := os.Stat(model); err != nil {
		return nil, fmt.Errorf("the language model cannot be read: %w", err)
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	key, err := newKey()
	if err != nil {
		return nil, err
	}
	args := []string{"-m", model, "--host", "127.0.0.1", "--port", strconv.Itoa(port),
		"-c", strconv.Itoa(Context), "-ngl", "999",
		// One ask at a time, with the whole context for it.
		"-np", "1",
		// Nothing reads what the server works on, and it is a profile.
		"--no-slots",
		"-b", strconv.Itoa(promptBatch),
		"--ctx-checkpoints", strconv.Itoa(checkpoints),
		// No copies of earlier prompts kept in memory, see FrameFairy.
		"--cache-ram", "0", "--no-cache-idle-slots"}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	cmd := exec.Command(server, args...)
	cmd.Env = append(os.Environ(), "LLAMA_API_KEY="+key)
	ready := make(chan struct{}, 1)
	out := &serverOutput{ready: ready}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start llama-server: %w", err)
	}
	noteServer(notes, serverNote{PID: cmd.Process.Pid, Port: port, Model: model})
	exited := make(chan struct{})
	var exitErr error
	go func() {
		exitErr = cmd.Wait()
		close(exited)
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			defer forgetServer(notes, cmd.Process.Pid)
			select {
			case <-exited:
				return
			default:
			}
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case <-exited:
			case <-time.After(stopGrace):
				_ = cmd.Process.Kill()
				<-exited
			}
		})
	}
	s := &Server{URL: url, Key: key, PID: cmd.Process.Pid, Port: port, stop: stop}
	started := time.Now()
	look := time.NewTicker(500 * time.Millisecond)
	defer look.Stop()
	for {
		select {
		case <-ctx.Done():
			stop()
			return nil, ctx.Err()
		case <-exited:
			if log != nil {
				log.Warn(saidStoppedLoading, "error", exitErr, "said", out.last())
			}
			stop()
			return nil, stoppedLoading(exitErr, out.last())
		case <-ready:
		case <-look.C:
		}
		if healthy(ctx, client, s) {
			if log != nil {
				log.Info("the language model is loaded", "ms", time.Since(started).Milliseconds(), "pid", s.PID)
			}
			return s, nil
		}
		if time.Since(started) > loadLimit {
			stop()
			return nil, fmt.Errorf("the language model did not finish loading within %s.%s", loadLimit, saidLast(out.last()))
		}
	}
}

// healthy says whether the server says it is ready, asking for no longer
// than two seconds.
func healthy(ctx context.Context, client *http.Client, s *Server) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL+"/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+s.Key)
	res, err := orDefault(client).Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

// orDefault is client, or one that follows no redirect when it is nil:
// the server is on the loopback and has nowhere to send the app.
func orDefault(client *http.Client) *http.Client {
	if client != nil {
		return client
	}
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// stoppedLoading is the server ending while it loaded. Killed while
// loading is what macOS does when memory runs out.
func stoppedLoading(exitErr error, last string) error {
	if strings.Contains(fmt.Sprint(exitErr), "killed") {
		return fmt.Errorf("llama-server was stopped by the system while it loaded the model, which it does when memory runs out. Quit other programs, or download a smaller model in Settings.%s", saidLast(last))
	}
	return fmt.Errorf("llama-server stopped while it loaded the model (%v).%s", exitErr, saidLast(last))
}

func saidLast(last string) string {
	if last == "" {
		return ""
	}
	return " It said last: " + last
}

// serverOutput hears llama-server's output a line at a time: it says on
// ready when a line holds one of readyMarks, and keeps the last lines,
// which an error while loading quotes.
type serverOutput struct {
	ready chan<- struct{}
	mu    sync.Mutex
	part  []byte
	lines []string
}

func (o *serverOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.part = append(o.part, p...)
	for {
		end := bytes.IndexByte(o.part, '\n')
		if end < 0 {
			break
		}
		line := strings.TrimSpace(string(o.part[:end]))
		o.part = o.part[end+1:]
		if line == "" {
			continue
		}
		for _, mark := range readyMarks {
			if strings.Contains(line, mark) {
				select {
				case o.ready <- struct{}{}:
				default:
				}
			}
		}
		o.lines = append(o.lines, line)
		if len(o.lines) > 6 {
			o.lines = o.lines[1:]
		}
	}
	// A line that never ends cannot fill memory.
	if len(o.part) > 64<<10 {
		o.part = o.part[:0]
	}
	return len(p), nil
}

// last is the last lines said, as many of the newest as fit in 400
// letters.
func (o *serverOutput) last() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	joined := []rune(strings.Join(o.lines, " "))
	if len(joined) > 400 {
		joined = append([]rune("…"), joined[len(joined)-400:]...)
	}
	return string(joined)
}
