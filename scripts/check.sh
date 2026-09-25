#!/bin/sh
# Says what this machine has for Speaker Trail and how to get what is
# missing. It changes nothing.
#
#   check.sh              everything
#   check.sh --toolchain  only what building needs, stops on a problem
MIN_GO_MINOR=27
MIN_NODE=22
SYSTEM=$(uname -s 2>/dev/null)
missing=0

ok() { [ "$QUIET" = 1 ] || printf '  ok       %s\n' "$1"; }
bad() { missing=1; printf '  missing  %s\n           %s\n' "$1" "$2"; }
hint() { if [ "$SYSTEM" = Darwin ]; then echo "$1"; else echo "$2"; fi; }

toolchain() {
	if command -v go >/dev/null 2>&1; then
		version=$(go env GOVERSION | sed 's/^go//')
		minor=$(echo "$version" | cut -d. -f2)
		if [ "${minor:-0}" -lt "$MIN_GO_MINOR" ]; then
			bad "Go 1.$MIN_GO_MINOR or newer, found $version" "$(hint 'brew upgrade go' 'Go from https://go.dev/dl')"
		else
			ok "Go $version"
		fi
	else
		bad "Go 1.$MIN_GO_MINOR or newer" "$(hint 'brew install go' 'Go from https://go.dev/dl')"
	fi
	if command -v node >/dev/null 2>&1; then
		major=$(node -p 'process.versions.node.split(".")[0]')
		if [ "$major" -lt "$MIN_NODE" ]; then
			bad "Node.js $MIN_NODE or newer, found $(node -v)" "$(hint 'brew upgrade node' 'Node.js from https://nodejs.org')"
		else
			ok "Node.js $(node -v)"
		fi
	else
		bad "Node.js $MIN_NODE or newer, for the interface" "$(hint 'brew install node' 'Node.js from https://nodejs.org')"
	fi
}

runtime() {
	if sh scripts/pg.sh bin pg_ctl >/dev/null 2>&1; then
		ok "PostgreSQL $("$(sh scripts/pg.sh bin postgres)" --version | awk '{print $3}')"
	else
		bad "PostgreSQL 16" "$(hint 'brew install postgresql@16' 'sudo apt install postgresql-16')"
	fi
	if [ -n "$(chromium_path)" ]; then
		ok "a Chromium for pages that need JavaScript"
	else
		bad "Chrome or Chromium, only for pages that need JavaScript" "$(hint 'brew install --cask google-chrome' 'sudo apt install chromium')"
	fi
	if command -v docker >/dev/null 2>&1; then
		ok "Docker, for make image"
	else
		printf '  optional Docker, only to build the production image with make image\n'
	fi
}

chromium_path() {
	[ -n "$CHROME_PATH" ] && { echo "$CHROME_PATH"; return; }
	for n in headless-shell chromium chromium-browser google-chrome google-chrome-stable; do
		command -v "$n" 2>/dev/null && return
	done
	for p in "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" "/Applications/Chromium.app/Contents/MacOS/Chromium" /opt/pw-browsers/chromium; do
		[ -x "$p" ] && { echo "$p"; return; }
	done
}

case "$1" in
--toolchain)
	QUIET=1
	toolchain
	[ "$missing" = 0 ] || { echo "Install the above, then run make again."; exit 1; }
	;;
*)
	echo "Building"
	toolchain
	echo "Running"
	runtime
	[ "$missing" = 0 ] && echo "Everything is in place." || exit 1
	;;
esac
