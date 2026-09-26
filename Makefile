# Everything runs in Docker. The only thing this machine needs is Docker
# itself, https://www.docker.com/products/docker-desktop/
#
#   make run        build the app and start it with its database on
#                   http://localhost:8080. Ctrl-C stops it
#
# That is the whole of it for everyday work. The first make run writes .env
# with a login for this machine and prints the password.
#
# The rest, for when you want one part of it:
#
#   make            build the app
#   make crawl      check every due source once, like the nightly job
#   make people     who is on stage on 5 event pages the runs found, by the
#                   engine's rules and by the local model. Stores nothing.
#                   URL="https://… https://…" names the pages instead
#   make ui         the interface with live reload on http://localhost:5173,
#                   next to a running make run
#   make mock       the interface alone, with invented data and no backend
#   make test       all tests: unit and interface
#   make unit       the Go tests, under the race detector
#   make interface  the interface type check
#   make shell      a shell in the toolbox, with Go, Node.js and Chromium
#   make check      whether Docker is ready
#   make pull-prod  copy the production database into the local one
#   make image      build the production image, as the deployment does
#   make stop       stop everything make started
#   make clean      remove what make built. The local database stays
#
# Details are in docs/BUILD.md.

SHELL := /bin/sh
.DEFAULT_GOAL := all

# Where the work happens. On a laptop in Docker. On a build runner, in a
# Claude Code cloud session and inside the toolbox itself, directly, with
# the tools that are already there. DOCKER=0 forces the direct way.
DOCKER ?= $(if $(or $(CI),$(CLAUDE_CODE_REMOTE),$(SPEAKERTRAIL_IN_DOCKER)),0,1)

# The language model for make people. It runs on this machine, on the Mac
# through Docker Model Runner. Any model on hub.docker.com/u/ai works, for
# example MODEL=ai/gemma3:4b-q4_K_M for a faster, smaller one.
MODEL ?= ai/gemma3:12b-q4_K_M

.PHONY: all run crawl people model model-if-on ui mock test unit interface shell check db pull-prod image stop clean help docker

help:
	@sed -n '1,31p' Makefile | sed 's/^# \{0,1\}//'

ifeq ($(DOCKER),1)

# ---- On a laptop: Docker does the work -----------------------------------

COMPOSE := docker compose
TOOLBOX := $(COMPOSE) run --rm --build
STARTING_DATA := grep -v "0 added" | sed 's/^.*result="\(.*\)"$$/Starting data: \1/'

all: docker
	@$(COMPOSE) build web
	@echo "Ready: the app is built"

run: all .env model-if-on
	@$(COMPOSE) run --rm web import 2>&1 | $(STARTING_DATA) || true
	@echo "Open http://localhost:8080 and log in with the password in .env"
	@LLM_MODEL=$(MODEL) $(COMPOSE) up --attach web --no-log-prefix web

.env:
	@sh scripts/env.sh $(COMPOSE) run --rm --no-deps -T web

crawl: all model-if-on
	@LLM_MODEL=$(MODEL) $(COMPOSE) run --rm nightly

people: all model
	@$(COMPOSE) run --rm -e LLM_MODEL=$(MODEL) web people $(URL)

# The model, downloaded once. The first time takes a while, it is several GB.
model: docker
	@docker model version >/dev/null 2>&1 || { \
		echo "Docker Model Runner is off. Turn it on with: docker desktop enable model-runner"; exit 1; }
	@docker model pull $(MODEL)

# Runs read event pages with the model when Model Runner is on. Without it
# the app works the same and only reads no pages.
model-if-on: docker
	@if docker model version >/dev/null 2>&1; then docker model pull $(MODEL); else \
		echo "Docker Model Runner is off, so runs read no event pages for people."; \
		echo "Turn it on with: docker desktop enable model-runner"; fi

# The toolbox runs this same Makefile, which then works directly.
test unit interface: docker
	@$(TOOLBOX) dev make $@

ui mock: docker
	@$(TOOLBOX) --service-ports --no-deps dev make $@

shell: docker
	@$(TOOLBOX) dev bash

db: docker
	@$(COMPOSE) up -d --wait postgres

pull-prod: docker
	@$(COMPOSE) run --rm pgtools scripts/pull-prod.sh

