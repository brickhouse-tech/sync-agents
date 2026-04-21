#!/usr/bin/env bash
set -euo pipefail

# ── Fix command demo ──────────────────────────────────────────────────
# Clones the broken fixture to ./tmp/, runs sync-agents fix, and shows
# the before/after state. Reset with: git checkout examples/fix/fixture
# or just re-run this script (tmp/ is recreated each time).
# ──────────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SYNC_AGENTS="$REPO_ROOT/src/sh/sync-agents.sh"
FIXTURE="$SCRIPT_DIR/fixture"
TMP="$SCRIPT_DIR/tmp"

BOLD='\033[1m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[0;33m'
RESET='\033[0m'

banner() { echo -e "\n${BOLD}${CYAN}── $1 ──${RESET}\n"; }

# Clean previous run
rm -rf "$TMP"

# Clone fixture into tmp/ (with a git repo so sync-agents finds root)
banner "Cloning fixture to tmp/"
cp -R "$FIXTURE" "$TMP"
git init --quiet "$TMP"
echo -e "${GREEN}done${RESET}"

# Show the broken state
banner "BEFORE fix — broken layout"
echo -e "${YELLOW}Flat skill file (should be directory layout):${RESET}"
ls -l "$TMP/.agents/skills/"
echo ""
echo -e "${YELLOW}Legacy top-level skills/ directory (should be in .agents/):${RESET}"
ls -lR "$TMP/skills/"
echo ""
echo -e "${YELLOW}No target symlinks exist yet:${RESET}"
ls -la "$TMP/.claude/" 2>/dev/null || echo "  .claude/ does not exist"
ls -la "$TMP/.windsurf/" 2>/dev/null || echo "  .windsurf/ does not exist"

# Run fix
banner "Running: sync-agents fix skills"
bash "$SYNC_AGENTS" -d "$TMP" fix skills
echo ""

# Show the fixed state
banner "AFTER fix — correct layout"
echo -e "${GREEN}Flat skill converted to directory layout:${RESET}"
ls -lR "$TMP/.agents/skills/"
echo ""
echo -e "${GREEN}Legacy skills/ migrated and replaced with symlink:${RESET}"
ls -la "$TMP/skills" 2>/dev/null || echo "  (removed — was empty)"
echo ""
echo -e "${GREEN}Target symlinks created:${RESET}"
for target in .claude .windsurf .cursor ".github/copilot"; do
  if [ -L "$TMP/$target/skills" ]; then
    echo "  $target/skills -> $(readlink "$TMP/$target/skills")"
  fi
done
echo ""

# Run index to show AGENTS.md picks up the skill
bash "$SYNC_AGENTS" -d "$TMP" index >/dev/null 2>&1
echo -e "${GREEN}AGENTS.md now lists the skill correctly:${RESET}"
grep -A2 "## Skills" "$TMP/AGENTS.md"

banner "Demo complete"
echo "To reset: rm -rf $TMP"
echo "To reset fixture: git checkout examples/fix/fixture"
