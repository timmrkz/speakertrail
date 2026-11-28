package main

// toolchain says what this machine has for building Speaker Trail, and how
// to get what is missing. It changes nothing. make runs it before every
// build, after make tools installed what it could, and make check runs it
// with -all. It was scripts/check.sh, and moved here with #57, so the
// rule of what a build needs is Go with a test.
//
// Go is asked about itself: the Go that runs this is the Go make builds
// with, since make runs it with go run. A Go older than go.mod asks for
// never gets this far, go run says so itself.

import (
	"flag"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// The oldest Go and Node.js a build works with. Go's is the go line of
// go.mod, Node's what Vite asks for.
const (
	minGoMinor = 27
	minNode    = 22
)

// machine is what toolchain asks of the computer it runs on, so a test
// can be any computer.
type machine struct {
	goos      string
	goVersion string
	// has says whether a program is on the path.
	has func(name string) bool
	// output runs a program and gives what it wrote, or an error when it
	// failed or is not there.
	output func(name string, args ...string) (string, error)
}

// thisMachine is the computer this runs on.
func thisMachine() machine {
	return machine{
		goos:      runtime.GOOS,
		goVersion: runtime.Version(),
		has: func(name string) bool {
			_, err := exec.LookPath(name)
			return err == nil
		},
		output: func(name string, args ...string) (string, error) {
			out, err := exec.Command(name, args...).Output()
			return strings.TrimSpace(string(out)), err
		},
	}
}

// tool is one thing a build needs, as found on the machine.
type tool struct {
	// what is the thing, as said to Tim, and found what was found of it,
	// when that says more.
	what, found string
	ok          bool
	// fix is the command that gets it.
	fix string
}

// survey finds what a build needs on m, in the order it is said.
func survey(m machine) []tool {
	var tools []tool
	goTool := tool{what: fmt.Sprintf("Go 1.%d or newer", minGoMinor), fix: "brew upgrade go"}
	version := strings.TrimPrefix(m.goVersion, "go")
	if minor, ok := goMinor(version); ok && minor >= minGoMinor {
		goTool = tool{what: "Go " + version, ok: true}
	} else {
		goTool.found = version
	}
	tools = append(tools, goTool)

	node := tool{what: fmt.Sprintf("Node.js %d or newer, for the interface", minNode), fix: "brew install node"}
	if m.has("node") {
		node.fix = "brew upgrade node"
		major, err := m.output("node", "-p", `process.versions.node.split(".")[0]`)
		n, convErr := strconv.Atoi(major)
		switch {
		case err != nil || convErr != nil:
			node.found = "a node that says no version"
		case n < minNode:
			v, _ := m.output("node", "-v")
			node.found = v
		default:
			v, _ := m.output("node", "-v")
			node = tool{what: "Node.js " + v, ok: true}
		}
	}
	tools = append(tools, node)

	if m.goos == "darwin" {
		_, err := m.output("xcode-select", "-p")
		tools = append(tools, tool{what: "the command line tools, for the app's window", ok: err == nil, fix: "xcode-select --install"})
		tools = append(tools, tool{what: "CMake, for the llama-server the app carries", ok: m.has("cmake"), fix: "brew install cmake"})
	} else {
		_, err := m.output("pkg-config", "--exists", "gtk4", "webkitgtk-6.0")
		tools = append(tools, tool{what: "GTK 4 and WebKitGTK 6, for the app's window", ok: err == nil, fix: "sudo apt install libgtk-4-dev libwebkitgtk-6.0-dev"})
	}
	return tools
}

// goMinor is the minor version of a Go version like 1.27.1 or 1.28rc1.
func goMinor(version string) (int, bool) {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 || parts[0] != "1" {
		return 0, false
	}
	digits := parts[1]
	for i, r := range digits {
		if r < '0' || r > '9' {
			digits = digits[:i]
			break
		}
	}
	n, err := strconv.Atoi(digits)
	return n, err == nil
}

// report writes what survey found. Without all, only what is missing,
// which is how make runs it before a build, and a missing thing is then
// an error that stops the build. With all, everything, and it never
// fails, which is make check.
func report(out io.Writer, tools []tool, all bool) error {
	missing := 0
	for _, t := range tools {
		if t.ok {
			if all {
				fmt.Fprintf(out, "  ok       %s\n", t.what)
			}
			continue
		}
		missing++
		what := t.what
		if t.found != "" {
			what += ", found " + t.found
		}
		fmt.Fprintf(out, "  missing  %s\n           %s\n", what, t.fix)
	}
	switch {
	case all && missing == 0:
		fmt.Fprintln(out, "Everything is here.")
	case !all && missing > 0:
		return fmt.Errorf("this machine is missing %d of what a build needs, see above", missing)
	}
	return nil
}

func toolchainCommand(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("toolchain", flag.ContinueOnError)
	all := fs.Bool("all", false, "say everything found, not only what is missing, and never fail")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return report(out, survey(thisMachine()), *all)
}
