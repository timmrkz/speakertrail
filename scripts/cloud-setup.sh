#!/bin/bash
# Setup script for the Speaker Trail cloud environment at claude.ai/code.
# Paste this file's content into the environment's "Setup script" field. It
# runs as root on Ubuntu 24.04 before a session starts, and its result is
# cached.
#
# The base image already has what Speaker Trail needs besides Go:
# - PostgreSQL 16 server and client, the same major version as Scaleway
# - Node.js 22, for building the Svelte interface
# - Chromium under /opt/pw-browsers, for the headless browser
#
# So this script only:
# - installs Go 1.27, downloaded from the Go module proxy
# - puts Chromium on the PATH, where chromedp looks for it
# - creates a local database role and database for development and tests
#
# Postgres is not left running, because running processes are not cached.
# scripts/session-start.sh starts it in every session.
#
# A failed step never stops the session. Its log stays in /tmp, and running
# this script again inside a session finishes the job.

GO_VERSION=1.27.1
PG_VERSION=16
DB_NAME=speakertrail
DB_USER=speakertrail
DB_PASSWORD=speakertrail

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

link_chromium() {
	[ -x /opt/pw-browsers/chromium ] || return 1
	ln -sf /opt/pw-browsers/chromium /usr/local/bin/chromium
	chromium --version
}

setup_postgres() {
	command -v pg_ctlcluster >/dev/null || return 1
	pg_ctlcluster "$PG_VERSION" main start || return 1
	# CREATEDB lets tests create and drop their own throwaway databases.
	runuser -u postgres -- psql -v ON_ERROR_STOP=1 -q <<-SQL || { pg_ctlcluster "$PG_VERSION" main stop; return 1; }
		SELECT 'CREATE ROLE $DB_USER LOGIN CREATEDB PASSWORD ''$DB_PASSWORD'''
		WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '$DB_USER')\gexec
		SELECT 'CREATE DATABASE $DB_NAME OWNER $DB_USER'
		WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$DB_NAME')\gexec
	SQL
	pg_ctlcluster "$PG_VERSION" main stop
	echo "role and database $DB_NAME are ready"
}

install_go >/tmp/speakertrail-setup-go.log 2>&1 &
go_job=$!

if link_chromium >/tmp/speakertrail-setup-chromium.log 2>&1; then
	log "Chromium ready: $(tail -n 1 /tmp/speakertrail-setup-chromium.log)"
else
	log "Chromium not found under /opt/pw-browsers"
fi
if setup_postgres >/tmp/speakertrail-setup-postgres.log 2>&1; then
	log "Postgres $PG_VERSION ready"
else
	log "Postgres failed, see /tmp/speakertrail-setup-postgres.log:"
	tail -n 20 /tmp/speakertrail-setup-postgres.log
fi
if wait "$go_job"; then
	log "Go ready: $(/usr/local/go/bin/go version)"
else
	log "Go failed, see /tmp/speakertrail-setup-go.log:"
	tail -n 20 /tmp/speakertrail-setup-go.log
fi
exit 0
