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
#   make llama      the llama-server the app carries, built from llama.cpp.
#                   make does it once by itself, on a Mac
#   make install    the same, copied to /Applications, to start it like
#                   any other app. Once is enough: from then on it updates
#                   itself, see docs/UPDATES.md
#   make update-key make the key builds are signed with, once, see
#                   docs/UPDATES.md
#   make carry-over the people you connected with in the old Docker
#                   version, into the app. Once, while Docker still has the
#                   old database
#   make mock       the interface alone in a browser, with invented data
#   make test       all tests: lint, unit, interface and walks
#   make lint       gofmt, go vet and golangci-lint, see .golangci.yml
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
# What the app is built with. -s -w leaves out the symbol table and the
# debug information, about 5 MB of what an update downloads. Nobody reads
# them from a build: a panic still names every function and line, from the
# table Go keeps to run. -buildvcs=false leaves out the commit Go stamps in
# by itself, so a build whose code did not change has the same bytes as
# the one before and an update can tell what it already has. A build still
# says its commit, in buildCommit. See docs/UPDATES.md.
SHIP_FLAGS := -trimpath -buildvcs=false
APP_LDFLAGS := -s -w -X main.buildVersion=$(BUILD_VERSION) -X main.buildChannel=$(BUILD_CHANNEL) -X main.buildCommit=$(BUILD_COMMIT)
# On a Mac every part of the app is built for the same oldest macOS, the
# one the bundle's Info.plist names. Without it clang builds the window's
# C for the macOS it runs on and Go links for 11.0, and the linker warns
# about every object file, as FrameFairy's build once did too.
MACOS_MIN := 13.0
ifeq ($(UNAME),Darwin)
export MACOSX_DEPLOYMENT_TARGET := $(MACOS_MIN)
export CGO_CFLAGS := -O2 -g -mmacosx-version-min=$(MACOS_MIN)
# -headerpad_max_install_names leaves room in the program's header for the
# paths carry-libs.sh adds, where it looks for the speech library. Go
# leaves that room by itself only while it keeps the debug information,
# which the app no longer has, see APP_LDFLAGS. Without it
# install_name_tool has no room and the app cannot find its library, as
# FrameFairy found.
export CGO_LDFLAGS := -O2 -g -mmacosx-version-min=$(MACOS_MIN) -Wl,-headerpad_max_install_names
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

.PHONY: all app run install update-key carry-over tools toolchain llama mock test lint golangci-lint unit interface walks check clean help

help:
	@sed -n '1,33p' Makefile | sed 's/^# \{0,1\}//'

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

# On a Mac the speech library goes beside the program, and the program is
# pointed at it rather than at the Go module cache, which is where cgo
# points it and which exists on no Mac but the one that built it. A build
# from GitHub's Mac, picked on the Updates page, would not start without
# it. See scripts/carry-libs.sh and docs/BUILD.md.
$(EXE): $(GO_SOURCES) $(UI_BUILT)
	@echo "Building the app"
	@$(GO) build $(SHIP_FLAGS) -ldflags "$(APP_LDFLAGS)" -o $@ ./cmd/speakertrail-app
ifeq ($(UNAME),Darwin)
	@sh scripts/carry-libs.sh $@ $(BIN)/lib
endif

# The llama-server that runs the language model, built from llama.cpp at
# the version scripts/build-llama.sh pins. It takes a few minutes, so it
# is built once into .build/llama and again only when that script
# changed, which the stamp beside it says by the script's checksum. A
# date would not do: a fresh checkout makes the script newer than the
# server the build workflow keeps in its cache. The sources it was built
# from go once it is built, about a gigabyte.
LLAMA_DIR := .build/llama
LLAMA := $(LLAMA_DIR)/bin/llama-server
llama:
	@want=$$(cksum scripts/build-llama.sh | cut -d' ' -f1); \
	if [ -x $(LLAMA) ] && [ "$$(cat $(LLAMA_DIR)/built 2>/dev/null)" = "$$want" ]; then exit 0; fi; \
	echo "Building llama-server from llama.cpp. Once, and it takes a few minutes"; \
	rm -rf .build/llama && sh scripts/build-llama.sh .build/llama && rm -rf .build/llama/work && \
	echo "$$want" >$(LLAMA_DIR)/built

