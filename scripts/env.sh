#!/bin/sh
# Writes .env with a login for the app on this machine, once, and prints
# the password the first time. .env is not in the repository.
#
#   env.sh COMMAND...   runs the app, which makes the password hash:
#                       bin/speakertrail, or the app in Docker
set -e
[ -f .env ] && exit 0
password=$(LC_ALL=C tr -dc 'a-km-z2-9' </dev/urandom | head -c 16)
secret=$(LC_ALL=C tr -dc 'a-f0-9' </dev/urandom | head -c 64)
hash=$(echo "$password" | "$@" hash-password 2>/dev/null)
case "$hash" in '$2'*) ;; *) echo "Could not make the password hash."; exit 1 ;; esac
cat >.env <<ENV
# The login for the app on this machine. Not in the repository.
# Delete this file and run make run again for a new password.
LOCAL_PASSWORD=$password
UI_PASSWORD_HASH='$hash'
SESSION_SECRET=$secret
ENV
echo "Wrote .env with a login for this machine. The password is: $password"
echo "It stays in .env as LOCAL_PASSWORD."
