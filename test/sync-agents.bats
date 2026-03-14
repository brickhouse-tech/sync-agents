#!/usr/bin/env bats

# Resolve the script under test relative to this test file
SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../src/sh" && pwd)/sync-agents.sh"

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
  [[ "$output" == *"sync-agents v0.1.0"* ]]
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
