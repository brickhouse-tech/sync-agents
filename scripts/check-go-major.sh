#!/usr/bin/env bash
# Fails when go.mod's /vN suffix disagrees with the major version releases will carry.
# Go ignores v2+ tags unless the module path ends in /vN, so a mismatch ships
# releases that `go install` can never resolve (v2.0.0..v4.0.0 shipped that way).
#
# Usage: check-go-major.sh [git-range]
#   With a range (e.g. origin/main..HEAD), a breaking-change commit in it means the
#   next release bumps the major, so go.mod must already carry the next suffix.
set -euo pipefail
cd "$(dirname "$0")/.."

major=$(node -p 'require("./package.json").version.split(".")[0]')
if [[ -n "${1:-}" ]] && git log --format='%B' "$1" | grep -qE '^[a-z]+(\([^)]*\))?!:|^BREAKING[ -]CHANGE:'; then
  major=$((major + 1))
fi

want="github.com/brickhouse-tech/sync-agents"
if ((major >= 2)); then
  want="$want/v$major"
fi
have=$(awk '/^module /{print $2; exit}' go.mod)

if [[ "$have" != "$want" ]]; then
  echo "::error file=go.mod::module path is $have but the release major is v$major; expected $want (also rewrite internal imports, .goreleaser.yaml, scripts/lib/ldflags.js)" >&2
  exit 1
fi
echo "go.mod module path matches release major v$major"
