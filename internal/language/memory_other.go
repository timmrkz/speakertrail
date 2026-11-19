//go:build !darwin

package language

// systemMemory is asked only on a Mac, see MachineMemory.
func systemMemory() int64 { return 0 }
