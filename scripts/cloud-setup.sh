#!/bin/bash
# Setup script for the Speaker Trail cloud environment at claude.ai/code.
# Paste this file's content into the environment's "Setup script" field. It
# runs as root on Ubuntu 24.04 before a session starts, and its result is
# cached.
#
# The base image already has Node.js 22, for building the Svelte
# interface. This script only:
# - installs Go 1.27, downloaded from the Go module proxy
# - installs GTK 4 and WebKitGTK 6, which the app's window compiles against
#   on Linux. The window cannot be opened here, but its code builds and its
#   tests run
# - installs Playwright's Chromium for the walks, unless the image has it
#
# A failed step never stops the session. Its log stays in /tmp, and running
# this script again inside a session finishes the job.

GO_VERSION=1.27.1
PLAYWRIGHT_VERSION=1.63.0

log() { echo "[speakertrail setup] $*"; }

install_go() {
	if /usr/local/go/bin/go version 2>/dev/null | grep -q "go$GO_VERSION "; then
		echo "Go $GO_VERSION is already there"
		return 0
	fi
	local name="v0.0.1-go$GO_VERSION.linux-amd64"
	local tmp
	tmp=$(mktemp -d)
	curl -fsSL --retry 3 -o "$tmp/go.zip" \
		"https://proxy.golang.org/golang.org/toolchain/@v/$name.zip" || return 1
	python3 -c 'import sys, zipfile; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])' \
		"$tmp/go.zip" "$tmp" || return 1
	local src="$tmp/golang.org/toolchain@$name"
	chmod +x "$src"/bin/* "$src"/pkg/tool/*/* || return 1
	rm -rf /usr/local/go
	mv "$src" /usr/local/go || return 1
	ln -sf /usr/local/go/bin/go /usr/local/bin/go
	ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
	rm -rf "$tmp"
	/usr/local/go/bin/go version
}

install_webview() {
	if pkg-config --exists gtk4 webkitgtk-6.0 2>/dev/null; then
		echo "GTK 4 and WebKitGTK 6 are already there"
		return 0
	fi
	local apt=(apt-get -o DPkg::Lock::Timeout=180 -o Acquire::Retries=3 -q)
	"${apt[@]}" update || echo "Some package sources could not be reached, continuing"
	DEBIAN_FRONTEND=noninteractive "${apt[@]}" install -y --no-install-recommends \
		pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
}

# The walks run in Playwright's Chromium. Many cloud images carry it
# already, then nothing is installed. The script runs before the
# repository is there, so the version is written here, the same as
# playwright in web/package.json.
install_chromium() {
	local dir=${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}
	if compgen -G "$dir/chromium-*" >/dev/null; then
		return 0
	fi
	npx --yes "playwright@$PLAYWRIGHT_VERSION" install --with-deps chromium
}

install_go >/tmp/speakertrail-setup-go.log 2>&1 &
go_job=$!
install_webview >/tmp/speakertrail-setup-webview.log 2>&1 &
webview_job=$!

if wait "$go_job"; then
	log "Go ready: $(/usr/local/go/bin/go version)"
else
	log "Go failed, see /tmp/speakertrail-setup-go.log:"
	tail -n 20 /tmp/speakertrail-setup-go.log
fi
if install_chromium >/tmp/speakertrail-setup-chromium.log 2>&1; then
	log "Chromium ready, for the walks"
else
	log "Chromium failed, see /tmp/speakertrail-setup-chromium.log:"
	tail -n 20 /tmp/speakertrail-setup-chromium.log
fi
if wait "$webview_job"; then
	log "GTK 4 and WebKitGTK 6 ready, for the app's window"
else
	log "GTK and WebKitGTK failed, see /tmp/speakertrail-setup-webview.log:"
	tail -n 20 /tmp/speakertrail-setup-webview.log
fi
exit 0
