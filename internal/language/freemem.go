package language

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Whether a model fits in the memory that is free now. FrameFairy's
// engine/freemem.go.
//
// A model is offered by the memory the Mac has, see FitsIn. What is free
// when a draft starts is another matter: a llama-server left behind, by
// this app or by FrameFairy, or any program holding memory, can leave too
// little. macOS then kills the new llama-server while it loads, and all a
// draft could say was that it stopped. FrameFairy saw it on 3 October
// 2026, with a server from an earlier run holding 16 GB of a 32 GB Mac.
// Tim runs both apps, each with Gemma 4 26B.
//
// So before llama-server starts, what the model needs is held against the
// memory the system can give it now. One that does not fit is not
// started, and the draft says why.

// FreeMemory is how much memory the system can give a program now, in
// bytes, or zero when it cannot tell. Memory the system only uses to cache
// files counts as free, because it gives that back as soon as it is asked,
// and on macOS so does what other programs hold, see vmStatFree.
func FreeMemory() int64 {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("vm_stat").Output()
		if err != nil {
			return 0
		}
		return vmStatFree(string(out), MachineMemory())
	case "linux":
		return meminfo("/proc/meminfo", "MemAvailable:")
	}
	return 0
}

// vmStatFree reads what vm_stat says into the memory that can be had: all
// of it, less what the system has wired down and what the compressor
// holds. Wired is where a model sits once Metal has it, and nothing can
// have it until that program lets go. The memory other programs hold is
// not counted: macOS compresses it, or moves it to disk, the moment a
// model asks for room. FrameFairy counted it at first, the way Activity
// Monitor counts the memory used, and a 32 GB Mac with Chrome open and its
// memory pressure green had 12.8 GB "free" for a model of 17. Zero when
// the text is not what vm_stat writes.
func vmStatFree(text string, total int64) int64 {
	if total <= 0 {
		return 0
	}
	page := int64(0)
	if _, rest, ok := strings.Cut(text, "page size of "); ok {
		if f := strings.Fields(rest); len(f) > 0 {
			if n, err := strconv.ParseInt(f[0], 10, 64); err == nil {
				page = n
			}
		}
	}
	if page <= 0 {
		return 0
	}
	pages := map[string]int64{}
	for _, line := range strings.Split(text, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(value), "."), 10, 64)
		if err == nil {
			pages[strings.TrimSpace(name)] = n
		}
	}
	wired, ok1 := pages["Pages wired down"]
	compressed, ok2 := pages["Pages occupied by compressor"]
	if !ok1 || !ok2 {
		return 0
	}
	return max(total-(wired+compressed)*page, 0)
}

// modelNeeds is the memory the model at path takes at Context: what the
// app knows of it, or for a model it does not know its file, which it
// takes at least.
func modelNeeds(path string) int64 {
	if m, ok := ByName(Models(), filepath.Base(path)); ok {
		return m.Needs
	}
	if info, err := os.Stat(path); err == nil {
		return info.Size() + workingAllowance
	}
	return 0
}

// ErrNoRoom is a model that does not fit in the memory free now.
var ErrNoRoom = errors.New("not enough memory for the language model")

// roomFor says why a model that needs need bytes cannot start with free
// bytes free, or nothing when it can, or when the system does not say what
// is free. servers are the llama-servers running now, which hold memory
// somebody else started them for, since none of this app's own is.
func roomFor(need, free int64, servers []string) error {
	if need <= 0 || free <= 0 || need <= free {
		return nil
	}
	said := fmt.Sprintf(". It needs about %s and %s are free.", inGB(need), inGB(free))
	if len(servers) > 0 {
		return fmt.Errorf("%w%s Another llama-server holds memory, process %s. Quit the app that started it, FrameFairy perhaps",
			ErrNoRoom, said, strings.Join(servers, ", "))
	}
	return fmt.Errorf("%w%s Quit other programs, or download a smaller model in Settings", ErrNoRoom, said)
}

// inGB is a size in gigabytes, to a tenth, the way Settings says one.
func inGB(n int64) string {
	return strconv.FormatFloat(float64(n)/1e9, 'f', 1, 64) + " GB"
}

// runningServers is the process numbers of the llama-servers the system
// runs now.
func runningServers() []string {
	out, err := exec.Command("ps", "-axo", "pid=,comm=").Output()
	if err != nil {
		return nil
	}
	return llamaServers(string(out))
}

// llamaServers picks the process numbers of llama-server out of what ps
// lists, one process a line, its number and then its program.
func llamaServers(listing string) []string {
	var pids []string
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && filepath.Base(fields[len(fields)-1]) == ServerName {
			pids = append(pids, fields[0])
		}
	}
	return pids
}
