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
#                   any other app
#   make carry-over the people you kept in the old Docker version, into the
#                   app. Once, while Docker still has the old database
#   make mock       the interface alone in a browser, with invented data
#   make test       all tests: unit and interface
#   make unit       the Go tests, under the race detector
#   make interface  the interface type check
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

# The commit, for the log line each start writes.
VERSION := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

UI_SOURCES := $(shell find web/src web/public -type f 2>/dev/null) web/index.html web/package.json \
	web/vite.config.ts web/tsconfig.json
UI_BUILT := web/dist/index.html
GO_SOURCES := $(shell find cmd internal web -name '*.go' -o -name '*.csv' 2>/dev/null) go.mod go.sum

.PHONY: all app run install carry-over tools toolchain mock test unit interface check clean help

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
	@$(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o $@ ./cmd/speakertrail-app

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

# The people kept in the old Docker version, read from its database
# volume and added to the app's file. See scripts/carry-over.sh.
carry-over: toolchain
	@sh scripts/carry-over.sh

# The interface on its own, with invented data, for work on how it looks.
mock: web/node_modules/.package-lock.json
	@cd web && MOCK=1 $(NPM) run dev

test: unit interface

# The race detector is always on: runs ask several searches side by side,
# and a race there only shows on someone else's machine.
unit:
	@$(GO) test -race -count=1 ./...

interface: web/node_modules/.package-lock.json
	@out=$$(cd web && $(NPM) run --silent check 2>&1) || { echo "$$out"; exit 1; }
	@printf 'ok  \tinterface types\n'

check:
	@sh scripts/check.sh || true

clean:
	@rm -rf $(BIN) web/node_modules
	@find web/dist -mindepth 1 ! -name .keep -exec rm -rf {} + 2>/dev/null || true
	@echo "Removed bin/, web/node_modules/ and the built interface. The app's file stays"
