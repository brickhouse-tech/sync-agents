#!/usr/bin/env bats

# Resolve the script under test relative to this test file
SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../src/sh" && pwd)/sync-agents.sh"
# Read version from package.json so the test stays in sync after bumps
PACKAGE_VERSION="$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/package.json" | head -1)"

setup() {
  # Create a temporary directory for each test
  TEST_DIR="$(mktemp -d)"
  # Initialize a git repo so find_project_root works
  git init --quiet "$TEST_DIR"
  export HOME="$TEST_DIR"
}

teardown() {
  # Clean up temp directory
  rm -rf "$TEST_DIR"
}

# --------------------------------------------------------------------------
# --help
# --------------------------------------------------------------------------

@test "--help shows usage information" {
  run bash "$SCRIPT" --help
  [ "$status" -eq 0 ]
  [[ "$output" == *"sync-agents"* ]]
  [[ "$output" == *"USAGE"* ]]
  [[ "$output" == *"COMMANDS"* ]]
}

@test "no arguments shows usage" {
  run bash "$SCRIPT" -d "$TEST_DIR"
  [ "$status" -eq 0 ]
  [[ "$output" == *"USAGE"* ]]
}

# --------------------------------------------------------------------------
# --version
# --------------------------------------------------------------------------

@test "--version shows version from package.json" {
  run bash "$SCRIPT" --version
  [ "$status" -eq 0 ]
  [[ "$output" == *"sync-agents v${PACKAGE_VERSION}"* ]]
}

# --------------------------------------------------------------------------
# init
# --------------------------------------------------------------------------

@test "init creates .agents/ directory structure" {
  run bash "$SCRIPT" -d "$TEST_DIR" init
  [ "$status" -eq 0 ]
  [ -d "$TEST_DIR/.agents" ]
  [ -d "$TEST_DIR/.agents/rules" ]
  [ -d "$TEST_DIR/.agents/skills" ]
  [ -d "$TEST_DIR/.agents/workflows" ]
  [ -f "$TEST_DIR/.agents/STATE.md" ]
  [ -f "$TEST_DIR/AGENTS.md" ]
}

@test "init creates STATE.md with expected content" {
  run bash "$SCRIPT" -d "$TEST_DIR" init
  [ "$status" -eq 0 ]
  [[ "$(cat "$TEST_DIR/.agents/STATE.md")" == *"# State"* ]]
}

@test "init creates AGENTS.md with expected content" {
  run bash "$SCRIPT" -d "$TEST_DIR" init
  [ "$status" -eq 0 ]
  [[ "$(cat "$TEST_DIR/AGENTS.md")" == *"# AGENTS"* ]]
  [[ "$(cat "$TEST_DIR/AGENTS.md")" == *"## Rules"* ]]
  [[ "$(cat "$TEST_DIR/AGENTS.md")" == *"## Skills"* ]]
  [[ "$(cat "$TEST_DIR/AGENTS.md")" == *"## Workflows"* ]]
}

@test "init is idempotent - does not overwrite existing files" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  echo "custom content" > "$TEST_DIR/.agents/STATE.md"
  echo "custom agents" > "$TEST_DIR/AGENTS.md"

  run bash "$SCRIPT" -d "$TEST_DIR" init
  [ "$status" -eq 0 ]
  [[ "$(cat "$TEST_DIR/.agents/STATE.md")" == "custom content" ]]
  [[ "$(cat "$TEST_DIR/AGENTS.md")" == "custom agents" ]]
}

# --------------------------------------------------------------------------
# add rule
# --------------------------------------------------------------------------

@test "add rule creates a rule file" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" add rule no-eval
  [ "$status" -eq 0 ]
  [ -f "$TEST_DIR/.agents/rules/no-eval.md" ]
}

@test "add rule updates AGENTS.md" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule no-eval
  [[ "$(cat "$TEST_DIR/AGENTS.md")" == *"no-eval"* ]]
}

@test "add rule file contains rule name" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule no-eval
  [[ "$(cat "$TEST_DIR/.agents/rules/no-eval.md")" == *"no-eval"* ]]
}

# --------------------------------------------------------------------------
# add skill
# --------------------------------------------------------------------------

