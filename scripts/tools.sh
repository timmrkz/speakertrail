#!/bin/sh
# What this machine needs to build Speaker Trail, installed where it is
# missing: Go and Node.js. make calls this on every build, so it costs
# nothing when nothing is missing: every check is a command -v.
#
# On a Mac it uses Homebrew, the way FrameFairy's make does. Elsewhere it
# says what to install, because that needs administrator rights.
set -e

missing=""
command -v go >/dev/null 2>&1 || missing="$missing go"
command -v npm >/dev/null 2>&1 || missing="$missing node"
[ -n "$missing" ] || exit 0

if [ "$(uname -s 2>/dev/null)" != Darwin ]; then
	echo "Missing:$missing. make check says how to get them."
	exit 0
fi
if ! command -v brew >/dev/null 2>&1; then
	echo "These are missing and Homebrew is not here to install them:$missing"
	echo "Homebrew is at https://brew.sh"
	exit 1
fi
# The app's window needs the compiler from the command line tools.
xcode-select -p >/dev/null 2>&1 || {
	echo "Installing the command line tools"
	xcode-select --install
}
for formula in $missing; do
	echo "Installing $formula"
	brew install "$formula"
done
