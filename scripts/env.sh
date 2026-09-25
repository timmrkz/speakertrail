#!/bin/sh
# Writes .env with what the app needs locally, once. It holds a login for
# the interface on this machine, which it prints the first time. .env is
# not in the repository.
#
#   env.sh BINARY     the built speakertrail, which makes the password hash
set -e
[ -f .env ] && exit 0
bin=$1
password=$(LC_ALL=C tr -dc 'a-km-z2-9' </dev/urandom | head -c 16)
secret=$(LC_ALL=C tr -dc 'a-f0-9' </dev/urandom | head -c 64)
hash=$(echo "$password" | "$bin" hash-password 2>/dev/null)
cat >.env <<ENV
# Local settings for make run. Not in the repository.
DATABASE_URL=postgres://speakertrail:speakertrail@localhost:5432/speakertrail?sslmode=disable
TEST_DATABASE_URL=postgres://speakertrail:speakertrail@localhost:5432/speakertrail?sslmode=disable
LOCAL_PASSWORD=$password
UI_PASSWORD_HASH='$hash'
SESSION_SECRET=$secret
PORT=8080
ENV
echo "Wrote .env with a login for this machine. The password is: $password"
echo "It stays in .env as LOCAL_PASSWORD."
