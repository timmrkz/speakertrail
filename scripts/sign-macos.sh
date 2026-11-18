#!/bin/sh
# Signs "Speaker Trail.app" again, with a Developer ID and Apple's hardened
# runtime, the way a build Tim updates to is signed.
#
#   SIGN_PASSWORD=... scripts/sign-macos.sh APP CERTIFICATE.p12 [IDENTITY]
#
# The certificate goes into a keychain of its own, made here and gone when
# the script ends, whatever happens, the way GitHub's own guide to signing
# on its Macs does it. IDENTITY is the certificate's name or the start of
# it, "Developer ID Application" unless said. publish.yml runs this on
# every build before it signs the zip with the update key, and CI runs it
# with a certificate made for the test, see docs/UPDATES.md.
#
# Builds made on the Mac with make stay signed ad hoc and without the
# hardened runtime, the way Xcode's Sign to Run Locally does, because under
# it an app signed ad hoc may load no library but Apple's, and Dictate's
# speech library is not Apple's.
#
# The hardened runtime is what keeps other programs from putting their own
# code into the app: macOS ignores the DYLD_ settings it is started with,
# refuses debuggers, and loads only libraries signed by Apple or by the
# same developer as the app. So every library is signed with the same
# Developer ID, first, and the app around them last. Inside out, as Apple
# asks, and as FrameFairy plans its signing in docs/PACKAGING.md.
#
# TIMESTAMP=none leaves out Apple's time stamp, for the test in CI. A real
# build has it, so its signature stays good after the certificate runs
# out.
set -e

APP=$1
P12=$2
IDENTITY=${3:-Developer ID Application}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
ENTITLEMENTS="$ROOT/build/entitlements.plist"

if [ "$(uname -s 2>/dev/null)" != Darwin ]; then
	echo "sign-macos.sh: codesign is a macOS thing and this is not a Mac." >&2
	exit 1
fi
if [ ! -x "$APP/Contents/MacOS/speakertrail-app" ] || [ ! -f "$P12" ]; then
	echo "usage: SIGN_PASSWORD=... scripts/sign-macos.sh APP CERTIFICATE.p12 [IDENTITY]" >&2
	exit 1
fi

DIR=$(mktemp -d)
KEYCHAIN="$DIR/signing.keychain-db"
PASS=$(openssl rand -hex 24)
BEFORE=$(security list-keychains -d user | tr -d '"')
finish() {
	# shellcheck disable=SC2086 # one path per word, as security lists them
	security list-keychains -d user -s $BEFORE 2>/dev/null || true
	security delete-keychain "$KEYCHAIN" 2>/dev/null || true
	rm -rf "$DIR"
}
trap finish EXIT

security create-keychain -p "$PASS" "$KEYCHAIN"
security set-keychain-settings -lut 1800 "$KEYCHAIN"
security unlock-keychain -p "$PASS" "$KEYCHAIN"
security import "$P12" -P "$SIGN_PASSWORD" -f pkcs12 -k "$KEYCHAIN" -T /usr/bin/codesign
# Apple's certificates between a Developer ID and Apple's root, which a
# certificate exported from Keychain Access does not carry. codesign
# needs the whole chain. They are public, and trust still comes from
# Apple's root in the system, so a wrong one could only make signing fail.
for ca in DeveloperIDG2CA DeveloperIDCA; do
	if curl -fsSL --retry 3 -o "$DIR/$ca.cer" "https://www.apple.com/certificateauthority/$ca.cer"; then
		security import "$DIR/$ca.cer" -k "$KEYCHAIN" 2>/dev/null || true
	fi
done
security set-key-partition-list -S apple-tool:,apple: -k "$PASS" "$KEYCHAIN" >/dev/null
# shellcheck disable=SC2086
security list-keychains -d user -s "$KEYCHAIN" $BEFORE

sign() {
	codesign --force --options runtime --timestamp${TIMESTAMP:+=$TIMESTAMP} \
		--keychain "$KEYCHAIN" -s "$IDENTITY" "$@"
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