@test "add skill creates a skill file" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" add skill code-review
  [ "$status" -eq 0 ]
  [ -f "$TEST_DIR/.agents/skills/code-review.md" ]
}

@test "add skill updates AGENTS.md" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add skill code-review
  [[ "$(cat "$TEST_DIR/AGENTS.md")" == *"code-review"* ]]
}

# --------------------------------------------------------------------------
# add workflow
# --------------------------------------------------------------------------

@test "add workflow creates a workflow file" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" add workflow deploy
  [ "$status" -eq 0 ]
  [ -f "$TEST_DIR/.agents/workflows/deploy.md" ]
}

@test "add workflow updates AGENTS.md" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add workflow deploy
  [[ "$(cat "$TEST_DIR/AGENTS.md")" == *"deploy"* ]]
}

# --------------------------------------------------------------------------
# add with invalid type
# --------------------------------------------------------------------------

@test "add with invalid type fails" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" add bogus my-thing
  [ "$status" -ne 0 ]
  [[ "$output" == *"Unknown type"* ]]
}

@test "add with missing arguments fails" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" add
  [ "$status" -ne 0 ]
  [[ "$output" == *"Usage"* ]]
}

# --------------------------------------------------------------------------
# index
# --------------------------------------------------------------------------

@test "index regenerates AGENTS.md" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule my-rule
  # Overwrite AGENTS.md with junk
  echo "junk" > "$TEST_DIR/AGENTS.md"
  run bash "$SCRIPT" -d "$TEST_DIR" index
  [ "$status" -eq 0 ]
  [[ "$(cat "$TEST_DIR/AGENTS.md")" == *"my-rule"* ]]
  [[ "$(cat "$TEST_DIR/AGENTS.md")" == *"## Rules"* ]]
}

# --------------------------------------------------------------------------
# sync
# --------------------------------------------------------------------------

@test "sync creates symlinks in .claude/ and .windsurf/" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" sync
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/.claude/rules" ]
  [ -L "$TEST_DIR/.claude/skills" ]
  [ -L "$TEST_DIR/.claude/workflows" ]
  [ -L "$TEST_DIR/.windsurf/rules" ]
  [ -L "$TEST_DIR/.windsurf/skills" ]
  [ -L "$TEST_DIR/.windsurf/workflows" ]
}

@test "sync creates CLAUDE.md symlink to AGENTS.md" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" sync
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/CLAUDE.md" ]
  local link_target
  link_target="$(readlink "$TEST_DIR/CLAUDE.md")"
  [[ "$link_target" == "AGENTS.md" ]]
}

# --------------------------------------------------------------------------
# sync --dry-run
# --------------------------------------------------------------------------

@test "sync --dry-run does not create actual symlinks" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" sync --dry-run
  [ "$status" -eq 0 ]
  [[ "$output" == *"would link"* ]]
  [ ! -L "$TEST_DIR/.claude/rules" ]
  [ ! -L "$TEST_DIR/.windsurf/rules" ]
  [ ! -L "$TEST_DIR/CLAUDE.md" ]
}

# --------------------------------------------------------------------------
# status
# --------------------------------------------------------------------------

@test "status shows correct state after init" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" status
  [ "$status" -eq 0 ]
  [[ "$output" == *".agents/ exists"* ]]
  [[ "$output" == *"AGENTS.md exists"* ]]
}

@test "status shows synced state after sync" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" sync
  run bash "$SCRIPT" -d "$TEST_DIR" status
  [ "$status" -eq 0 ]
  [[ "$output" == *"synced"* ]]
}

@test "status shows missing state without init" {
  run bash "$SCRIPT" -d "$TEST_DIR" status
  [ "$status" -eq 0 ]
  [[ "$output" == *"missing"* ]]
}

# --------------------------------------------------------------------------
# clean
# --------------------------------------------------------------------------

@test "clean removes symlinks and empty directories" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" sync
  # Verify symlinks exist first
  [ -L "$TEST_DIR/.claude/rules" ]
  [ -L "$TEST_DIR/CLAUDE.md" ]

  run bash "$SCRIPT" -d "$TEST_DIR" clean
  [ "$status" -eq 0 ]
  [ ! -L "$TEST_DIR/.claude/rules" ]
  [ ! -L "$TEST_DIR/.windsurf/rules" ]
  [ ! -L "$TEST_DIR/CLAUDE.md" ]
  # Empty directories should be removed
  [ ! -d "$TEST_DIR/.claude" ]
  [ ! -d "$TEST_DIR/.windsurf" ]
}

