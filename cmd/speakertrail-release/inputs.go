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
	"path/filepath"
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
// the same way. The app is two programs: speakertrail-app, Wails' window,
// WebKit and the keychain through Security, and llama-server, which runs
// the language model that drafts a first message.
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
		// Linked weakly by Wails: ServiceManagement to open the app at
		// login with SMAppService's mainAppService, see raising, and
		// UniformTypeIdentifiers for the file types of its dialogs. Both
		// were in main's builds before the check read weak links.
		"/System/Library/Frameworks/ServiceManagement.framework/Versions/A/ServiceManagement",
		"/System/Library/Frameworks/UniformTypeIdentifiers.framework/Versions/A/UniformTypeIdentifiers",
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libobjc.A.dylib", "/usr/lib/libresolv.9.dylib",
		// The speech library Dictate hears with, carried in
		// Contents/Frameworks by the build, see docs/BUILD.md.
		"@rpath/libonnxruntime.dylib", "@rpath/libsherpa-onnx-c-api.dylib",
	},
	// llama-server, which runs the language model that drafts a first
	// message, built from llama.cpp by scripts/build-llama.sh: Metal for
	// the model, Accelerate for the cores, as FrameFairy lists it.
	"llama-server": {
		"/System/Library/Frameworks/Accelerate.framework/Versions/A/Accelerate",
		"/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation",
		"/System/Library/Frameworks/Foundation.framework/Versions/C/Foundation",
		"/System/Library/Frameworks/Metal.framework/Versions/A/Metal",
		"/System/Library/Frameworks/MetalKit.framework/Versions/A/MetalKit",
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libc++.1.dylib", "/usr/lib/libobjc.A.dylib",
	},
	// The speech library itself, sherpa-onnx's release without speech
	// synthesis, and the ONNX runtime under it, as FrameFairy lists them.
	"libonnxruntime.dylib": {
		"/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation",
		"/System/Library/Frameworks/CoreML.framework/Versions/A/CoreML",
		"/System/Library/Frameworks/Foundation.framework/Versions/C/Foundation",
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libc++.1.dylib", "/usr/lib/libiconv.2.dylib", "/usr/lib/libobjc.A.dylib",
	},
	"libsherpa-onnx-c-api.dylib": {
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libc++.1.dylib", "@rpath/libonnxruntime.dylib",
	},
	"libsherpa-onnx-cxx-api.dylib": {
		"/usr/lib/libSystem.B.dylib", "/usr/lib/libc++.1.dylib", "@rpath/libonnxruntime.dylib", "@rpath/libsherpa-onnx-c-api.dylib",
	},
}

// asks are the reasons Info.plist may give the person for what the app
// asks to reach: the microphone, for Dictate on Report a Problem, and
// nothing else. No folder, no camera, no screen. A reason added fails.
var asks = []string{"NSMicrophoneUsageDescription"}

// entitled are the entitlements a program may be signed with: the
// microphone, for Dictate, and nothing else. Above all not the camera,
// Apple events, or what lets other code into the app, library validation
// turned off, the dyld environment or get-task-allow. The microphone is
// one because publish.yml signs every build with the hardened runtime,
// see devid.go, and under it macOS hands an app that opens
// the microphone without it nothing but silence. It grants nothing:
// macOS still asks the person, in the words of
// NSMicrophoneUsageDescription. The list is FrameFairy's.
var entitled = []string{"com.apple.security.device.audio-input"}

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
		// Every file of a build is inside the .app. A name that climbs out
		// with ../, or starts at /, would land outside the folder it is
		// unpacked into, on the runner that signs it.
		if !insideApp(f.Name) {
			reads[i].found = append(reads[i].found, f.Name+": lies outside "+appName)
			continue
		}
		if f.Mode()&setID != 0 {
			reads[i].found = append(reads[i].found, f.Name+": runs as its owner, set-user-ID or set-group-ID")
		}
		// A link can point a folder of the build, Contents/Frameworks
		// say, at one outside it, and the updater keeps a link that does,
		// so every path the check reads as inside could lead out. No
		// build of the app has one.
		if f.Mode()&fs.ModeSymlink != 0 {
			reads[i].found = append(reads[i].found, f.Name+": is a link, which could lead outside the build")
			continue
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
	// On a Mac, CFBundle reads Info-macos.plist in place of Info.plist,
	// so a build has none of those, which nothing here would read.
	if base := strings.ToLower(path.Base(name)); strings.HasPrefix(base, "info-") && strings.HasSuffix(base, ".plist") {
		return tidy(append(found, "is an Info.plist macOS would read in place of Info.plist"))
	}
	// A Mac's disk does not tell capitals from small letters, so
	// Contents/info.plist is the app's Info.plist there, and in the
	// updater that unpacks the build.
	if strings.EqualFold(path.Base(name), "Info.plist") {
		keys, err := plistKeys(data)
		if err != nil {
			return append(found, fmt.Sprintf("cannot be read: %v", err))
		}
		for _, k := range keys {
			if strings.HasPrefix(k, "NS") && strings.HasSuffix(k, "UsageDescription") && !slices.Contains(asks, k) {
				found = append(found, "asks for "+k)
			}
		}
		// The app is always itself: macOS gives an app signed by Tim's team
		// the permissions and keychain items of whatever app it says it is.
		if strings.EqualFold(name, appName+"/Contents/Info.plist") {
			id, err := plistString(data, "CFBundleIdentifier")
			switch {
			case err != nil:
				found = append(found, fmt.Sprintf("its identifier cannot be read: %v", err))
			case id != bundleID:
				found = append(found, fmt.Sprintf("says it is %q, not %s", id, bundleID))
			}
		}
		return tidy(found)
	}
	files, err := archesOf(data)
	if err != nil {
		return append(found, err.Error())
	}
	for _, a := range files {
		found = append(found, machoInputs(name, a.f, a.data)...)
	}
	return tidy(found)
}

