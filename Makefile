# One command does everything:
#
#   make            install what is missing, build bin/speakertrail
#   make run        the same, then start the database and the app on
#                   http://localhost:8080
#
# That is the whole of it for everyday work. The first make run writes .env
# with a login for this machine and prints the password.
#
# The rest, for when you want one part of it:
#
#   make crawl      check every due source once, like the nightly job
#   make ui         the interface with live reload, next to a running make run
#   make mock       the interface alone, with invented data and no backend
#   make test       all tests: unit and interface
#   make unit       the Go tests, under the race detector
#   make interface  the interface type check
#   make check      what this machine has and what it still needs
#   make tools      install the missing tools and nothing else
#   make db         start the local database and set it up
#   make pull-prod  copy the production database into the local one
#   make image      build the production Docker image
#   make clean      remove what make built
#
# Details are in docs/BUILD.md.

SHELL := /bin/sh

GO ?= go
NPM ?= npm
BIN := bin
GOTOOLCHAIN ?= local
export GOTOOLCHAIN

# Whether make may install what this machine is missing. Off wherever CI is
# set: a build runner installs its own packages from its own workflow.
INSTALL ?= $(if $(CI),0,1)

UI_SOURCES := $(shell find web/src web/public -type f 2>/dev/null) web/index.html web/package.json \
	web/vite.config.ts web/tsconfig.json
UI_BUILT := web/dist/index.html
GO_SOURCES := $(shell find cmd internal web -name '*.go' -o -name '*.sql' -o -name '*.csv' 2>/dev/null) go.mod go.sum

ENV := sh scripts/with-env.sh

.PHONY: all run crawl ui mock test unit interface check tools deps toolchain db pull-prod image clean help

all: deps toolchain $(BIN)/speakertrail
	@echo "Ready: $(BIN)/speakertrail"

help:
	@sed -n '1,26p' Makefile | sed 's/^# \{0,1\}//'

deps:
ifeq ($(INSTALL),1)
	@sh scripts/tools.sh
endif

toolchain:
	@sh scripts/check.sh --toolchain

# The interface's packages, exactly as locked.
web/node_modules/.package-lock.json: web/package-lock.json
	@echo "Installing the interface packages"
	@out=$$(cd web && $(NPM) ci --no-audit --no-fund --loglevel=error 2>&1) || { echo "$$out"; exit 1; }

# The interface, built into web/dist/, which Go embeds. Not in the
# repository, only web/dist/.keep is.
$(UI_BUILT): $(UI_SOURCES) web/node_modules/.package-lock.json
	@echo "Building the interface"
	@cd web && $(NPM) run --silent build -- --logLevel warn

$(BIN)/speakertrail: $(GO_SOURCES) $(UI_BUILT)
	@echo "Building $@"
	@$(GO) build -trimpath -o $@ ./cmd/speakertrail

run: all db
	@sh scripts/env.sh $(BIN)/speakertrail
	@$(ENV) $(BIN)/speakertrail import 2>&1 | grep -v "0 added" | sed 's/^.*result="\(.*\)"$$/Starting data: \1/' || true
	@$(ENV) sh -c 'echo "Open http://localhost:$${PORT:-8080} and log in with the password in .env"'
	@$(ENV) $(BIN)/speakertrail serve

crawl: all db
	@sh scripts/env.sh $(BIN)/speakertrail
	@$(ENV) $(BIN)/speakertrail nightly

db:
	@$(ENV) sh scripts/db.sh

# Live reload for the interface. It sends the API to the app from make run,
# so run that in another terminal first.
ui: web/node_modules/.package-lock.json
	@cd web && $(NPM) run dev

# The interface on its own, with invented data, for work on how it looks.
mock: web/node_modules/.package-lock.json
	@cd web && MOCK=1 $(NPM) run dev

test: unit interface

# The race detector is always on: the queue runs jobs on many goroutines,
# and a race there only shows on someone else's machine.
unit: db
	@$(ENV) sh -c 'TEST_DATABASE_URL=$${TEST_DATABASE_URL:-$$DATABASE_URL} $(GO) test -race -count=1 ./...'

interface: web/node_modules/.package-lock.json
	@out=$$(cd web && $(NPM) run --silent check 2>&1) || { echo "$$out"; exit 1; }
	@printf 'ok  \tinterface types\n'

check:
	@sh scripts/check.sh || true

tools:
	@sh scripts/tools.sh

# Production data into the local database, one way only. Needs
# PROD_DATABASE_URL, the address from the GitHub secret DATABASE_URL.
pull-prod: db
	@$(ENV) sh scripts/pull-prod.sh

image:
	@docker build --platform linux/amd64 -t speakertrail:local .

clean:
	@rm -rf $(BIN) web/node_modules
	@find web/dist -mindepth 1 ! -name .keep -exec rm -rf {} + 2>/dev/null || true
	@echo "Removed bin/, web/node_modules/ and the built interface"
