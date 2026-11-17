//go:build !darwin

package asr

import (
	"os"
	"strconv"
	"strings"
)

// fastCores is zero where the system does not tell performance cores from
// others, and every core counts.
func fastCores() int { return 0 }

// memory is the machine's memory in bytes, from /proc/meminfo on Linux,
// zero where the system does not say.
func memory() int64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for line := range strings.Lines(string(data)) {
		if rest, ok := strings.CutPrefix(line, "MemTotal:"); ok {
			kb, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(rest), " kB"), 10, 64)
			if err != nil {
				return 0
			}
			return kb << 10
		}
	}
	return 0
}
