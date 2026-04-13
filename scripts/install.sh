#!/bin/sh

set -eu

usage() {
	cat >&2 <<'EOF'
usage: scripts/install.sh --bindir <path>
EOF
	exit 1
}

bindir=""
while [ "$#" -gt 0 ]; do
	case "$1" in
	--bindir)
		[ "$#" -ge 2 ] || usage
		bindir="$2"
		shift 2
		;;
	-h|--help)
		usage
		;;
	*)
		echo "unknown argument: $1" >&2
		usage
		;;
	esac
done

[ -n "$bindir" ] || usage
mkdir -p "$bindir"

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)

"$repo_root/scripts/build.sh" --output "$bindir/transcriber"
