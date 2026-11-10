#!/bin/sh
# Says what this machine has for building Speaker Trail, and how to get
# what is missing. It changes nothing.
#
#   check.sh              everything
#   check.sh --toolchain  only what building needs, stops on a problem
MIN_GO_MINOR=27
MIN_NODE=22
missing=0

ok() { [ "$QUIET" = 1 ] || printf '  ok       %s\n' "$1"; }
bad() { missing=1; printf '  missing  %s\n           %s\n' "$1" "$2"; }

if command -v go >/dev/null 2>&1; then
	version=$(go env GOVERSION | sed 's/^go//')
	minor=$(echo "$version" | cut -d. -f2)
	if [ "${minor:-0}" -lt "$MIN_GO_MINOR" ]; then
		bad "Go 1.$MIN_GO_MINOR or newer, found $version" "brew upgrade go"
	else
		ok "Go $version"
	fi
else
	bad "Go 1.$MIN_GO_MINOR or newer" "brew install go"
fi
if command -v node >/dev/null 2>&1; then
	major=$(node -p 'process.versions.node.split(".")[0]')
	if [ "$major" -lt "$MIN_NODE" ]; then
		bad "Node.js $MIN_NODE or newer, found $(node -v)" "brew upgrade node"
	else
		ok "Node.js $(node -v)"
	fi
else
	bad "Node.js $MIN_NODE or newer, for the interface" "brew install node"
fi
if [ "$(uname -s)" = Darwin ]; then
	if xcode-select -p >/dev/null 2>&1; then
		ok "the command line tools, for the app's window"
	else
		bad "the command line tools, for the app's window" "xcode-select --install"
	fi
elif ! pkg-config --exists gtk4 webkitgtk-6.0 2>/dev/null; then
	bad "GTK 4 and WebKitGTK 6, for the app's window" "sudo apt install libgtk-4-dev libwebkitgtk-6.0-dev"
else
	ok "GTK 4 and WebKitGTK 6"
fi

if [ "$1" = --toolchain ] && [ "$missing" = 1 ]; then
	exit 1
fi
[ "$1" = --toolchain ] || [ "$missing" = 1 ] || echo "Everything is here."
