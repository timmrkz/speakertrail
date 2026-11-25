package main

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
)

// Screen recordings in Report a Problem, issue #49. A walk through a
// problem says more than a screenshot, so a report takes the recordings
// the usual screen recorders make, within the same 10 MB a picture may
// have: MP4, MOV, M4V and MKV, which the Mac's own recorder, QuickTime,
// phones and OBS make, and WebM, which browsers make.
//
// A recording is a file attached as it is, byte for byte, and goes into
// the report under recordings/. The app reads nothing in it but its
// first bytes, which say whether it is a recording at all, so it carries
// no program to read videos (Tim, 25 November 2026). FrameFairy, a video
// editor, reads a recording with its ffmpeg and copies it without its
// metadata. Here the file keeps what it says about itself, the place and
// the device too, the way a file Tim hands over by mail would.

// recordingEndings are the endings a recording is kept under.
var recordingEndings = []string{"mp4", "mov", "m4v", "mkv", "webm"}

// recordingPattern is recordingEndings as the filter of a file box.
func recordingPattern() string {
	parts := make([]string, len(recordingEndings))
	for i, ending := range recordingEndings {
		parts[i] = "*." + ending
	}
	return strings.Join(parts, ";")
}

// sniffRecording says which family of recording data is, by its first
// bytes, "mov" for MP4 and QuickTime, "matroska" for MKV and WebM, or ""
// for none. A HEIC or AVIF picture is in the same box as a MOV and is no
// recording.
func sniffRecording(data []byte) string {
	if len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		return "matroska"
	}
	if len(data) < 12 {
		return ""
	}
	switch string(data[4:8]) {
	case "ftyp":
		switch string(data[8:12]) {
		case "heic", "heix", "heim", "heis", "hevc", "hevx", "mif1", "msf1", "avif", "avis":
			return ""
		}
		return "mov"
	case "moov", "mdat", "wide", "free", "skip":
		return "mov"
	}
	return ""
}

// recordingEnding is the ending a recording of family is kept under: the
// one its name had when that fits the family, and otherwise the one
// every recording of the family can be kept under.
func recordingEnding(family, name string) string {
	had := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if family == "matroska" {
		if had == "webm" {
			return "webm"
		}
		return "mkv"
	}
	if had == "mp4" || had == "m4v" {
		return had
	}
	return "mov"
}

// isRecordingEnding says whether a kept draft may name ending.
func isRecordingEnding(ending string) bool {
	return slices.Contains(recordingEndings, ending)
}
