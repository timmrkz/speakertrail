#!/bin/sh
# Signs "Speaker Trail.app" again, with a Developer ID and Apple's hardened
# runtime, the way a build Tim updates to is signed.
#
#   scripts/sign-macos.sh APP IDENTITY [KEYCHAIN]
#
# IDENTITY is the certificate's name or the start of it, "Developer ID
# Application" when the keychain holds only the one. publish.yml runs it
# on every build before it signs the zip with the update key, see
# docs/UPDATES.md. Builds made on the Mac with make stay signed ad hoc and
# without the hardened runtime, the way Xcode's Sign to Run Locally does,
# because under it an app signed ad hoc may load no library but Apple's,
# and Dictate's speech library is not Apple's.
#
# The hardened runtime is what keeps other programs from putting their own
# code into the app: macOS ignores the DYLD_ settings it is started with,
# refuses debuggers, and loads only libraries signed by Apple or by the
# same developer as the app. So every library is signed with the same
# Developer ID, first, and the app around them last. Inside out, as Apple
# asks, and as FrameFairy plans its signing in docs/PACKAGING.md.
#
# TIMESTAMP=none leaves out Apple's time stamp, for the test in CI, which
# signs with a certificate of its own. A real build has it, so its
# signature stays good after the certificate runs out.
set -e

APP=$1
IDENTITY=$2
KEYCHAIN=$3
EXE=speakertrail-app
ROOT=$(cd "$(dirname "$0")/.." && pwd)
ENTITLEMENTS="$ROOT/build/entitlements.plist"

if [ "$(uname -s 2>/dev/null)" != Darwin ]; then
	echo "sign-macos.sh: codesign is a macOS thing and this is not a Mac." >&2
	exit 1
fi
if [ ! -x "$APP/Contents/MacOS/$EXE" ] || [ -z "$IDENTITY" ]; then
	echo "usage: scripts/sign-macos.sh APP IDENTITY [KEYCHAIN]" >&2
	exit 1
fi

sign() {
	if [ -n "$KEYCHAIN" ]; then
		codesign --force --options runtime --timestamp${TIMESTAMP:+=$TIMESTAMP} \
			--keychain "$KEYCHAIN" -s "$IDENTITY" "$@"
	else
		codesign --force --options runtime --timestamp${TIMESTAMP:+=$TIMESTAMP} \
			-s "$IDENTITY" "$@"
	fi
}

for lib in "$APP"/Contents/Frameworks/*.dylib; do
	[ -e "$lib" ] || continue
	sign "$lib"
done
# Signing the bundle signs its program, with the entitlements, and seals
# everything else in it.
sign --entitlements "$ENTITLEMENTS" "$APP"

# Read back as macOS will read it. A signature that is wrong looks exactly
# like one that is right until the app will not open, or opens and hears
# only silence.
codesign --verify --strict --deep "$APP"
for code in "$APP" "$APP"/Contents/Frameworks/*.dylib; do
	[ -e "$code" ] || continue
	if ! codesign -d --verbose=2 "$code" 2>&1 | grep -q 'flags=0x10000(runtime)'; then
		echo "sign-macos.sh: $code is not signed with the hardened runtime:" >&2
		codesign -d --verbose=2 "$code" >&2 || true
		exit 1
	fi
done
if ! codesign -d --entitlements - "$APP" 2>/dev/null | grep -q 'com.apple.security.device.audio-input'; then
	echo "sign-macos.sh: the app is signed without the microphone, so Dictate would hear only silence." >&2
	exit 1
fi

codesign -d --verbose=2 "$APP" 2>&1 | grep -E '^(Authority|TeamIdentifier|Timestamp|CodeDirectory)' || true
echo "Signed: $APP"
