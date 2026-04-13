#!/usr/bin/env bash
set -euo pipefail

version="${1:-}"
out="${2:-}"

if [ -z "$version" ] || [ -z "$out" ]; then
  echo "Usage: $0 <version> <output>" >&2
  exit 1
fi

{
  echo "# Release $version"
  echo ""
  if [ -f CHANGELOG.md ] && grep -q "^## $version" CHANGELOG.md; then
    awk "/^## $version/{flag=1;next}/^## /{flag=0}flag" CHANGELOG.md
  else
    echo "- Automated release build."
    echo "- See commit history for full details."
  fi
} >"$out"
