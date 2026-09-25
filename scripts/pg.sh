#!/bin/sh
# Finds a PostgreSQL program, also where Homebrew keeps it out of the path.
#
#   pg.sh bin NAME     prints the full path of NAME, fails when there is none
[ "$1" = bin ] || { echo "usage: pg.sh bin NAME" >&2; exit 2; }
name=$2
command -v "$name" 2>/dev/null && exit 0
for dir in \
	"$(brew --prefix 2>/dev/null)/opt/postgresql@16/bin" \
	/usr/lib/postgresql/16/bin \
	/Applications/Postgres.app/Contents/Versions/16/bin; do
	[ -x "$dir/$name" ] && { echo "$dir/$name"; exit 0; }
done
exit 1