# Also the one-off containers of make ui, make test and the like.
stop:
	@docker rm -f $$(docker ps -aq --filter label=com.docker.compose.project=speakertrail) >/dev/null 2>&1 || true
	@$(COMPOSE) --profile '*' down

clean:
	@rm -rf bin
	@find web/dist -mindepth 1 ! -name .keep -exec rm -rf {} + 2>/dev/null || true
	@$(MAKE) --no-print-directory stop >/dev/null
	@$(COMPOSE) --profile '*' down --rmi local >/dev/null 2>&1 || true
	@docker volume rm speakertrail_node_modules speakertrail_gomod speakertrail_gocache >/dev/null 2>&1 || true
	@echo "Removed the built app, the images and the caches. The local database stays"

check: docker
	@echo "Docker is ready"

docker:
	@command -v docker >/dev/null 2>&1 || { \
		echo "Docker is missing. It is the one thing to install:"; \
		echo "https://www.docker.com/products/docker-desktop/"; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "Docker is not running. Start Docker Desktop, then run make again."; exit 1; }

else

# ---- Directly: build runners, cloud sessions, inside the toolbox ---------

GO ?= go
NPM ?= npm
BIN := bin
GOTOOLCHAIN ?= local
export GOTOOLCHAIN

LOCAL_DATABASE := postgres://speakertrail:speakertrail@localhost:5432/speakertrail?sslmode=disable
DATABASE_URL ?= $(LOCAL_DATABASE)
TEST_DATABASE_URL ?= $(DATABASE_URL)
export DATABASE_URL TEST_DATABASE_URL

UI_SOURCES := $(shell find web/src web/public -type f 2>/dev/null) web/index.html web/package.json \
	web/vite.config.ts web/tsconfig.json
UI_BUILT := web/dist/index.html
GO_SOURCES := $(shell find cmd internal web -name '*.go' -o -name '*.sql' -o -name '*.csv' 2>/dev/null) go.mod go.sum

ENV := sh scripts/with-env.sh

all: toolchain $(BIN)/speakertrail
	@echo "Ready: $(BIN)/speakertrail"

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
	@echo "Open http://localhost:$${PORT:-8080} and log in with the password in .env"
	@$(ENV) $(BIN)/speakertrail serve

crawl: all db
	@$(ENV) $(BIN)/speakertrail nightly

# Directly, the model answers on http://localhost:12434, or at LLM_URL.
people: all db
	@LLM_MODEL=$(MODEL) $(ENV) $(BIN)/speakertrail people $(URL)

model:
	@echo "Directly, start the model yourself and set LLM_URL if it is not on localhost:12434"

# The toolbox's database is its own container, which compose starts.
db:
ifneq ($(SPEAKERTRAIL_IN_DOCKER),1)
	@sh scripts/db.sh
endif

# Live reload for the interface. It sends the API to the app from make run.
ui: web/node_modules/.package-lock.json
	@cd web && $(NPM) run dev

# The interface on its own, with invented data, for work on how it looks.
mock: web/node_modules/.package-lock.json
	@cd web && MOCK=1 $(NPM) run dev

test: unit interface

# The race detector is always on: the queue runs jobs on many goroutines,
# and a race there only shows on someone else's machine.
unit: db
	@$(GO) test -race -count=1 ./...

interface: web/node_modules/.package-lock.json
	@out=$$(cd web && $(NPM) run --silent check 2>&1) || { echo "$$out"; exit 1; }
	@printf 'ok  \tinterface types\n'

shell:
	@$${SHELL:-sh}

check:
	@sh scripts/check.sh || true

# Production data into the local database, one way only. Needs
# PROD_DATABASE_URL, the address from the GitHub secret DATABASE_URL.
pull-prod: db
	@sh scripts/pull-prod.sh

stop:
	@echo "Nothing to stop here: make run stops with Ctrl-C"

clean:
	@rm -rf $(BIN) web/node_modules
	@find web/dist -mindepth 1 ! -name .keep -exec rm -rf {} + 2>/dev/null || true
	@echo "Removed bin/, web/node_modules/ and the built interface"

endif

image:
	@docker build --platform linux/amd64 -t speakertrail:image .
