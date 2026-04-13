#!/bin/sh

set -eu

usage() {
	cat >&2 <<'EOF'
usage: scripts/package.sh --version <vX.Y.Z> --output-dir <dir>

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

version=""
output_dir=""

while [ "$#" -gt 0 ]; do
	case "$1" in
	--version)
		[ "$#" -ge 2 ] || usage
		version="$2"
		shift 2
		;;
	--output-dir)
		[ "$#" -ge 2 ] || usage
		output_dir="$2"
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

[ -n "$version" ] || usage
[ -n "$output_dir" ] || usage

script_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
goos=${GOOS:-$(go env GOOS)}
goarch=${GOARCH:-$(go env GOARCH)}
bin_name="transcriber_${version}_${goos}_${goarch}"
if [ "$goos" = "windows" ]; then
	bin_name="${bin_name}.exe"
	archive_ext=".zip"
else
	archive_ext=".tar.gz"
fi
bin_path="$output_dir/$bin_name"
archive_path="$output_dir/transcriber_${version}_${goos}_${goarch}${archive_ext}"
checksums_path="$output_dir/checksums.txt"

mkdir -p "$output_dir"

GO="${GO:-go}" GOOS="$goos" GOARCH="$goarch" VERSION="$version" COMMIT="${COMMIT:-}" DATE="${DATE:-}" \
	"$repo_root/scripts/build.sh" --output "$bin_path" >/dev/null

case "$archive_ext" in
	.zip)
	(
		cd "$output_dir"
		zip -q "$(basename "$archive_path")" "$bin_name"
	)
	;;
	.tar.gz)
		tar -C "$output_dir" -czf "$archive_path" "$bin_name"
	;;
esac

(
	cd "$output_dir"
	shasum -a 256 "$(basename "$archive_path")" >> "$(basename "$checksums_path")"
)

printf '%s\n' "$archive_path"
