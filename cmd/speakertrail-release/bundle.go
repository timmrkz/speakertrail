package main

// bundle puts the built program into "Speaker Trail.app", the folder
// Finder draws as one icon. It was scripts/bundle-macos.sh, and moved here
// with #57. make runs it on a Mac after carry-libs, so the app run every
// day has the same Info.plist, the same icon and the same name in the Dock
// as one opened from /Applications. FrameFairy's bundle is made the same
// way.
//
// Nothing here signs with a Developer ID. The ad hoc signature is enough
// to run on the Mac that built it. A build Tim updates to is signed again
// by publish.yml, with his Developer ID and the hardened runtime, see
// devid.go. Not here, because under the hardened runtime an app signed ad
// hoc may load no library but Apple's, and Dictate's speech library is not
// Apple's. Xcode's Sign to Run Locally leaves it off too.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// infoPlist is the app's Info.plist. %[1]s is the version, escaped, and
// %[2]s the icon's key, or nothing.
const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Speaker Trail</string>
	<key>CFBundleDisplayName</key>
	<string>Speaker Trail</string>
	<key>CFBundleExecutable</key>
	<string>speakertrail-app</string>
	<key>CFBundleIdentifier</key>
	<string>com.speakertrail.app</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleShortVersionString</key>
	<string>%[1]s</string>
	<key>CFBundleVersion</key>
	<string>%[1]s</string>
%[2]s	<key>LSMinimumSystemVersion</key>
	<string>13.0</string>
	<key>LSApplicationCategoryType</key>
	<string>public.app-category.productivity</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<!-- Full screen reaches the top of the display, as FrameFairy's does.
	     Without this macOS keeps a band of black across the top of a full
	     screen window, as wide as the whole display and 29 points tall,
	     which is the safe area it leaves clear of a MacBook's camera
	     housing. The window's own title bar is what ends up under the
	     housing, and it carries one word at the far left, so there is
	     nothing there for the housing to cover. -->
	<key>NSPrefersDisplaySafeAreaLayoutGuide</key>
	<false/>
	<!-- Asked the first time Dictate is pressed on Report a Problem.
	     Without it macOS ends an app that opens the microphone. -->
	<key>NSMicrophoneUsageDescription</key>
	<string>Speaker Trail writes down what you say about a problem, on this Mac, so you can send it in a report.</string>
