// Package standin is the one way a test writes a program it then runs, as
// a shell script.
//
// A program that is open for writing cannot be run. Linux refuses it with
// "text file busy". The file is open only while it is written, but a fork
// made by another test in that moment copies the open file into the child,
// where it stays until the child has started its own program. The tests
// run side by side, so now and then a stand-in was refused the first time
// it was run.
//
// Write holds syscall.ForkLock for reading while the file is open. Every
// fork in Go creates its child while it holds the lock for writing, so no
// child is created while the file is open, and the program can be run the
// moment Write returns.
package standin

import (
	"io/fs"
	"os"
	"syscall"
)

// Write writes a program the way os.WriteFile writes a file.
func Write(path string, body []byte, mode fs.FileMode) error {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	return os.WriteFile(path, body, mode)
}
