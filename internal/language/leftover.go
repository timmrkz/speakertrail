package language

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A llama-server left behind. FrameFairy's engine/leftover.go.
//
// The app stops its model when it quits. An app that crashes, or is
// killed from Activity Monitor, stops nothing, and llama-server goes on
// holding the model in memory, 16 GB of a 32 GB Mac, until the Mac is
// restarted. Nothing on screen says so, and the next draft loads a second
// copy beside it.
//
// So a running server is written down, its process, its port, its model
// and the process that started it, one note per server in the app's
// folder, and the note goes when the server is stopped. A note whose
// owner has gone is a server left behind. When the app starts it is
// stopped if it is still running and is exactly that server, the same
// process with the same port and the same model. Anything else under that
// number is somebody else's and is left alone, FrameFairy's own servers
// among them, which it notes in a folder of its own.

// NotesDir is where the notes live in the app's folder.
func NotesDir(appFolder string) string {
	return filepath.Join(appFolder, "llama-servers")
}

type serverNote struct {
	PID   int    `json:"pid"`
	Port  int    `json:"port"`
	Model string `json:"model"`
	// Owner is the process that started the server.
	Owner int `json:"owner"`
}

func noteFile(dir string, pid int) string {
	return filepath.Join(dir, strconv.Itoa(pid)+".json")
}

// noteServer writes down a server that has just started, owned by this
// process.
func noteServer(dir string, n serverNote) {
	if dir == "" {
		return
	}
	if n.Owner == 0 {
		n.Owner = os.Getpid()
	}
	body, err := json.Marshal(n)
	if err != nil {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	tmp := noteFile(dir, n.PID) + ".tmp"
	if os.WriteFile(tmp, body, 0o644) == nil {
		_ = os.Rename(tmp, noteFile(dir, n.PID))
	}
}

// forgetServer takes the note away once that server has stopped.
func forgetServer(dir string, pid int) {
	if dir != "" {
		_ = os.Remove(noteFile(dir, pid))
	}
}

// StopLeftoverServers stops the llama-servers an app gone before left
// running, and gives the process numbers of those it stopped. The app
// calls it when it starts.
func StopLeftoverServers(dir string) []int {
	return stopLeftovers(dir, func(p *os.Process) error { return p.Signal(os.Interrupt) })
}

// stopLeftovers is StopLeftoverServers, asking a server to stop through
// interrupt, so a test hears which were asked.
func stopLeftovers(dir string, interrupt func(*os.Process) error) []int {
	if dir == "" || runtime.GOOS == "windows" {
		return nil
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	var stopped []int
	for _, path := range paths {
		var n serverNote
		data, err := os.ReadFile(path)
		ok := err == nil && json.Unmarshal(data, &n) == nil && n.PID > 0
		if ok && n.Owner != 0 && alive(n.Owner) {
			continue
		}
		_ = os.Remove(path)
		if ok && stopServer(n, interrupt) {
			stopped = append(stopped, n.PID)
		}
	}
	return stopped
}

// stopServer stops the server a note names, if it is still that server.
func stopServer(n serverNote, interrupt func(*os.Process) error) bool {
	if !isThatServer(n) {
		return false
	}
	proc, err := os.FindProcess(n.PID)
	if err != nil {
		return false
	}
	_ = interrupt(proc)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !isThatServer(n) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = proc.Kill()
	return true
}

// alive says whether a process with this number is running. A number the
// system has given to another program since reads as alive too, which
// leaves a leftover running rather than stopping a server in use.
func alive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// isThatServer says whether the process under the note's number is still
// the server the note was written for: its command line holds the same
// port and the same model. The command line is compared as a whole, so a
// model path with a space in it still matches.
func isThatServer(n serverNote) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(n.PID), "-o", "args=").Output()
	if err != nil {
		return false
	}
	line := " " + strings.TrimSpace(string(out)) + " "
	return strings.Contains(line, " --port "+strconv.Itoa(n.Port)+" ") &&
		strings.Contains(line, " -m "+n.Model+" ")
}