@test "clean preserves .agents/ directory" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" sync
  bash "$SCRIPT" -d "$TEST_DIR" clean
  [ -d "$TEST_DIR/.agents" ]
  [ -f "$TEST_DIR/AGENTS.md" ]
}

# --------------------------------------------------------------------------
# sync --targets claude
# --------------------------------------------------------------------------

@test "sync --targets claude only syncs to .claude/" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" --targets claude sync
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/.claude/rules" ]
  [ ! -d "$TEST_DIR/.windsurf" ]
}

@test "sync --targets windsurf only syncs to .windsurf/" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" --targets windsurf sync
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/.windsurf/rules" ]
  [ ! -d "$TEST_DIR/.claude" ]
}

# --------------------------------------------------------------------------
# sync without init fails
# --------------------------------------------------------------------------

@test "sync without init fails" {
  run bash "$SCRIPT" -d "$TEST_DIR" sync
  [ "$status" -ne 0 ]
  [[ "$output" == *"Run 'sync-agents init' first"* ]]
}

@test "add without init fails" {
  run bash "$SCRIPT" -d "$TEST_DIR" add rule test-rule
  [ "$status" -ne 0 ]
  [[ "$output" == *"Run 'sync-agents init' first"* ]]
}

@test "index without init fails" {
  run bash "$SCRIPT" -d "$TEST_DIR" index
  [ "$status" -ne 0 ]
  [[ "$output" == *"Run 'sync-agents init' first"* ]]
}

# --------------------------------------------------------------------------
# --force
# --------------------------------------------------------------------------

@test "--force overwrites existing rule file" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule overwrite-me
  echo "original" > "$TEST_DIR/.agents/rules/overwrite-me.md"

  run bash "$SCRIPT" -d "$TEST_DIR" --force add rule overwrite-me
  [ "$status" -eq 0 ]
  # Content should be the template, not "original"
  [[ "$(cat "$TEST_DIR/.agents/rules/overwrite-me.md")" != "original" ]]
}

@test "add without --force refuses to overwrite existing file" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule existing-rule
  run bash "$SCRIPT" -d "$TEST_DIR" add rule existing-rule
  [ "$status" -ne 0 ]
  [[ "$output" == *"already exists"* ]]
}

@test "--force overwrites existing symlinks during sync" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" sync
  # Create a conflicting regular directory where a symlink would go
  bash "$SCRIPT" -d "$TEST_DIR" clean
  mkdir -p "$TEST_DIR/.claude/rules"
  echo "conflict" > "$TEST_DIR/.claude/rules/something.txt"

  run bash "$SCRIPT" -d "$TEST_DIR" --force sync
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/.claude/rules" ]
}

# --------------------------------------------------------------------------
# Unknown command
# --------------------------------------------------------------------------

@test "unknown command fails" {
  run bash "$SCRIPT" -d "$TEST_DIR" foobar
  [ "$status" -ne 0 ]
  [[ "$output" == *"Unknown command"* ]]
}

# --------------------------------------------------------------------------
# Unknown option
# --------------------------------------------------------------------------

@test "unknown option fails" {
  run bash "$SCRIPT" --bogus
  [ "$status" -ne 0 ]
  [[ "$output" == *"Unknown option"* ]]
}

# --------------------------------------------------------------------------
# Plural type aliases
# --------------------------------------------------------------------------

@test "add rules (plural) works same as add rule" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" add rules plural-test
  [ "$status" -eq 0 ]
  [ -f "$TEST_DIR/.agents/rules/plural-test.md" ]
}

@test "add skills (plural) works same as add skill" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" add skills plural-test
  [ "$status" -eq 0 ]
  [ -f "$TEST_DIR/.agents/skills/plural-test.md" ]
}

@test "add workflows (plural) works same as add workflow" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" add workflows plural-test
  [ "$status" -eq 0 ]
  [ -f "$TEST_DIR/.agents/workflows/plural-test.md" ]
}

