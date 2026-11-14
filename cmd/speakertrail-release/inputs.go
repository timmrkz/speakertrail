package main

// The app must never read keys typed into other apps, or watch the screen
// or the camera, FrameFairy issue #369, and never runs anything as the root user or
// with administrator rights, FrameFairy issue #751. macOS asks the person before any
// of that works, so what is checked here is that a build never asks: a
// build that can is refused before it is signed, and no app installs a
// build that is not signed, see updates.Verify. sign runs from main, see publish.yml, so
// these lists are main's, and a branch that changes them changes nothing
// about how its own build is judged.
//
// What this cannot catch is code that hides the name it looks up, built
// from pieces at run time. Then macOS is the wall that holds: none of
// these inputs works without the person allowing it in System Settings.

import (
	"archive/zip"
	"bytes"
	"debug/macho"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"
)

// watching are the functions and the Objective-C classes that read input
// meant for other apps or watch the screen or the camera. Each is refused
// as an import, and as text anywhere in a program, which catches one looked
// up by name at run time, with dlsym or NSClassFromString.
// RegisterEventHotKey is not one of them: Wails links it for global
// shortcuts, which take only the keys they are registered for, and the app
// registers none.
var watching = []string{
	// Every key and click of every app, with Input Monitoring.
	"CGEventTapCreate", "CGEventTapCreateForPid", "CGRequestListenEventAccess",
	"IOHIDManagerOpen", "IOHIDDeviceRegisterInputValueCallback",
	// Other apps' windows and what is typed into them, with Accessibility.
	"AXIsProcessTrusted", "AXIsProcessTrustedWithOptions", "AXUIElementCreateSystemWide",
	// The screen, with Screen Recording.
	"CGWindowListCreateImage", "CGDisplayStreamCreate", "SCStream", "SCShareableContent",
	// The camera.
	"AVCaptureDevice",
}

// raising are the ways a program on a Mac gets administrator rights, or
// runs something as the root user: the Authorization calls that ask for
// an administrator's password, a helper installed to run as root with
// SMJobBless or as a daemon of SMAppService, AppleScript's "with
// administrator privileges", and sudo. Each is refused the way watching
// is, as an import and as text anywhere in a program.
//
// Some look alike and stay. SMAppService's mainAppService, which Wails
// links to open the app at login, runs it as the person. setuid and
// setgid, which Go's own os/exec imports on every Mac, and seteuid like
// them, only ever give rights up, unless the program was started with
// more than the person's, which a file of the build marked set-user-ID
// would be, see setID. sudo
// is named by its path, because "sudo" is a word inside Go's own runtime,
// its sudog.
var raising = []string{
	"AuthorizationExecuteWithPrivileges", "AuthorizationCreate", "AuthorizationCopyRights",
	"SMJobBless", "daemonServiceWithPlistName",
	"with administrator privileges", "/usr/bin/sudo",
}

// setID are the bits of a file's mode that make whatever runs it run as
// the file's owner or group, root's when it was installed by root.
const setID = fs.ModeSetuid | fs.ModeSetgid

// linked are the libraries each program of the app links, by the name of
// its file. A library not on its list, ScreenCaptureKit, AVFoundation or
// IOKit for the keyboard's own reports, fails until the list is changed on
// main, in a pull request that says why. A program not on the list fails
// the same way. The app is one program: Wails' window, WebKit, and the
// keychain through Security.
var linked = map[string][]string{
	"speakertrail-app": {
		"/System/Library/Frameworks/AppKit.framework/Versions/C/AppKit",
		"/System/Library/Frameworks/Carbon.framework/Versions/A/Carbon",
		"/System/Library/Frameworks/Cocoa.framework/Versions/A/Cocoa",
		"/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation",
		"/System/Library/Frameworks/CoreGraphics.framework/Versions/A/CoreGraphics",
		"/System/Library/Frameworks/Foundation.framework/Versions/C/Foundation",
		"/System/Library/Frameworks/QuartzCore.framework/Versions/A/QuartzCore",
		"/System/Library/Frameworks/Security.framework/Versions/A/Security",
		"/System/Library/Frameworks/WebKit.framework/Versions/A/WebKit",
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libobjc.A.dylib", "/usr/lib/libresolv.9.dylib",
	},
}

// asks are the reasons Info.plist may give the person for what the app
// asks to reach. Speaker Trail asks for nothing: no folder, no microphone,
// no camera. A reason added fails.
var asks = []string{}

