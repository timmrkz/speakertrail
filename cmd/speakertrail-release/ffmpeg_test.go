package main

import (
	"slices"
	"strings"
	"testing"
)

// The configure line asks for nothing GPL or paid, keeps only files and
// pipes as ways in and out, leaves out every format that reads other
// files, and asks for Apple's own coders only on a Mac.
func TestFFmpegIsConfiguredLGPLAndReadingOnlyFiles(t *testing.T) {
	for _, system := range []string{"darwin", "linux"} {
		args := ffmpegConfigure("/out", system)
		for _, bad := range []string{"--enable-gpl", "--enable-nonfree", "--enable-libx264", "--enable-network"} {
			if slices.Contains(args, bad) {
				t.Errorf("on %s the configure line asks for %s", system, bad)
			}
		}
		for _, want := range []string{"--prefix=/out", "--disable-network", "--disable-protocols", "--enable-protocol=file,pipe",
			"--disable-demuxer=" + strings.Join(listFormats, ","), "--disable-autodetect"} {
			if !slices.Contains(args, want) {
				t.Errorf("on %s the configure line has no %s: %v", system, want, args)
			}
		}
		if mac := slices.Contains(args, "--enable-videotoolbox"); mac != (system == "darwin") {
			t.Errorf("on %s the configure line asks for VideoToolbox: %v", system, mac)
		}
		// The Mac's iconv is a library of its own, which nothing else
		// links in without libass.
		if mac := slices.Contains(args, "--extra-libs=-liconv"); mac != (system == "darwin") {
			t.Errorf("on %s the configure line links libiconv: %v", system, mac)
		}
	}
}

// What ffmpeg -L says is read as the Lesser GPL only when it says so and
// does not say GPL.
func TestFFmpegsLicenceIsReadOffTheBinary(t *testing.T) {
	const lesser = `ffmpeg version 9.0.2 Copyright (c) 2000-2026 the FFmpeg developers

FFmpeg is free software; you can redistribute it and/or modify
it under the terms of the GNU Lesser General Public License as published by
the Free Software Foundation; either version 2.1 of the License, or
`
	const gpl = `ffmpeg version 9.0.2 Copyright (c) 2000-2026 the FFmpeg developers

FFmpeg is free software; you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation; either version 2 of the License, or
`
	if !lgpl(lesser) {
		t.Error("the Lesser GPL is not read as such")
	}
	if lgpl(gpl) {
		t.Error("the GPL is read as the Lesser GPL")
	}
	if lgpl("") {
		t.Error("nothing is read as the Lesser GPL")
	}
}

// The names of a list ffmpeg prints are read below its line of dashes,
// one or several to a line.
func TestFFmpegsListsAreRead(t *testing.T) {
	const encoders = `Encoders:
 V..... = Video
 A..... = Audio
 ------
 V....D libx264              libx264 H.264 / AVC / MPEG-4 AVC (codec h264)
 V....D png                  PNG (Portable Network Graphics) image
`
	if got := listed(encoders); !slices.Equal(got, []string{"libx264", "png"}) {
		t.Errorf("the encoders read are %v", got)
	}
	const demuxers = `File formats:
 D. = Demuxing supported
 E. = Muxing supported
 --
 D  hls             Apple HTTP Live Streaming
 D  mov,mp4,m4a,3gp,3g2,mj2 QuickTime / MOV
`
	if got := listed(demuxers); !slices.Equal(got, []string{"hls", "mov", "mp4", "m4a", "3gp", "3g2", "mj2"}) {
		t.Errorf("the demuxers read are %v", got)
	}
	const protocols = `Supported file protocols:
Input:
  file
  pipe
Output:
  file
  pipe
`
	if got := ioProtocols(protocols); !slices.Equal(got, []string{"file", "pipe"}) {
		t.Errorf("the protocols read are %v", got)
	}
	if got := ioProtocols(protocols + "  http\n"); !slices.Equal(got, []string{"file", "http", "pipe"}) {
		t.Errorf("a way out through http is read as %v", got)
	}
}

// A library of the machine that built ffmpeg is named, on a Mac and on
// Linux, and the system's own are not.
func TestALibraryOfTheMachineThatBuiltItIsNamed(t *testing.T) {
	const mac = `.build/ffmpeg/bin/ffmpeg:
	/System/Library/Frameworks/VideoToolbox.framework/Versions/A/VideoToolbox (compatibility version 1.0.0, current version 1.0.0)
	/usr/lib/libz.1.dylib (compatibility version 1.0.0, current version 1.2.12)
	/opt/homebrew/opt/libunibreak/lib/libunibreak.7.dylib (compatibility version 8.0.0, current version 8.0.0)
`
	if got := borrowedLibraries(mac, "darwin"); !slices.Equal(got, []string{"/opt/homebrew/opt/libunibreak/lib/libunibreak.7.dylib"}) {
		t.Errorf("on a Mac the borrowed are %v", got)
	}
	const linux = `	linux-vdso.so.1 (0x00007ffd)
	libm.so.6 => /lib/x86_64-linux-gnu/libm.so.6 (0x00007f)
	libz.so.1 => /lib/x86_64-linux-gnu/libz.so.1 (0x00007f)
	libharfbuzz.so.0 => /lib/x86_64-linux-gnu/libharfbuzz.so.0 (0x00007f)
	libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x00007f)
	/lib64/ld-linux-x86-64.so.2 (0x00007f)
`
	if got := borrowedLibraries(linux, "linux"); !slices.Equal(got, []string{"libharfbuzz.so.0"}) {
		t.Errorf("on Linux the borrowed are %v", got)
	}
}