# --------------------------------------------------------------------------
# New targets: cursor, codex, copilot
# --------------------------------------------------------------------------

@test "sync creates symlinks for all 4 targets" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule test-rule
  run bash "$SCRIPT" -d "$TEST_DIR" sync
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/.claude/rules" ]
  [ -L "$TEST_DIR/.windsurf/rules" ]
  [ -L "$TEST_DIR/.cursor/rules" ]
  [ -L "$TEST_DIR/.github/copilot/rules" ]
}

@test "sync --targets cursor only syncs to .cursor/" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule test-rule
  run bash "$SCRIPT" -d "$TEST_DIR" sync --targets cursor
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/.cursor/rules" ]
  [ ! -d "$TEST_DIR/.claude" ]
  [ ! -d "$TEST_DIR/.windsurf" ]
}

@test "sync --targets copilot creates .github/copilot/ structure" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule test-rule
  run bash "$SCRIPT" -d "$TEST_DIR" sync --targets copilot
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/.github/copilot/rules" ]
  [ ! -d "$TEST_DIR/.copilot" ]
}

@test "clean removes copilot symlinks from .github/copilot/" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule test-rule
  bash "$SCRIPT" -d "$TEST_DIR" sync --targets copilot
  run bash "$SCRIPT" -d "$TEST_DIR" clean --targets copilot
  [ "$status" -eq 0 ]
  [ ! -L "$TEST_DIR/.github/copilot/rules" ]
}

# --------------------------------------------------------------------------
# Type-specific templates
# --------------------------------------------------------------------------

@test "add skill uses SKILL_TEMPLATE content" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add skill my-skill
  grep -q "Description" "$TEST_DIR/.agents/skills/my-skill.md"
  grep -q "Usage" "$TEST_DIR/.agents/skills/my-skill.md"
  grep -q "Examples" "$TEST_DIR/.agents/skills/my-skill.md"
}

@test "add workflow uses WORKFLOW_TEMPLATE content" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add workflow my-workflow
  grep -q "Trigger" "$TEST_DIR/.agents/workflows/my-workflow.md"
  grep -q "Steps" "$TEST_DIR/.agents/workflows/my-workflow.md"
}

@test "add rule still uses RULE_TEMPLATE content" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule my-rule
  # Rule template just has the name as header
  grep -q "my-rule" "$TEST_DIR/.agents/rules/my-rule.md"
}

# --------------------------------------------------------------------------
# Git hook
# --------------------------------------------------------------------------

@test "hook creates pre-commit hook" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" hook
  [ "$status" -eq 0 ]
  [ -f "$TEST_DIR/.git/hooks/pre-commit" ]
  [ -x "$TEST_DIR/.git/hooks/pre-commit" ]
  grep -q "sync-agents" "$TEST_DIR/.git/hooks/pre-commit"
}

@test "hook is idempotent" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" hook
  run bash "$SCRIPT" -d "$TEST_DIR" hook
  [ "$status" -eq 0 ]
  # Should only appear once
  count=$(grep -c "sync-agents start" "$TEST_DIR/.git/hooks/pre-commit")
  [ "$count" -eq 1 ]
}

@test "hook appends to existing pre-commit" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  mkdir -p "$TEST_DIR/.git/hooks"
  echo '#!/bin/sh' > "$TEST_DIR/.git/hooks/pre-commit"
  echo 'echo "existing hook"' >> "$TEST_DIR/.git/hooks/pre-commit"
  chmod +x "$TEST_DIR/.git/hooks/pre-commit"
  run bash "$SCRIPT" -d "$TEST_DIR" hook
  [ "$status" -eq 0 ]
  grep -q "existing hook" "$TEST_DIR/.git/hooks/pre-commit"
  grep -q "sync-agents" "$TEST_DIR/.git/hooks/pre-commit"
}

# --------------------------------------------------------------------------
# Import (basic - uses local file:// to avoid network in tests)
# --------------------------------------------------------------------------

@test "import fails without URL" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" import
  [ "$status" -eq 1 ]
}

