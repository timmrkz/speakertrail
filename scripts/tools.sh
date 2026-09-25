#!/bin/sh
# Installs what this machine is missing to build and run Speaker Trail.
#
# make calls this on every build, so it costs nothing when nothing is
# missing: every check is a command -v or a file test, and Homebrew is only
# started when something is actually missing. On macOS it installs with
# Homebrew. Elsewhere it says what to install, because that needs
# administrator rights.
set -e
SYSTEM=$(uname -s 2>/dev/null)

if [ "$SYSTEM" != Darwin ]; then
	# Nothing to say when nothing is missing, because a line that is always
	# there is a line nobody reads.
	if ! command -v go >/dev/null 2>&1 || ! command -v npm >/dev/null 2>&1 || ! command -v psql >/dev/null 2>&1; then
		echo "On this system, install the tools as docs/BUILD.md describes."
		echo "make check shows what is missing."
	fi
	exit 0
fi

missing=""
want() { command -v "$1" >/dev/null 2>&1 || missing="$missing $2"; }
want go go
want npm node
# Postgres 16, the version Scaleway runs. Homebrew keeps it out of the
# search path, so look where it puts it.
if ! command -v pg_ctl >/dev/null 2>&1 && [ ! -x "$(brew --prefix 2>/dev/null)/opt/postgresql@16/bin/pg_ctl" ]; then
	missing="$missing postgresql@16"
fi

if [ -n "$missing" ]; then
	if ! command -v brew >/dev/null 2>&1; then
		echo "These are missing and Homebrew is not here to install them:$missing"
		echo "Homebrew is at https://brew.sh, or see docs/BUILD.md."
		exit 1
	fi
	for formula in $missing; do
		echo "Installing $formula"
		brew install "$formula"
	done
fi
