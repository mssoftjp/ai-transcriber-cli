#!/bin/sh

set -eu

usage() {
	cat >&2 <<'EOF'
usage: scripts/build.sh --output <path>

Environment overrides:
  GO               Go binary to use (default: go)
  GOOS             Target OS
  GOARCH           Target architecture
  VERSION          Build version to embed
  COMMIT           Git commit to embed
  DATE             Build date (UTC RFC3339) to embed
EOF
	exit 1
}

output=""
while [ "$#" -gt 0 ]; do
	case "$1" in
	--output)
		[ "$#" -ge 2 ] || usage
		output="$2"
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

[ -n "$output" ] || usage

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
go_bin=${GO:-go}

cd "$repo_root"

module_path=$("$go_bin" list -m)
base_version=$(awk -F'"' '/Version = / { print $2; exit }' internal/buildinfo/buildinfo.go)
if [ -z "$base_version" ]; then
	base_version=0.3.0
fi

version=${VERSION:-}
commit=${COMMIT:-}
date_value=${DATE:-}

if [ -z "$version" ]; then
	if version_candidate=$(git describe --tags --exact-match 2>/dev/null); then
		version=$version_candidate
	else
		version="${base_version}-dev"
	fi
fi

if [ -z "$commit" ]; then
	if commit_candidate=$(git rev-parse --short=12 HEAD 2>/dev/null); then
		commit=$commit_candidate
	else
		commit=unknown
	fi
fi

if [ -z "$date_value" ]; then
	date_value=$(date -u +%Y-%m-%dT%H:%M:%SZ)
fi

ldflags="-X ${module_path}/internal/buildinfo.Version=${version} -X ${module_path}/internal/buildinfo.Commit=${commit} -X ${module_path}/internal/buildinfo.Date=${date_value}"

mkdir -p "$(dirname "$output")"
env GOOS="${GOOS:-}" GOARCH="${GOARCH:-}" \
	"$go_bin" build -ldflags "$ldflags" -o "$output" ./cmd/transcriber

printf '%s\n' "$output"
