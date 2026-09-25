#!/bin/sh
# Copies the production database into the local one. Data only flows this
# way, never back. The copy holds real names, so it stays on this machine.
set -e
: "${PROD_DATABASE_URL:?Set PROD_DATABASE_URL to the production address, the GitHub secret DATABASE_URL}"
URL=${DATABASE_URL:-postgres://speakertrail:speakertrail@localhost:5432/speakertrail?sslmode=disable}
case "$URL" in
*@localhost:*|*@127.0.0.1:*) ;;
*) echo "DATABASE_URL is not local. This only ever writes into a local database."; exit 1 ;;
esac
dump=$(mktemp)
trap 'rm -f "$dump"' EXIT
echo "Copying production"
"$(sh scripts/pg.sh bin pg_dump)" "$PROD_DATABASE_URL" -Fc --no-owner --no-privileges -f "$dump"
echo "Restoring into the local database"
"$(sh scripts/pg.sh bin pg_restore)" --clean --if-exists --no-owner --no-privileges -d "$URL" "$dump"
echo "Done. The local database is a copy of production now."
