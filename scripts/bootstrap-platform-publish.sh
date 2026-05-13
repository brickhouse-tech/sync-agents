#!/usr/bin/env bash
# Bootstrap-publishes the 5 platform sub-packages to npm using a classic
# NPM token. Required ONCE so that trusted-publisher configurations can be
# attached to each package on npmjs.org afterward — npm does not currently
# allow pre-registering trusted publishers for packages that have never
# been published (see https://github.com/npm/cli/issues/8544).
#
# After this script succeeds:
#   1. Configure Trusted Publishers for the main package + 5 sub-packages
#      on npmjs.org (Settings → Trusted Publishers on each package's page)
#   2. Delete the classic token used here
#   3. All future releases publish via the OIDC workflow with provenance
#
# Usage:
#   export NPM_TOKEN=npm_xxxxxxxxxxxx   # classic automation token
#   ./scripts/bootstrap-platform-publish.sh

set -eo pipefail
# Intentionally not `set -u` — macOS ships bash 3.2 where empty-array
# expansions like `"${ARR[@]}"` trip the unbound check even when the
# array was declared. Iteration on the result arrays would error.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Auth: prefer NPM_TOKEN env if set (CI / one-off override), otherwise rely on
# whatever the caller has in ~/.npmrc from a prior `npm login`.
if [[ -n "${NPM_TOKEN:-}" ]]; then
  NPMRC="$(mktemp)"
  trap 'rm -f "$NPMRC"' EXIT
  cat > "$NPMRC" <<EOF
//registry.npmjs.org/:_authToken=$NPM_TOKEN
registry=https://registry.npmjs.org/
EOF
  export NPM_CONFIG_USERCONFIG="$NPMRC"
  echo "Auth: using NPM_TOKEN env var"
else
  WHO=$(npm whoami 2>/dev/null || true)
  if [[ -z "$WHO" ]]; then
    echo "ERROR: not authenticated to npm. Run 'npm login' or 'export NPM_TOKEN=...'" >&2
    exit 1
  fi
  echo "Auth: using existing npm session (logged in as $WHO)"
fi
echo

ROOT_VERSION=$(node -p "require('./package.json').version")
echo "Root version: $ROOT_VERSION"
echo

PLATFORMS=(darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-x64)
# `set -u` treats an empty array reference as unbound in bash < 4.4; declare
# the arrays without an initial value list to avoid that quirk.
declare -a PUBLISHED SKIPPED FAILED

for triple in "${PLATFORMS[@]}"; do
  pkg="@brickhouse-tech/sync-agents-$triple"
  echo "===== $pkg ====="

  if npm view "$pkg@$ROOT_VERSION" version >/dev/null 2>&1; then
    echo "  already published at $ROOT_VERSION — skipping"
    SKIPPED+=("$pkg")
    echo
    continue
  fi

  if (cd "npm/$triple" && npm publish --access public); then
    PUBLISHED+=("$pkg")
  else
    FAILED+=("$pkg")
  fi
  echo
done

echo "===== SUMMARY ====="
printf "Published (%d):\n" "${#PUBLISHED[@]}"
for p in "${PUBLISHED[@]}"; do echo "  ✓ $p"; done
printf "Skipped   (%d):\n" "${#SKIPPED[@]}"
for p in "${SKIPPED[@]}"; do echo "  - $p"; done
printf "Failed    (%d):\n" "${#FAILED[@]}"
for p in "${FAILED[@]}"; do echo "  ✗ $p"; done

if (( ${#FAILED[@]} > 0 )); then
  exit 1
fi

cat <<'EOM'

Next steps:
  1. Configure Trusted Publisher on each of these 6 package pages:
       https://www.npmjs.com/package/@brickhouse-tech/sync-agents/access
       https://www.npmjs.com/package/@brickhouse-tech/sync-agents-darwin-arm64/access
       https://www.npmjs.com/package/@brickhouse-tech/sync-agents-darwin-x64/access
       https://www.npmjs.com/package/@brickhouse-tech/sync-agents-linux-arm64/access
       https://www.npmjs.com/package/@brickhouse-tech/sync-agents-linux-x64/access
       https://www.npmjs.com/package/@brickhouse-tech/sync-agents-win32-x64/access
     For each: Repo = brickhouse-tech/sync-agents | Workflow = publish.yml | Env = (blank)
  2. Delete the classic NPM token you used for this bootstrap.
  3. Tag v0.2.0 (or merge a release PR) — the publish workflow will run via OIDC.
EOM
