# One command does everything:
#
#   make run        install what is missing, build the app and open it
#
# That is the whole of it for everyday work. On a Mac make installs Go and
# Node.js with Homebrew when they are missing, builds the interface and the
# app, puts them together as "Speaker Trail.app" and starts it. No Docker,
# no database: the app keeps what it knows in one file in
# ~/Library/Application Support/Speaker Trail.
#
# The rest, for when you want one part of it:
#
#   make            build the app, bin/Speaker Trail.app on a Mac
#   make install    the same, copied to /Applications, to start it like
#                   any other app. Once is enough: from then on it updates
#                   itself, see docs/UPDATES.md
#   make update-key make the key builds are signed with, once, see
#                   docs/UPDATES.md
#   make carry-over the people you connected with in the old Docker
#                   version, into the app. Once, while Docker still has the
#                   old database
#   make mock       the interface alone in a browser, with invented data
#   make test       all tests: unit, interface and walks
#   make unit       the Go tests, under the race detector
#   make interface  the interface type check
#   make walks      the interface driven in Chromium, with invented data
#   make check      what this machine has and still needs
#   make clean      remove what make built
#
# Details are in docs/BUILD.md.

SHELL := /bin/sh
.DEFAULT_GOAL := all

GO ?= go
NPM ?= npm
BIN := bin
APP := $(BIN)/Speaker Trail.app
EXE := $(BIN)/speakertrail-app
UNAME := $(shell uname -s)
GOTOOLCHAIN ?= local
export GOTOOLCHAIN

# What the build workflow says about a build of the app: its version, the
# channel it came from and the commit. A build made here leaves them empty,
# and follows no channel until one is picked on the Updates page. See
# docs/UPDATES.md.
BUILD_VERSION ?=
BUILD_CHANNEL ?=
BUILD_COMMIT ?=
APP_LDFLAGS := -X main.buildVersion=$(BUILD_VERSION) -X main.buildChannel=$(BUILD_CHANNEL) -X main.buildCommit=$(BUILD_COMMIT)
# On a Mac every part of the app is built for the same oldest macOS, the
# one the bundle's Info.plist names. Without it clang builds the window's
# C for the macOS it runs on and Go links for 11.0, and the linker warns
# about every object file, as FrameFairy's build once did too.
MACOS_MIN := 13.0
ifeq ($(UNAME),Darwin)
export MACOSX_DEPLOYMENT_TARGET := $(MACOS_MIN)
export CGO_CFLAGS := -O2 -g -mmacosx-version-min=$(MACOS_MIN)
export CGO_LDFLAGS := -O2 -g -mmacosx-version-min=$(MACOS_MIN)
APP_LDFLAGS += -extldflags=-mmacosx-version-min=$(MACOS_MIN)
endif
# The app's version, in its Info.plist. The build's own, with its channel
# and commit, is BUILD_VERSION.
VERSION := $(shell awk -F'"' '/^const appVersion = /{ print $$2; exit }' cmd/speakertrail-app/appfiles.go)

UI_SOURCES := $(shell find web/src web/public -type f 2>/dev/null) web/index.html web/package.json \
	web/vite.config.ts web/tsconfig.json
UI_BUILT := web/dist/index.html
# The .txt is the update key's public half, built into the app, so a new
# key builds the app again.
GO_SOURCES := $(shell find cmd internal web -name '*.go' -o -name '*.csv' -o -name '*.txt' 2>/dev/null) go.mod go.sum

.PHONY: all app run install update-key carry-over tools toolchain mock test unit interface walks check clean help

help:
	@sed -n '1,25p' Makefile | sed 's/^# \{0,1\}//'

all: app

# What this machine is missing, installed. On a Mac that is Homebrew doing
# Go and Node.js. A build runner never installs: CI in the environment
# turns it off. Costs nothing when nothing is missing.
tools:
ifeq ($(CI),)
	@sh scripts/tools.sh
endif

toolchain: tools
	@QUIET=1 sh scripts/check.sh --toolchain

# The interface's packages, exactly as locked.
web/node_modules/.package-lock.json: web/package-lock.json
	@echo "Installing the interface packages"
	@out=$$(cd web && $(NPM) ci --no-audit --no-fund --loglevel=error 2>&1) || { echo "$$out"; exit 1; }

# The interface, built into web/dist/, which Go embeds. Not in the
# repository, only web/dist/.keep is.
$(UI_BUILT): $(UI_SOURCES) web/node_modules/.package-lock.json
	@echo "Building the interface"
	@cd web && $(NPM) run --silent build -- --logLevel warn

$(EXE): $(GO_SOURCES) $(UI_BUILT)
	@echo "Building the app"
	@$(GO) build -trimpath -ldflags "$(APP_LDFLAGS)" -o $@ ./cmd/speakertrail-app

# The app as the folder Finder draws as one icon. Only on a Mac: elsewhere
# the program in bin/ is the app.
app: toolchain $(EXE)
ifeq ($(UNAME),Darwin)
	@sh scripts/bundle-macos.sh $(BIN) "$(VERSION)"
else
	@echo "Ready: $(EXE)"
endif

# Started from inside the bundle rather than with open, so the log stays in
# this terminal. macOS still reads the bundle's Info.plist, because it finds
# the bundle by walking up from the program.
run: app
ifeq ($(UNAME),Darwin)
	@"$(APP)/Contents/MacOS/speakertrail-app"
else
	@$(EXE)
endif

install: app
ifeq ($(UNAME),Darwin)
	@rm -rf "/Applications/Speaker Trail.app"
	@ditto "$(APP)" "/Applications/Speaker Trail.app"
	@echo "Installed /Applications/Speaker Trail.app"
else
	@echo "make install puts the Mac app in /Applications, and this is not a Mac."
endif

# The people connected in the old Docker version, read from its database
# volume and added to the app's file. See scripts/carry-over.sh.
carry-over: toolchain
	@sh scripts/carry-over.sh

# The key builds are signed with, made once. It puts the private half on
# the clipboard, for GitHub's secrets and a password manager, and writes
# the public half into cmd/speakertrail-app/update-key.txt. See
# docs/UPDATES.md.
update-key: toolchain
	@$(GO) run ./cmd/speakertrail-release key

# The interface on its own, with invented data, for work on how it looks.
mock: web/node_modules/.package-lock.json
	@cd web && MOCK=1 $(NPM) run dev

test: unit interface walks

# The race detector is always on: runs ask several searches side by side,
# and a race there only shows on someone else's machine.
unit:
	@$(GO) test -race -count=1 ./...
	@sh scripts/needs-build-test.sh >/dev/null && printf 'ok  \tscripts/needs-build.sh\n'

interface: web/node_modules/.package-lock.json
	@out=$$(cd web && $(NPM) run --silent check 2>&1) || { echo "$$out"; exit 1; }
	@printf 'ok  \tinterface types\n'

# The walks drive the interface with invented data in Chromium, the way
# FrameFairy's walks drive its own, see web/walks/.
walks: web/node_modules/.package-lock.json
	@cd web && node walks/sidebar.mjs && node walks/person.mjs && node walks/titlebar.mjs

check:
	@sh scripts/check.sh || true

clean:
	@rm -rf $(BIN) web/node_modules
	@find web/dist -mindepth 1 ! -name .keep -exec rm -rf {} + 2>/dev/null || true
	@echo "Removed bin/, web/node_modules/ and the built interface. The app's file stays"