// entitled are the entitlements a program may be signed with: none. Above
// all not the camera, the microphone, Apple events, or what lets other code
// into the app, library validation turned off, the dyld environment or
// get-task-allow.
var entitled = []string{}

// maxUnpacked is the most the files of a build may hold once unpacked,
// all together. The app is some tens of MB. A zip that unpacks to more is
// not a build, and reading it whole would only fill the runner's memory.
const maxUnpacked = 1 << 30

// checkInputs reads every file of the zip and says everything in it that
// could read input meant for other apps, watch the screen or the camera,
// or ask for administrator rights.
func checkInputs(zipPath string) error {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("the build is not a zip: %w", err)
	}
	defer z.Close()
	// What each file says it holds is checked first, and what it really
	// holds as it is read, because a zip can say less than it holds.
	var total uint64
	for _, f := range z.File {
		total += f.UncompressedSize64
		if total > maxUnpacked {
			return fmt.Errorf("the build unpacks to more than %d MB, which no build of the app does", maxUnpacked>>20)
		}
	}
	// Each file is unpacked and read on its own goroutine: unpacking is
	// most of the time.
	type read struct {
		found []string
		err   error
	}
	reads := make([]read, len(z.File))
	var wg sync.WaitGroup
	for i, f := range z.File {
		if f.Mode()&setID != 0 {
			reads[i].found = append(reads[i].found, f.Name+": runs as its owner, set-user-ID or set-group-ID")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		wg.Go(func() {
			r, err := f.Open()
			if err != nil {
				reads[i].err = fmt.Errorf("%s: %w", f.Name, err)
				return
			}
			data, err := io.ReadAll(io.LimitReader(r, int64(f.UncompressedSize64)+1))
			r.Close()
			if err != nil {
				reads[i].err = fmt.Errorf("%s: %w", f.Name, err)
				return
			}
			if uint64(len(data)) > f.UncompressedSize64 {
				reads[i].err = fmt.Errorf("%s: holds more than the zip says it does", f.Name)
				return
			}
			for _, p := range inputsOf(f.Name, data) {
				reads[i].found = append(reads[i].found, f.Name+": "+p)
			}
		})
	}
	wg.Wait()
	var found []string
	for _, r := range reads {
		if r.err != nil {
			return r.err
		}
		found = append(found, r.found...)
	}
	if len(found) > 0 {
		return fmt.Errorf("the build could read input meant for other apps, watch the screen or the camera, or ask for administrator rights, so it is not signed. If that is meant, the lists in cmd/speakertrail-release/inputs.go change on main first, in a pull request that says why:\n  %s", strings.Join(found, "\n  "))
	}
	return nil
}

// inputsOf says what in one file of the build could read input or ask
// for administrator rights, or nothing.
//
// Every file is read for the names as text, whatever it is: a script or
// a library the app loads at run time is no Mach-O program and can name
// them all the same. A file that starts like a Mach-O program and cannot
// be read as one is refused, because what cannot be read cannot be
// checked.
func inputsOf(name string, data []byte) []string {
	var found []string
	for _, n := range slices.Concat(watching, raising) {
		if bytes.Contains(data, []byte(n)) {
			found = append(found, "names "+n)
		}
	}
	if path.Base(name) == "Info.plist" {
		keys, err := plistKeys(data)
		if err != nil {
			return append(found, fmt.Sprintf("cannot be read: %v", err))
		}
		for _, k := range keys {
			if strings.HasPrefix(k, "NS") && strings.HasSuffix(k, "UsageDescription") && !slices.Contains(asks, k) {
				found = append(found, "asks for "+k)
			}
		}
		return tidy(found)
	}
	// Each architecture with its own bytes: the offsets inside one, like
	// where its signature is, count from the start of its slice.
	type arch struct {
		f    *macho.File
		data []byte
	}
	var files []arch
	if fat, err := macho.NewFatFile(bytes.NewReader(data)); err == nil {
		for _, a := range fat.Arches {
			end := uint64(a.Offset) + uint64(a.Size)
			if end > uint64(len(data)) {
				return []string{fmt.Sprintf("its %v part reaches past the end of the file", a.Cpu)}
			}
			files = append(files, arch{a.File, data[a.Offset:end]})
		}
	} else if f, err := macho.NewFile(bytes.NewReader(data)); err == nil {
		files = append(files, arch{f, data})
	} else if looksMachO(data) {
		return append(found, "starts like a program but cannot be read as one")
	} else {
		return tidy(found)
	}
	for _, a := range files {
		found = append(found, machoInputs(path.Base(name), a.f, a.data)...)
	}
	return tidy(found)
}

