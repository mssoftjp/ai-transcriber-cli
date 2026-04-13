#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <version>"
  echo ""
  echo "Examples:"
  echo "  $0 v0.2.0"
  echo "  $0 v0.2.0-rc.1"
  exit 1
}

[ $# -eq 1 ] || usage
version="$1"

if ! echo "$version" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'; then
  echo "Error: version must match vX.Y.Z or vX.Y.Z-suffix"
  exit 1
fi

if echo "$version" | grep -qE -- '-(rc|alpha|beta|dev)'; then
  echo "Pre-release: $version"
  prerelease=true
else
  echo "Stable release: $version"
  prerelease=false
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "Error: working tree is not clean. Commit or stash changes first."
  exit 1
fi

if git rev-parse "$version" >/dev/null 2>&1; then
  echo "Error: tag $version already exists."
  exit 1
fi

if [ "$prerelease" = false ] && ! grep -q "$version" CHANGELOG.md 2>/dev/null; then
  echo ""
  echo "Warning: CHANGELOG.md does not mention $version."
  echo "Consider updating CHANGELOG.md before releasing."
  echo ""
  read -r -p "Continue anyway? [y/N] " confirm
  [ "$confirm" = "y" ] || [ "$confirm" = "Y" ] || exit 1
fi

echo ""
echo "Running tests..."
make test

echo ""
echo "Ready to tag and push $version"
echo "  Commit: $(git rev-parse --short HEAD)"
echo "  Branch: $(git branch --show-current)"
echo ""
read -r -p "Create tag and push? [y/N] " confirm
[ "$confirm" = "y" ] || [ "$confirm" = "Y" ] || exit 1

git tag "$version"
git push origin "$version"

echo ""
echo "Tag $version pushed. Release workflow will build and publish the release."
