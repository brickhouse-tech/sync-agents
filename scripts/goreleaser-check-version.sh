#!/bin/bash
# Goreleaser before-hook: verify package.json version matches the git tag.
set -euo pipefail

if [ -z "${1:-}" ]; then
  echo "usage: $0 <tag>" >&2
  exit 1
fi

TAG="$1"
PKG_VER=$(jq -r .version package.json)

if [ "v${PKG_VER}" != "${TAG}" ]; then
  echo "version drift: package.json=${PKG_VER} tag=${TAG}" >&2
  exit 1
fi

echo "version check: package.json=${PKG_VER} tag=${TAG} OK"