// arch is one architecture of a program with its own bytes: the offsets
// inside one, like where its signature is, count from the start of its
// slice.
type arch struct {
	f    *macho.File
	data []byte
}

// archesOf are the architectures of a Mach-O program, thin or fat, or
// none when the file is no program. A file that starts like one and
// cannot be read as one is an error, because what cannot be read cannot
// be checked.
func archesOf(data []byte) ([]arch, error) {
	if fat, err := macho.NewFatFile(bytes.NewReader(data)); err == nil {
		var files []arch
		for _, a := range fat.Arches {
			end := uint64(a.Offset) + uint64(a.Size)
			if end > uint64(len(data)) {
				return nil, fmt.Errorf("its %v part reaches past the end of the file", a.Cpu)
			}
			files = append(files, arch{a.File, data[a.Offset:end]})
		}
		return files, nil
	}
	if f, err := macho.NewFile(bytes.NewReader(data)); err == nil {
		return []arch{{f, data}}, nil
	}
	if looksMachO(data) {
		return nil, errors.New("starts like a program but cannot be read as one")
	}
	return nil, nil
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
// links, loads from outside the build or is entitled to that could read
// input or ask for administrator rights. name is where the program is in
// the build.
func machoInputs(name string, f *macho.File, data []byte) []string {
	base := path.Base(name)
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
	more, odd := otherLinks(f)
	libs = append(libs, more...)
	found = append(found, odd...)
	known, ok := linked[base]
	for _, l := range libs {
		if !ok {
			found = append(found, "is a program the list of linked libraries does not know, and links "+l)
		} else if !slices.Contains(known, l) {
			found = append(found, "links "+l)
		}
	}
	for _, l := range f.Loads {
		if r, ok := l.(*macho.Rpath); ok && !insideBuild(name, r.Path, f.Type == macho.TypeExec) {
			found = append(found, "looks for libraries outside the build, in "+r.Path)
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

// Load commands Go's debug/macho does not read for itself. Each of the
// first four links a library as LC_LOAD_DYLIB does, and loads it as
// surely: weakly, which Wails does for ServiceManagement and
// UniformTypeIdentifiers, re-exported, lazily or upward.
const (
	loadWeakDylib   = 0x80000018
	reexportDylib   = 0x8000001f
	lazyLoadDylib   = 0x20
	loadUpwardDylib = 0x80000023
	loadDylinker    = 0xe
	dyldEnvironment = 0x27
)

// otherLinks are the libraries a program links with the load commands Go
// does not read, and what else in its load commands could load code the
// check never read: a loader other than macOS's own, or an environment
// for it, which can set where it looks, like DYLD_LIBRARY_PATH.
func otherLinks(f *macho.File) (libs, found []string) {
	for _, l := range f.Loads {
		raw := l.Raw()
		cmd := f.ByteOrder.Uint32(raw)
		switch cmd {
		case loadWeakDylib, reexportDylib, lazyLoadDylib, loadUpwardDylib, loadDylinker, dyldEnvironment:
		default:
			continue
		}
		if len(raw) < 12 {
			found = append(found, fmt.Sprintf("its load command %#x cannot be read", cmd))
			continue
		}
		at := f.ByteOrder.Uint32(raw[8:])
		if at >= uint32(len(raw)) {
			found = append(found, fmt.Sprintf("its load command %#x cannot be read", cmd))
			continue
		}
		name, _, _ := bytes.Cut(raw[at:], []byte{0})
		switch cmd {
		case loadDylinker:
			if string(name) != "/usr/lib/dyld" {
				found = append(found, "is loaded by "+string(name))
			}
		case dyldEnvironment:
			found = append(found, "sets the loader's environment, "+string(name))
		default:
			libs = append(libs, string(name))
		}
	}
	return libs, found
}

// insideBuild says whether a place a program looks for the libraries it
// links as @rpath, an LC_RPATH, is inside the build. The list of linked
// libraries allows @rpath for the speech library, so whatever is found
// first under these places is what loads, and a place outside the build,
// like the Go module cache of the Mac that built it or a folder anyone
// can write to, would load a library nobody checked. Inside is where the
// file is, @loader_path, or where the program that runs is,
// @executable_path, and below them, never climbing out of the .app.
// exe says the file is a program of its own, whose @executable_path is
// its own folder. For a library it is the app's program, in
// Contents/MacOS.
//
// The app's builds look in @executable_path, @executable_path/lib and
// @executable_path/../Frameworks, and the speech library in @loader_path,
// read from main's build of 18 November 2026.
func insideBuild(name, rpath string, exe bool) bool {
	// The .app the file is in, or the top of the zip when it is in none,
	// which no rpath may climb out of either.
	bundle, at := "", path.Dir(name)
	if i := strings.Index(name, ".app/"); i >= 0 {
		bundle = name[:i+len(".app")]
		if !exe {
			at = bundle + "/Contents/MacOS"
		}
	}
	var rest string
	switch {
	case rpath == "@loader_path" || strings.HasPrefix(rpath, "@loader_path/"):
		at, rest = path.Dir(name), strings.TrimPrefix(rpath, "@loader_path")
	case rpath == "@executable_path" || strings.HasPrefix(rpath, "@executable_path/"):
		rest = strings.TrimPrefix(rpath, "@executable_path")
	default:
		return false
	}
	where := path.Clean(path.Join(at, rest))
	if bundle == "" {
		return where != ".." && !strings.HasPrefix(where, "../")
	}
	return where == bundle || strings.HasPrefix(where, bundle+"/")
}

// entitlements are the keys of what the program was signed as entitled
// to, from the code signature codesign writes into it, or none when it
// was signed without any or not at all. The entitlements are the XML
// blob, slot 5.
func entitlements(f *macho.File, data []byte) ([]string, error) {
	blob, err := signatureBlob(f, data, 5, 0xfade7171)
	if err != nil || blob == nil {
		return nil, err
	}
	return plistKeys(blob[8:])
}

// signatureBlob is the blob in one slot of the code signature codesign
// writes into a program, header and all, or nil when the program has no
// signature or nothing in that slot. The signature is a blob of blobs,
// big-endian whatever the program is.
func signatureBlob(f *macho.File, data []byte, slot, magic uint32) ([]byte, error) {
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
			if kind != slot {
				continue
			}
			if at+8 > size || be.Uint32(sig[at:]) != magic {
				return nil, fmt.Errorf("its slot %d is not where its index says", slot)
			}
			end := at + uint64(be.Uint32(sig[at+4:]))
			if end > size || end < at+8 {
				return nil, fmt.Errorf("its slot %d reaches past its end", slot)
			}
			return sig[at:end], nil
		}
	}
	return nil, nil
}

// bundleID is what the app is, to macOS, and what it is signed as.
const bundleID = "com.speakertrail.app"

// insideApp says whether a name in a build's zip is a path inside the
// .app, read the way a file system reads it.
func insideApp(name string) bool {
	clean := path.Clean(name)
	return filepath.IsLocal(clean) && !strings.Contains(name, "\\") &&
		(clean == appName || strings.HasPrefix(clean, appName+"/"))
}

// plistString is the string the dictionary at the top of a property list
// in XML holds under key, or "" when it holds none. A key that is there
// twice is an error: macOS reads one of the two, and which is not what a
// check should guess.
func plistString(data []byte, key string) (string, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = false
	depth, next, seen := 0, false, false
	value := ""
	for {
		t, err := d.Token()
		if err == io.EOF {
			return value, nil
		}
		if err != nil {
			return "", err
		}
		switch e := t.(type) {
		case xml.StartElement:
			depth++
			if depth != 3 {
				continue
			}
			var v string
			if err := d.DecodeElement(&v, &e); err != nil {
				return "", err
			}
			depth--
			if next {
				if e.Name.Local == "string" {
					value = strings.TrimSpace(v)
				}
				next = false
				continue
			}
			if e.Name.Local == "key" && strings.TrimSpace(v) == key {
				if seen {
					return "", fmt.Errorf("it holds %s twice", key)
				}
				seen, next = true, true
			}
		case xml.EndElement:
			depth--
		}
	}
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
