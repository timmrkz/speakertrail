#!/bin/sh
# Removes from the release dev what no entry of the channel list reaches:
# a build's zip and its zip packed with zstd, the parts of every build
# that is neither listed nor one of the newest that left the list, which
# the next build is patched against, see scripts/bases.sh, and every piece
# and patch that no remaining parts name. The publish workflow runs it in
# its list job, after channel-list.sh wrote the list. See
# .github/workflows/publish.yml and docs/UPDATES.md.
#
#   sh scripts/unreachable.sh channels.json
#
# It needs gh and jq, with GH_TOKEN and GH_REPO set.
#
# Nothing goes before it is an hour old: a publish that runs beside this
# may have uploaded files whose entry the list has not seen yet. A parts
# file that cannot be read keeps every piece and patch, since nothing then
# says which may go.
set -eu

list=$1
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

gh release view dev --json assets --jq '.assets[] | "\(.name) \(.createdAt)"' > "$work/assets.txt"

# The zips the list points at, and the packed zip beside each, by the
# names internal/updates gives them, PackedName and PartsName. And the
# parts of the builds the next build is patched against, which are the
# listed ones and the newest that left the list.
jq -r '.channels[].url | split("/") | last' "$list" > "$work/zips.txt"
: > "$work/kept.txt"
while read -r zip; do
	printf '%s\n%s.zst\n' "$zip" "$zip" >> "$work/kept.txt"
done < "$work/zips.txt"
sh "$(dirname "$0")/bases.sh" "$list" "$work/assets.txt" > "$work/parts.txt"
cat "$work/parts.txt" >> "$work/kept.txt"

# The pieces and patches every kept parts file names, PieceName and
# PatchName in internal/updates.
known=true
: > "$work/named.txt"
while read -r parts; do
	if ! gh release download dev --pattern "$parts" --dir "$work/parts" --clobber ||
		! jq -r '.parts[] | select(.sha256 != null) | . as $p
			| "piece-\($p.sha256).zst", (($p.patches // [])[] | "patch-\(.from)-\($p.sha256).bsdiff")' \
			"$work/parts/$parts" >> "$work/named.txt"; then
		echo "reading $parts failed, so every piece and patch stays"
		known=false
	fi
done < "$work/parts.txt"

now=$(date -u +%s)
while read -r name created; do
	case $name in
	app-*.zip | app-*.zip.zst | app-*.parts.json)
		grep -qxF "$name" "$work/kept.txt" && continue
		;;
	piece-*.zst | patch-*.bsdiff)
		[ "$known" = true ] || continue
		grep -qxF "$name" "$work/named.txt" && continue
		;;
	*) continue ;;
	esac
	age=$((now - $(date -u -d "$created" +%s)))
	if [ "$age" -gt 3600 ]; then
		echo "removing $name"
		gh release delete-asset dev "$name" -y || true
	fi
done < "$work/assets.txt"
