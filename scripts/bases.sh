#!/bin/sh
# Names the parts files of the builds a new build is patched against, one
# a line: every build the channel list names, and the newest builds that
# left it, BASES of them. A build leaves the list when its pull request is
# merged or a newer commit replaces it, and the app that runs it is then
# the one most likely to update next, to main after a merge above all. The
# publish workflow patches against these, see pack in
# cmd/speakertrail-release, and scripts/unreachable.sh keeps them and the
# pieces they name. See docs/UPDATES.md.
#
#   sh scripts/bases.sh channels.json assets.txt
#
# assets.txt is the release's files, "name createdAt" a line, as
#   gh release view dev --json assets --jq '.assets[] | "\(.name) \(.createdAt)"'
# writes them. It needs jq.
set -eu

# How many builds that left the list a new build is patched against. Each
# costs the publish a bsdiff of a few seconds for each file that changed,
# four at a time.
BASES=${BASES:-8}

list=$1
assets=$2

listed=$(jq -r '.channels[].url | split("/") | last | sub("\\.zip$"; ".parts.json")' "$list" 2>/dev/null || true)
{
	printf '%s\n' "$listed"
	grep -E '^app-[^ ]*\.parts\.json ' "$assets" | sort -k2 -r | awk '{ print $1 }' |
		grep -vxF -e "$listed" -e '' | head -n "$BASES" || true
} | awk 'NF && !seen[$0]++' | while read -r name; do
	# Only what the release holds.
	awk -v n="$name" '$1 == n { found = 1 } END { exit !found }' "$assets" && echo "$name"
done
