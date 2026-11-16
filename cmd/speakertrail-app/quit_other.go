//go:build !darwin

package main

// quitBySystem says no away from macOS. Only a Mac's logout, restart and
// shutdown are told apart, see quit_darwin.go.
func quitBySystem() bool { return false }
