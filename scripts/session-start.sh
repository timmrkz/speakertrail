#!/bin/bash
# Starts the local Postgres in Claude Code cloud sessions, where the
# environment's setup script installed and configured it. Does nothing
# elsewhere.
[ "$CLAUDE_CODE_REMOTE" = "true" ] || exit 0

if command -v pg_ctlcluster >/dev/null && ! pg_isready -q -h localhost; then
	pg_ctlcluster 16 main start || echo "Postgres did not start" >&2
fi

if [ -n "$CLAUDE_ENV_FILE" ]; then
	url="postgres://speakertrail:speakertrail@localhost:5432/speakertrail?sslmode=disable"
	echo "export DATABASE_URL=\${DATABASE_URL:-$url}" >>"$CLAUDE_ENV_FILE"
	echo "export TEST_DATABASE_URL=\${TEST_DATABASE_URL:-$url}" >>"$CLAUDE_ENV_FILE"
fi
exit 0
