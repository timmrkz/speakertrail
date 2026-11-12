package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// appVersion is the app's version. A build from a channel says its own,
// like 0.1.0-pr9.a1b2c3d, see buildVersion.
const appVersion = "0.1.0"

// loadJSON reads a small file of the app's beside its main file. A file
// that is missing or cannot be read leaves v as it is.
func loadJSON(path string, v any) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, v)
}

// saveJSON writes such a file whole, to a new file that then takes the
// old one's place, the way the store writes its own.
func saveJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".updates-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