// tidy sorts what was found and says each thing once, or is nil.
func tidy(found []string) []string {
	if len(found) == 0 {
		return nil
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// looksMachO says whether data starts with the magic number of a Mach-O
// program, thin or fat, 32 or 64 bit, in either byte order.
func looksMachO(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	switch binary.BigEndian.Uint32(data) {
	case 0xfeedface, 0xcefaedfe, 0xfeedfacf, 0xcffaedfe, 0xcafebabe, 0xbebafeca, 0xcafebabf, 0xbfbafeca:
		return true
	}
	return false
}

// machoInputs says what one program, or one architecture of it, imports,
// links or is entitled to that could read input or ask for administrator
// rights.
func machoInputs(base string, f *macho.File, data []byte) []string {
	var found []string
	syms, err := f.ImportedSymbols()
	if err != nil {
		found = append(found, fmt.Sprintf("its imports cannot be read: %v", err))
	}
	for _, s := range syms {
		bare := strings.TrimPrefix(strings.TrimPrefix(s, "_"), "OBJC_CLASS_$_")
		if slices.Contains(watching, bare) || slices.Contains(raising, bare) {
			found = append(found, "imports "+s)
		}
	}
	libs, err := f.ImportedLibraries()
	if err != nil {
		found = append(found, fmt.Sprintf("its libraries cannot be read: %v", err))
	}
	known, ok := linked[base]
	for _, l := range libs {
		if !ok {
			found = append(found, "is a program the list of linked libraries does not know, and links "+l)
		} else if !slices.Contains(known, l) {
			found = append(found, "links "+l)
		}
	}
	keys, err := entitlements(f, data)
	if err != nil {
		found = append(found, fmt.Sprintf("its signature cannot be read: %v", err))
	}
	for _, k := range keys {
		if !slices.Contains(entitled, k) {
			found = append(found, "is entitled to "+k)
		}
	}
	return found
}

// entitlements are the keys of what the program was signed as entitled
// to, from the code signature codesign writes into it, or none when it
// was signed without any or not at all. The signature is a blob of blobs,
// big-endian whatever the program is, and the entitlements are the XML
// one, slot 5.
func entitlements(f *macho.File, data []byte) ([]string, error) {
	const codeSignature = 0x1d
	be := binary.BigEndian
	for _, l := range f.Loads {
		raw := l.Raw()
		if len(raw) < 16 || f.ByteOrder.Uint32(raw) != codeSignature {
			continue
		}
		off, size := uint64(f.ByteOrder.Uint32(raw[8:])), uint64(f.ByteOrder.Uint32(raw[12:]))
		if off+size > uint64(len(data)) || size < 12 {
			return nil, errors.New("it reaches past the end of the file")
		}
		sig := data[off : off+size]
		if be.Uint32(sig) != 0xfade0cc0 {
			return nil, errors.New("it is no blob of blobs")
		}
		count := uint64(be.Uint32(sig[8:]))
		if 12+8*count > size {
			return nil, errors.New("its index reaches past its end")
		}
		for i := range count {
			kind, at := be.Uint32(sig[12+8*i:]), uint64(be.Uint32(sig[16+8*i:]))
			if kind != 5 {
				continue
			}
			if at+8 > size || be.Uint32(sig[at:]) != 0xfade7171 {
				return nil, errors.New("its entitlements are not where its index says")
			}
			end := at + uint64(be.Uint32(sig[at+4:]))
			if end > size || end < at+8 {
				return nil, errors.New("its entitlements reach past its end")
			}
			return plistKeys(sig[at+8 : end])
		}
	}
	return nil, nil
}

// plistKeys are the keys of the dictionary at the top of a property list
// in XML, which is how Info.plist and the entitlements are written.
// Comments are no keys.
func plistKeys(data []byte) ([]string, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	var keys []string
	depth := 0
	for {
		t, err := d.Token()
		if err == io.EOF {
			return keys, nil
		}
		if err != nil {
			return nil, err
		}
		switch e := t.(type) {
		case xml.StartElement:
			depth++
			// plist, then dict, then the dictionary's own keys.
			if e.Name.Local == "key" && depth == 3 {
				var k string
				if err := d.DecodeElement(&k, &e); err != nil {
					return nil, err
				}
				keys = append(keys, strings.TrimSpace(k))
				depth--
			}
		case xml.EndElement:
			depth--
		}
	}
}
