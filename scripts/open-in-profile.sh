#!/bin/bash
# Opens a link in one browser profile on a Mac, the way the Mac app will.
# A proof of concept for docs/open-in-profile.md. It uses only what macOS
# ships with: open, osascript and bash. It installs nothing.
#
#   bash scripts/open-in-profile.sh
#       lists the browsers and their profiles
#   bash scripts/open-in-profile.sh "Google Chrome" "Tim" https://www.linkedin.com/in/...
#       opens the link in the Chrome profile named Tim
#   bash scripts/open-in-profile.sh Safari - https://www.linkedin.com/in/...
#       hands the link to Safari, whose own setting picks the profile
#   bash scripts/open-in-profile.sh Arc "Work" https://www.linkedin.com/in/...
#       opens the link in the Arc space named Work
set -euo pipefail

# Chromium browsers keep their profiles in a file called Local State,
# under ~/Library/Application Support/<folder>.
chromium_folder() {
	case "$1" in
	"Google Chrome") echo "Google/Chrome" ;;
	"Google Chrome Beta") echo "Google/Chrome Beta" ;;
	"Brave Browser") echo "BraveSoftware/Brave-Browser" ;;
	"Microsoft Edge") echo "Microsoft Edge" ;;
	"Vivaldi") echo "Vivaldi" ;;
	"Chromium") echo "Chromium" ;;
	*) return 1 ;;
	esac
}

# Prints one line per profile: the folder name Chrome wants, a tab, and the
# name the person gave the profile. Reads the JSON with JavaScript for
# Automation, which every Mac has.
chromium_profiles() {
	osascript -l JavaScript -e '
function run(argv) {
	ObjC.import("Foundation")
	const text = $.NSString.stringWithContentsOfFileEncodingError(argv[0], $.NSUTF8StringEncoding, null)
	if (!text || text.isNil()) return ""
	const cache = (JSON.parse(text.js).profile || {}).info_cache || {}
	return Object.keys(cache).sort().map(dir => dir + "\t" + (cache[dir].name || "")).join("\n")
}' "$1"
}

list() {
	local found=0
	for app in "Google Chrome" "Google Chrome Beta" "Brave Browser" "Microsoft Edge" "Vivaldi" "Chromium"; do
		local state="$HOME/Library/Application Support/$(chromium_folder "$app")/Local State"
		[ -f "$state" ] || continue
		found=1
		echo "$app"
		chromium_profiles "$state" | while IFS=$'\t' read -r dir name; do
			printf '  %-24s (folder %s)\n' "$name" "$dir"
		done
	done
	if [ -d "/Applications/Safari.app" ]; then
		found=1
		echo "Safari"
		echo "  picks the profile itself: Settings, Websites, Open Links With Profile"
	fi
	if [ -d "/Applications/Arc.app" ]; then
		found=1
		echo "Arc"
		osascript -e 'tell application "Arc" to get name of every space of front window' 2>/dev/null |
			tr ',' '\n' | sed 's/^ */  /' || echo "  (start Arc to see its spaces)"
	fi
	[ "$found" = 1 ] || echo "No browser with profiles found."
}

# A link from a crawled page is untrusted. Only a plain https link goes on,
# so that nothing can reach the browser as a command line switch.
check_link() {
	case "$1" in
	https://*) ;;
	*)
		echo "Only https links can be opened: $1" >&2
		exit 2
		;;
	esac
	case "$1" in
	*[[:space:]]*)
		echo "A link cannot contain spaces: $1" >&2
		exit 2
		;;
	esac
}

open_link() {
	local app="$1" profile="$2" link="$3"
	check_link "$link"

	if folder=$(chromium_folder "$app"); then
		local state="$HOME/Library/Application Support/$folder/Local State"
		[ -f "$state" ] || { echo "$app has no profiles on this Mac." >&2; exit 1; }
		# Match the name Tim sees first, then the folder name.
		local dir
		dir=$(chromium_profiles "$state" | awk -F'\t' -v p="$profile" '$2 == p { print $1; exit }')
		[ -n "$dir" ] || dir=$(chromium_profiles "$state" | awk -F'\t' -v p="$profile" '$1 == p { print $1; exit }')
		if [ -z "$dir" ]; then
			# Chrome would quietly create a new, logged out profile for an
			# unknown folder. Refuse instead.
			echo "$app has no profile called \"$profile\". These exist:" >&2
			chromium_profiles "$state" | cut -f2 | sed 's/^/  /' >&2
			exit 1
		fi
		# -n starts a second copy of the browser, which hands the link to the
		# one already running and quits. The running browser opens it as a tab
		# in that profile's last window, or a new window for that profile.
		open -n -a "$app" --args --profile-directory="$dir" "$link"
		echo "Opened in $app, profile \"$profile\" (folder $dir)."
		return
	fi

	case "$app" in
	Safari)
		open -a Safari "$link"
		echo "Handed to Safari. Its Open Links With Profile setting picks the profile."
		;;
	Arc)
		osascript - "$profile" "$link" <<-'APPLESCRIPT'
			on run argv
				set spaceName to item 1 of argv
				set theLink to item 2 of argv
				tell application "Arc"
					tell front window
						tell space spaceName
							focus
							make new tab with properties {URL:theLink}
						end tell
					end tell
					activate
				end tell
			end run
		APPLESCRIPT
		echo "Opened in Arc, space \"$profile\"."
		;;
	*)
		echo "Unknown browser: $app" >&2
		exit 1
		;;
	esac
}

if [ "$(uname)" != "Darwin" ]; then
	echo "This script runs on a Mac." >&2
	exit 1
fi

case $# in
0) list ;;
3) open_link "$1" "$2" "$3" ;;
*)
	sed -n '5,13p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
	;;
esac