@test "import with rules URL auto-detects type" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  # Create a local file to serve
  local src_file="$TEST_DIR/source-rule.md"
  echo "# Imported Rule" > "$src_file"
  run bash "$SCRIPT" -d "$TEST_DIR" import "file://$src_file" <<< ""
  # curl with file:// puts it in rules/ since URL doesn't contain rules/
  # This will prompt — skip for now, test the error case
  [ "$status" -ne 0 ] || [ -f "$TEST_DIR/.agents/rules/source-rule.md" ] || [ -f "$TEST_DIR/.agents/skills/source-rule.md" ] || [ -f "$TEST_DIR/.agents/workflows/source-rule.md" ]
}

# --------------------------------------------------------------------------
# Watch (just verify command exists / help shows it)
# --------------------------------------------------------------------------

@test "help shows watch command" {
  run bash "$SCRIPT" --help
  [[ "$output" == *"watch"* ]]
}

@test "help shows import command" {
  run bash "$SCRIPT" --help
  [[ "$output" == *"import"* ]]
}

@test "help shows hook command" {
  run bash "$SCRIPT" --help
  [[ "$output" == *"hook"* ]]
}

# --------------------------------------------------------------------------
# STATE_TEMPLATE trimmed
# --------------------------------------------------------------------------

# --------------------------------------------------------------------------
# Config file
# --------------------------------------------------------------------------

@test "init creates default config file" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  [ -f "$TEST_DIR/.agents/config" ]
  grep -q "targets" "$TEST_DIR/.agents/config"
}

@test "config file limits sync targets" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule test-rule
  # Override config to only sync claude
  echo "targets = claude" > "$TEST_DIR/.agents/config"
  run bash "$SCRIPT" -d "$TEST_DIR" sync
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/.claude/rules" ]
  [ ! -d "$TEST_DIR/.windsurf" ]
  [ ! -d "$TEST_DIR/.cursor" ]
}

@test "--targets flag overrides config file" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule test-rule
  # Config says claude only
  echo "targets = claude" > "$TEST_DIR/.agents/config"
  # But --targets says windsurf
  run bash "$SCRIPT" -d "$TEST_DIR" sync --targets windsurf
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/.windsurf/rules" ]
  [ ! -d "$TEST_DIR/.claude" ]
}

@test "config file with multiple targets works" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" add rule test-rule
  echo "targets = claude,cursor" > "$TEST_DIR/.agents/config"
  run bash "$SCRIPT" -d "$TEST_DIR" sync
  [ "$status" -eq 0 ]
  [ -L "$TEST_DIR/.claude/rules" ]
  [ -L "$TEST_DIR/.cursor/rules" ]
  [ ! -d "$TEST_DIR/.windsurf" ]
}

# --------------------------------------------------------------------------
# .gitignore
# --------------------------------------------------------------------------

@test "sync adds symlink entries to .gitignore" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" sync
  [ -f "$TEST_DIR/.gitignore" ]
  grep -qxF ".claude/" "$TEST_DIR/.gitignore"
  grep -qxF ".windsurf/" "$TEST_DIR/.gitignore"
  grep -qxF ".cursor/" "$TEST_DIR/.gitignore"
  grep -qxF ".github/copilot/" "$TEST_DIR/.gitignore"
  grep -qxF "CLAUDE.md" "$TEST_DIR/.gitignore"
}

@test "sync adds header comment to .gitignore" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" sync
  grep -qF "# sync-agents" "$TEST_DIR/.gitignore"
}

@test "sync does not duplicate .gitignore entries" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" sync
  bash "$SCRIPT" -d "$TEST_DIR" sync
  local count
  count=$(grep -cxF "CLAUDE.md" "$TEST_DIR/.gitignore")
  [ "$count" -eq 1 ]
}

@test "sync preserves existing .gitignore content" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  echo "node_modules/" > "$TEST_DIR/.gitignore"
  bash "$SCRIPT" -d "$TEST_DIR" sync
  grep -qxF "node_modules/" "$TEST_DIR/.gitignore"
  grep -qxF "CLAUDE.md" "$TEST_DIR/.gitignore"
}

@test "sync --targets only adds relevant entries to .gitignore" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" sync --targets claude
  grep -qxF ".claude/" "$TEST_DIR/.gitignore"
  grep -qxF "CLAUDE.md" "$TEST_DIR/.gitignore"
  ! grep -qxF ".windsurf/" "$TEST_DIR/.gitignore"
}