# The app as the folder Finder draws as one icon. Only on a Mac: elsewhere
# the program in bin/ is the app. llama-server and its licence go beside
# the program, where the app looks for it, see internal/language.
ifeq ($(UNAME),Darwin)
app: toolchain llama $(EXE)
	@cp $(LLAMA) $(LLAMA_DIR)/bin/LICENSE-llama.cpp $(BIN)/
	@sh scripts/bundle-macos.sh $(BIN) "$(VERSION)"
else
app: toolchain $(EXE)
	@echo "Ready: $(EXE)"
endif

# Started from inside the bundle rather than with open, so what the app
# says before its log is open stays in this terminal. macOS still reads the bundle's Info.plist, because it finds
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

test: lint unit interface walks

# Everything that reads the Go code without running it, the same here and
# in CI: gofmt, go vet and the rules in .golangci.yml.
GOLANGCI_LINT_VERSION := 2.14.0
GOLANGCI_LINT := $(BIN)/tools/golangci-lint-$(GOLANGCI_LINT_VERSION)/golangci-lint
lint: toolchain golangci-lint
	@out=$$(gofmt -l $$(git ls-files '*.go')); [ -z "$$out" ] || { echo "gofmt -w these:"; echo "$$out"; exit 1; }
	@printf 'ok  \tgofmt\n'
	@$(GO) vet ./...
	@printf 'ok  \tgo vet\n'
	@out=$$($(GOLANGCI_LINT) run ./... 2>&1) || { echo "$$out"; exit 1; }
	@printf 'ok  \tour code logs into the app log\n'

# The release golangci-lint builds and checks against its own checksums,
# as its install.sh does. Not install.sh itself, which asks github.com's
# release page for the tag, a page a cloud session cannot reach. Fetched
# into bin/, in CI too, the way Go fetches a module: it installs nothing
# on the machine. As in FrameFairy.
golangci-lint: $(GOLANGCI_LINT)
$(GOLANGCI_LINT):
	@echo "Fetching golangci-lint $(GOLANGCI_LINT_VERSION)"
	@name=golangci-lint-$(GOLANGCI_LINT_VERSION)-$$(uname -s | tr A-Z a-z)-$$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/'); \
		url=https://github.com/golangci/golangci-lint/releases/download/v$(GOLANGCI_LINT_VERSION); \
		tmp=$$(mktemp -d); \
		(cd $$tmp && curl -sSfLO $$url/$$name.tar.gz && curl -sSfLO $$url/golangci-lint-$(GOLANGCI_LINT_VERSION)-checksums.txt && \
			grep " $$name.tar.gz$$" golangci-lint-$(GOLANGCI_LINT_VERSION)-checksums.txt | shasum -a 256 -c - >/dev/null && \
			tar -xzf $$name.tar.gz) && mkdir -p $(@D) && mv $$tmp/$$name/golangci-lint $@; \
		status=$$?; rm -rf $$tmp; exit $$status

# The race detector is always on: runs ask several searches side by side,
# and a race there only shows on someone else's machine.
unit:
	@$(GO) test -race -count=1 ./...
	@sh scripts/needs-build-test.sh >/dev/null && printf 'ok  \tscripts/needs-build.sh\n'

interface: web/node_modules/.package-lock.json
	@out=$$(cd web && $(NPM) run --silent check 2>&1) || { echo "$$out"; exit 1; }
	@printf 'ok  \tinterface types\n'
	@out=$$(cd web && $(NPM) run --silent test 2>&1) || { echo "$$out"; exit 1; }
	@printf 'ok  \tinterface tests\n'

# The walks drive the interface with invented data in Chromium, the way
# FrameFairy's walks drive its own, see web/walks/.
walks: web/node_modules/.package-lock.json
	@cd web && node walks/sidebar.mjs && node walks/person.mjs && node walks/links.mjs && node walks/titlebar.mjs && node walks/logs.mjs && node walks/report.mjs && node walks/quit.mjs && node walks/speech.mjs && node walks/draft.mjs && node walks/template.mjs && node walks/focus.mjs && node walks/beam.mjs && node walks/pick.mjs && node walks/updates.mjs && node walks/look.mjs

check:
	@sh scripts/check.sh || true

clean:
	@rm -rf $(BIN) web/node_modules
	@find web/dist -mindepth 1 ! -name .keep -exec rm -rf {} + 2>/dev/null || true
	@echo "Removed bin/, web/node_modules/ and the built interface. The app's file stays"
