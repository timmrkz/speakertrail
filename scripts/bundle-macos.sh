#!/bin/sh
# Puts the built program into "Speaker Trail.app", the folder Finder draws
# as one icon.
#
#   scripts/bundle-macos.sh [BINDIR] [VERSION]
#
# This is what make run starts, so the app run every day has the same
# Info.plist, the same icon and the same name in the Dock as one opened
# from /Applications. FrameFairy's bundle is made the same way.
#
# Nothing here signs with a Developer ID. The ad hoc signature is enough to
# run on the Mac that built it, which is the only Mac it runs on.
set -e

BINDIR=${1:-bin}
VERSION=${2:-dev}
NAME="Speaker Trail"
EXE=speakertrail-app
ROOT=$(cd "$(dirname "$0")/.." && pwd)

if [ "$(uname -s 2>/dev/null)" != Darwin ]; then
	echo "bundle-macos.sh: a .app is a macOS thing and this is not a Mac." >&2
	exit 1
fi
if [ ! -x "$BINDIR/$EXE" ]; then
	echo "bundle-macos.sh: $BINDIR/$EXE is not built. Run make first." >&2
	exit 1
fi

APP="$BINDIR/$NAME.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BINDIR/$EXE" "$APP/Contents/MacOS/$EXE"

ICON=""
if [ -f "$ROOT/build/icon.png" ]; then
	ICNS="$ROOT/$BINDIR/.icon/icon.icns"
	sh "$ROOT/scripts/make-icon.sh" "$ICNS"
	cp "$ICNS" "$APP/Contents/Resources/icon.icns"
	ICON='	<key>CFBundleIconFile</key>
	<string>icon</string>'
fi

cat >"$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>$NAME</string>
	<key>CFBundleDisplayName</key>
	<string>$NAME</string>
	<key>CFBundleExecutable</key>
	<string>$EXE</string>
	<key>CFBundleIdentifier</key>
	<string>com.speakertrail.app</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleShortVersionString</key>
	<string>$VERSION</string>
	<key>CFBundleVersion</key>
	<string>$VERSION</string>
$ICON
	<key>LSMinimumSystemVersion</key>
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
</dict>
</plist>
PLIST
printf 'APPL????' >"$APP/Contents/PkgInfo"

# The copy can lose the signature, so it is made again, ad hoc. A signing
# that fails leaves an app macOS refuses to open with nothing to say why,
# so it stops the build here, and the bundle is read back as macOS will.
codesign --remove-signature "$APP/Contents/MacOS/$EXE" 2>/dev/null || true
codesign -s - -f "$APP/Contents/MacOS/$EXE"
codesign --verify --strict "$APP"

echo "Ready: $APP"