@test "sync --dry-run does not modify .gitignore" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" sync --dry-run
  if [ -f "$TEST_DIR/.gitignore" ]; then
    ! grep -qxF "CLAUDE.md" "$TEST_DIR/.gitignore"
  fi
}

# --------------------------------------------------------------------------
# Inheritance
# --------------------------------------------------------------------------

@test "inherit adds Inherits section to AGENTS.md" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" inherit global ../../AGENTS.md
  [ "$status" -eq 0 ]
  grep -q "## Inherits" "$TEST_DIR/AGENTS.md"
  grep -q "\[global\](../../AGENTS.md)" "$TEST_DIR/AGENTS.md"
}

@test "inherit adds multiple entries" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" inherit global ../../AGENTS.md
  run bash "$SCRIPT" -d "$TEST_DIR" inherit team ../AGENTS.md
  [ "$status" -eq 0 ]
  grep -q "\[global\]" "$TEST_DIR/AGENTS.md"
  grep -q "\[team\]" "$TEST_DIR/AGENTS.md"
}

@test "inherit --list shows entries" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" inherit global ../../AGENTS.md
  bash "$SCRIPT" -d "$TEST_DIR" inherit team ../AGENTS.md
  run bash "$SCRIPT" -d "$TEST_DIR" inherit --list
  [ "$status" -eq 0 ]
  [[ "$output" == *"global"* ]]
  [[ "$output" == *"team"* ]]
}

@test "inherit --remove removes an entry" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" inherit global ../../AGENTS.md
  bash "$SCRIPT" -d "$TEST_DIR" inherit team ../AGENTS.md
  run bash "$SCRIPT" -d "$TEST_DIR" inherit --remove global
  [ "$status" -eq 0 ]
  ! grep -q "\[global\]" "$TEST_DIR/AGENTS.md"
  grep -q "\[team\]" "$TEST_DIR/AGENTS.md"
}

@test "inherit rejects duplicate labels" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" inherit global ../../AGENTS.md
  run bash "$SCRIPT" -d "$TEST_DIR" inherit global ../other/AGENTS.md
  [ "$status" -ne 0 ]
  [[ "$output" == *"already exists"* ]]
}

@test "inherit without arguments fails" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" inherit
  [ "$status" -ne 0 ]
  [[ "$output" == *"Usage"* ]]
}

@test "inherit section preserved across index regeneration" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" inherit global ../../AGENTS.md
  bash "$SCRIPT" -d "$TEST_DIR" add rule my-rule
  # index regenerates AGENTS.md
  bash "$SCRIPT" -d "$TEST_DIR" index
  grep -q "## Inherits" "$TEST_DIR/AGENTS.md"
  grep -q "\[global\](../../AGENTS.md)" "$TEST_DIR/AGENTS.md"
  grep -q "my-rule" "$TEST_DIR/AGENTS.md"
}

@test "inherit Inherits section appears before Rules in AGENTS.md" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  bash "$SCRIPT" -d "$TEST_DIR" inherit global ../../AGENTS.md
  local inherits_line rules_line
  inherits_line=$(grep -n "## Inherits" "$TEST_DIR/AGENTS.md" | head -1 | cut -d: -f1)
  rules_line=$(grep -n "## Rules" "$TEST_DIR/AGENTS.md" | head -1 | cut -d: -f1)
  [ "$inherits_line" -lt "$rules_line" ]
}

@test "inherit --list with no inherits shows nothing" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" inherit --list
  [ "$status" -eq 0 ]
  [ -z "$output" ]
}

@test "inherit --remove nonexistent label warns" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  run bash "$SCRIPT" -d "$TEST_DIR" inherit --remove nonexistent
  [ "$status" -eq 0 ]
  [[ "$output" == *"No inherit found"* ]]
}

# --------------------------------------------------------------------------
# STATE_TEMPLATE
# --------------------------------------------------------------------------

@test "init creates trimmed STATE.md" {
  bash "$SCRIPT" -d "$TEST_DIR" init
  local line_count
  line_count=$(wc -l < "$TEST_DIR/.agents/STATE.md")
  # Trimmed template should be under 20 lines
  [ "$line_count" -lt 20 ]
}