</dict>
</plist>
`

// iconKey names the icon in the Info.plist.
const iconKey = "\t<key>CFBundleIconFile</key>\n\t<string>icon</string>\n"

// thePlist is the Info.plist of a build of version, with the icon or
// without.
func thePlist(version string, icon bool) []byte {
	var v bytes.Buffer
	_ = xml.EscapeText(&v, []byte(version))
	key := ""
	if icon {
		key = iconKey
	}
	return fmt.Appendf(nil, infoPlist, v.String(), key)
}

// bundler is what bundleApp asks of the Mac, so a test can be one.
type bundler struct {
	// run runs codesign.
	run func(name string, args ...string) ([]byte, error)
	// icon makes the .icns at out from build/icon.png.
	icon func(out string) error
	// gomod is the Go module cache, which the program must not look in.
	gomod string
}

// bundleApp makes bin/Speaker Trail.app of the program, llama-server and
// the speech library in bin, as version. root is the repository, where
// build/icon.png is.
func bundleApp(root, bin, version string, b bundler) (string, error) {
	exe := filepath.Join(bin, appProgram)
	if info, err := os.Stat(exe); err != nil || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%s is not built. Run make first", exe)
	}
	// llama-server, which runs the language model that drafts a first
	// message, beside the program, the only place internal/language
	// takes it from. A bundle without it is not made: Draft would have
	// nothing to run the model with, and Tim no terminal step to add it.
	// make builds it, see build-llama.sh.
	llama := filepath.Join(bin, "llama-server")
	if info, err := os.Stat(llama); err != nil || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("no llama-server in %s, and the app runs no other. Build it with make", bin)
	}

	app := filepath.Join(bin, appName)
	contents := filepath.Join(app, "Contents")
	if err := os.RemoveAll(app); err != nil {
		return "", err
	}
	for _, d := range []string{"MacOS", "Resources", "Frameworks"} {
		if err := os.MkdirAll(filepath.Join(contents, d), 0o755); err != nil {
			return "", err
		}
	}
	inApp := filepath.Join(contents, "MacOS", appProgram)
	if err := copyFile(exe, inApp, 0o755); err != nil {
		return "", err
	}
	if err := copyFile(llama, filepath.Join(contents, "MacOS", "llama-server"), 0o755); err != nil {
		return "", err
	}
	// Its licence goes in Resources, not beside it, because codesign
	// takes everything in Contents/MacOS for code. As in FrameFairy.
	if licence := filepath.Join(bin, "LICENSE-llama.cpp"); isFile(licence) {
		if err := copyFile(licence, filepath.Join(contents, "Resources", "LICENSE-llama.cpp"), 0o644); err != nil {
			return "", err
		}
	}

	// The speech library Dictate hears with. carry-libs has already
	// pointed the program at @executable_path/../Frameworks, which from
	// Contents/MacOS is this folder, so copying it here is the whole of
	// it. Never one with espeak-ng in it, which is GPL 3: carry-libs puts
	// sherpa-onnx's release without speech synthesis in bin/lib, and this
	// is the last place that can say no. As in FrameFairy.
	libs, err := filepath.Glob(filepath.Join(bin, "lib", "*.dylib"))
	if err != nil {
		return "", err
	}
	var gpl []string
	for _, lib := range libs {
		to := filepath.Join(contents, "Frameworks", filepath.Base(lib))
		if err := copyFile(lib, to, 0o755); err != nil {
			return "", err
		}
		data, err := os.ReadFile(to)
		if err != nil {
			return "", err
		}
		if bytes.Contains(data, []byte("espeak-ng")) {
			gpl = append(gpl, "  "+to)
		}
	}
	if gpl != nil {
		return "", fmt.Errorf("a speech library in the bundle carries espeak-ng:\n%s", strings.Join(gpl, "\n"))
	}

	icon := isFile(filepath.Join(root, "build", "icon.png"))
	if icon {
		icns := filepath.Join(bin, ".icon", "icon.icns")
		if err := b.icon(icns); err != nil {
			return "", err
		}
		if err := copyFile(icns, filepath.Join(contents, "Resources", "icon.icns"), 0o644); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), thePlist(version, icon), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(contents, "PkgInfo"), []byte("APPL????"), 0o644); err != nil {
		return "", err
	}

	// The copy can lose the signature, so it is made again, ad hoc. A
	// signing that fails leaves an app macOS refuses to open with nothing
	// to say why, so it stops the build here, and the bundle is read back
	// as macOS will.
	_, _ = b.run("codesign", "--remove-signature", inApp)
	// llama-server first, because signing the program seals the bundle
	// with what is in it then.
	for _, args := range [][]string{
		{"-s", "-", "-f", filepath.Join(contents, "MacOS", "llama-server")},
		{"-s", "-", "-f", inApp},
		{"--verify", "--strict", app},
	} {
		if _, err := b.run("codesign", args...); err != nil {
			return "", err
		}
	}

	// And read back: does every speech library the program needs resolve
	// inside the bundle, and does it still look in the Go module cache of
	// the machine that built it, which no other Mac has. carry-libs asks
	// it the same way of bin/.
	if err := verified(inApp, b.gomod); err != nil {
		return "", err
	}
	return app, nil
}

// makeIcon runs scripts/make-icon.sh, which stays a script: it only runs
// sips and iconutil, Tim decided on 28 November 2026.
func makeIcon(out string) error {
	cmd := exec.Command("/bin/sh", "scripts/make-icon.sh", out)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scripts/make-icon.sh: %w", err)
	}
	return nil
}

// bundleCommand is make's last step of a build on a Mac.
func bundleCommand(args []string, out io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: speakertrail-release bundle BINDIR VERSION")
	}
	if runtime.GOOS != "darwin" {
		return errors.New("a .app is a macOS thing and this is not a Mac")
	}
	version := args[1]
	if version == "" {
		version = "dev"
	}
	gomod, err := goModCache()
	if err != nil {
		return err
	}
	app, err := bundleApp(".", args[0], version, bundler{run: buildTools, icon: makeIcon, gomod: gomod})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Ready:", app)
	return nil
}
