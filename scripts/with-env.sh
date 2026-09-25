#!/bin/sh
# Runs a command with the settings from .env, when there is one. Values
# already set in the environment win, so the cloud session's own stay.
if [ -f .env ]; then
	while IFS= read -r line; do
		case "$line" in ''|'#'*) continue ;; esac
		key=${line%%=*}
		eval "current=\${$key-}"
		[ -n "$current" ] && continue
		value=${line#*=}
		value=${value#\'}
		value=${value%\'}
		export "$key=$value"
	done <.env
fi
exec "$@"
