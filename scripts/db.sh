#!/bin/sh
# Makes sure the local database is running and set up, so make run and
# make test just work. Quiet when it already is.
#
# It starts PostgreSQL 16 the way this machine has it: Homebrew's service on
# macOS, the system cluster on Debian and Ubuntu. Then it creates the role
# and the database from DATABASE_URL, which defaults to the local one.
set -e
URL=${DATABASE_URL:-postgres://speakertrail:speakertrail@localhost:5432/speakertrail?sslmode=disable}
# A build runner brings its own database with its workflow.
if [ -n "$CI" ] && ! sh scripts/pg.sh bin psql >/dev/null 2>&1; then
	exit 0
fi
PSQL=$(sh scripts/pg.sh bin psql) || { echo "PostgreSQL is missing. make tools installs it on macOS, see docs/BUILD.md elsewhere."; exit 1; }
PGBIN=$(dirname "$PSQL")

reachable() { "$PSQL" "$URL" -Atqc 'SELECT 1' >/dev/null 2>&1; }
reachable && exit 0

case "$URL" in
*@localhost:*|*@127.0.0.1:*|*@localhost/*|*@127.0.0.1/*) ;;
*) echo "Cannot reach the database in DATABASE_URL, and it is not a local one make can start."; exit 1 ;;
esac

# The server, started if it is not running.
if ! "$PGBIN/pg_isready" -q -h localhost -p 5432; then
	echo "Starting PostgreSQL"
	if [ "$(uname -s)" = Darwin ] && command -v brew >/dev/null 2>&1; then
		brew services start postgresql@16 >/dev/null
	elif command -v pg_ctlcluster >/dev/null 2>&1; then
		if [ "$(id -u)" = 0 ]; then pg_ctlcluster 16 main start; else sudo pg_ctlcluster 16 main start; fi
	else
		echo "PostgreSQL is installed but not running, and make does not know how to start it here."
		echo "Start it, then run make again. See docs/BUILD.md."
		exit 1
	fi
	for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
		"$PGBIN/pg_isready" -q -h localhost -p 5432 && break
		sleep 1
	done
fi

# The role and the database, created as the server's own superuser: the
# user who installed it with Homebrew, or postgres on Linux.
user=$(echo "$URL" | sed -E 's#^postgres(ql)?://([^:@/]+).*#\2#')
pass=$(echo "$URL" | sed -nE 's#^postgres(ql)?://[^:@/]+:([^@]*)@.*#\2#p')
name=$(echo "$URL" | sed -E 's#^[^/]*//[^/]*/([^?]*).*#\1#')
sql="SELECT format('CREATE ROLE %I LOGIN CREATEDB PASSWORD %L', '$user', '$pass')
	WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '$user')\\gexec
SELECT format('CREATE DATABASE %I OWNER %I', '$name', '$user')
	WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$name')\\gexec"
if echo "$sql" | "$PSQL" -h localhost -d postgres -qv ON_ERROR_STOP=1 >/dev/null 2>&1; then
	:
elif [ "$(id -u)" = 0 ] && id postgres >/dev/null 2>&1; then
	echo "$sql" | runuser -u postgres -- "$PSQL" -qv ON_ERROR_STOP=1 >/dev/null
else
	echo "$sql" | sudo -u postgres "$PSQL" -qv ON_ERROR_STOP=1 >/dev/null
fi
echo "Database $name is ready"
reachable
