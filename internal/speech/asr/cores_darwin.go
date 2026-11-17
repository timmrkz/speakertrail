//go:build darwin

package asr

import "golang.org/x/sys/unix"

// fastCores is how many performance cores the Mac has. Apple's chips have
// performance cores and efficiency cores, and the speech model is heard as
// fast as the performance cores go: on an M2 Max, 8 of its 12, threads past
// 8 made it slower. Zero when macOS does not say, on a Mac with one kind.
func fastCores() int {
	n, err := unix.SysctlUint32("hw.perflevel0.physicalcpu")
	if err != nil {
		return 0
	}
	return int(n)
}

// memory is the Mac's memory in bytes, zero when macOS does not say.
func memory() int64 {
	n, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return int64(n)
}
