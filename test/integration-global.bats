#!/usr/bin/env bats
#
# Integration tests for v0.3.0 commands: promote, global init/sync/status/clean.
#
# Isolation strategy: --global-root "$GLOBAL_ROOT" sets the global .agents/ dir.
# Per-tool dirs (claude, codeium, cursor, etc.) are derived from GLOBAL_ROOT's
# parent ($TEST_DIR), so nothing touches the real ~/.claude/ or ~/.codeium/.
#
# Run: npx bats test/integration-global.bats
# Or:  make test  (runs all bats in test/)

_REPO_ROOT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)"
SCRIPT="${SYNC_AGENTS_BIN:-$_REPO_ROOT/bin/sync-agents}"
PACKAGE_VERSION="$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' "$_REPO_ROOT/package.json" | head -1)"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

setup() {
  # OpenClaw's workspace follows these vars; exported ones would point
  # the openclaw target at a live workspace instead of $TEST_DIR.
  unset OPENCLAW_HOME OPENCLAW_STATE_DIR OPENCLAW_CONFIG_PATH OPENCLAW_WORKSPACE_DIR OPENCLAW_PROFILE

  TEST_DIR="$(mktemp -d)"
  GLOBAL_ROOT="$TEST_DIR/.agents"   # per-tool dirs live under $TEST_DIR

  # Project workspace (for promote tests that read from project .agents/)
  PROJECT_DIR="$TEST_DIR/project"
  mkdir -p "$PROJECT_DIR"
  git init --quiet "$PROJECT_DIR"

  # Derived tool dirs (global scope, parent = $TEST_DIR)
  CLAUDE_DIR="$TEST_DIR/.claude"
  CODEIUM_DIR="$TEST_DIR/.codeium/windsurf"
  CURSOR_DIR="$TEST_DIR/.cursor"
  COPILOT_DIR="$TEST_DIR/.copilot"
  CODEX_DIR="$TEST_DIR/.codex"
  OPENCODE_DIR="$TEST_DIR/.config/opencode"
  LEGACY_COPILOT="$TEST_DIR/.github/copilot/instructions.md"
}

teardown() {
  rm -rf "$TEST_DIR"
}

# Seed a project-level .agents/ so promote tests have source artifacts.
init_project() {
  "$SCRIPT" init -d "$PROJECT_DIR"
}

# Write a passive rule to the global root (rules default to passive).
make_global_passive_rule() {
  local name="$1"
  mkdir -p "$GLOBAL_ROOT/rules"
  cat > "$GLOBAL_ROOT/rules/$name.md" <<EOF
---
invocable: false
---
# $name
Passive rule content for $name.
EOF
}

# Write an invocable rule to the global root.
make_global_invocable_rule() {
  local name="$1"
  mkdir -p "$GLOBAL_ROOT/rules"
  cat > "$GLOBAL_ROOT/rules/$name.md" <<EOF
---
invocable: true
---
# $name
Invocable rule content for $name.
EOF
}

# Write a single-file invocable skill to the global root.
make_global_single_skill() {
  local name="$1"
  mkdir -p "$GLOBAL_ROOT/skills/$name"
  cat > "$GLOBAL_ROOT/skills/$name/SKILL.md" <<EOF
---
invocable: true
---
# $name Skill
Skill content for $name.
EOF
}

# Write a multi-file invocable skill to the global root (will be SKIP for Windsurf).
make_global_multi_skill() {
  local name="$1"
  mkdir -p "$GLOBAL_ROOT/skills/$name"
  cat > "$GLOBAL_ROOT/skills/$name/SKILL.md" <<EOF
---
invocable: true
---
# $name Skill
EOF
  echo "# helper" > "$GLOBAL_ROOT/skills/$name/helper.sh"
}

# Write an invocable workflow to the global root.
make_global_invocable_workflow() {
  local name="$1"
  mkdir -p "$GLOBAL_ROOT/workflows"
  cat > "$GLOBAL_ROOT/workflows/$name.md" <<EOF
---
invocable: true
---
# $name Workflow
Invocable workflow content.
EOF
}

# Write a passive workflow to the global root.
make_global_passive_workflow() {
  local name="$1"
  mkdir -p "$GLOBAL_ROOT/workflows"
  cat > "$GLOBAL_ROOT/workflows/$name.md" <<EOF
---
invocable: false
---
# $name Workflow
Passive workflow content.
EOF
}

# ---------------------------------------------------------------------------
# global init
# ---------------------------------------------------------------------------

@test "global init creates .agents/ skeleton at --global-root" {
  run "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ -d "$GLOBAL_ROOT/rules" ]
  [ -d "$GLOBAL_ROOT/skills" ]
  [ -d "$GLOBAL_ROOT/workflows" ]
  [ -f "$GLOBAL_ROOT/config" ]
}

@test "global init config file contains global target list" {
  run "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  grep -q "targets" "$GLOBAL_ROOT/config"
  grep -q "claude" "$GLOBAL_ROOT/config"
  grep -q "cursor" "$GLOBAL_ROOT/config"
}

@test "global init is idempotent" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  echo "custom = value" >> "$GLOBAL_ROOT/config"
  run "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  # Custom config line must survive
  grep -q "custom = value" "$GLOBAL_ROOT/config"
}

@test "global init --dry-run does not create directories" {
  run "$SCRIPT" global init --global-root "$GLOBAL_ROOT" --dry-run
  [ "$status" -eq 0 ]
  [ ! -d "$GLOBAL_ROOT" ]
}

@test "global init respects SYNC_AGENTS_GLOBAL_ROOT env var" {
  SYNC_AGENTS_GLOBAL_ROOT="$GLOBAL_ROOT" run "$SCRIPT" global init
  [ "$status" -eq 0 ]
  [ -d "$GLOBAL_ROOT/rules" ]
}

# ---------------------------------------------------------------------------
# promote — canonical form
# ---------------------------------------------------------------------------

@test "promote rule copies rule to global root" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add rule my-rule -d "$PROJECT_DIR"

  run "$SCRIPT" promote rule my-rule -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ -f "$GLOBAL_ROOT/rules/my-rule.md" ]
  diff "$PROJECT_DIR/.agents/rules/my-rule.md" "$GLOBAL_ROOT/rules/my-rule.md"
}

@test "promote skill copies skill directory to global root" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add skill my-skill -d "$PROJECT_DIR"

  run "$SCRIPT" promote skill my-skill -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ -f "$GLOBAL_ROOT/skills/my-skill/SKILL.md" ]
}

@test "promote workflow copies workflow to global root" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add workflow my-flow -d "$PROJECT_DIR"

  run "$SCRIPT" promote workflow my-flow -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ -f "$GLOBAL_ROOT/workflows/my-flow.md" ]
}

@test "promote fails when destination exists and no --force" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add rule dup-rule -d "$PROJECT_DIR"
  "$SCRIPT" promote rule dup-rule -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"

  run "$SCRIPT" promote rule dup-rule -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"already exists"* ]] || [[ "$output" == *"--force"* ]]
}

@test "promote --force overwrites existing destination" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add rule dup-rule -d "$PROJECT_DIR"
  "$SCRIPT" promote rule dup-rule -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"

  echo "# Updated" >> "$PROJECT_DIR/.agents/rules/dup-rule.md"
  run "$SCRIPT" promote rule dup-rule -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT" --force
  [ "$status" -eq 0 ]
  diff "$PROJECT_DIR/.agents/rules/dup-rule.md" "$GLOBAL_ROOT/rules/dup-rule.md"
}

@test "promote --dry-run does not copy file" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add rule dry-rule -d "$PROJECT_DIR"

  run "$SCRIPT" promote rule dry-rule -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT" --dry-run
  [ "$status" -eq 0 ]
  [[ "$output" == *"dry-run"* ]] || [[ "$output" == *"would"* ]]
  [ ! -f "$GLOBAL_ROOT/rules/dry-rule.md" ]
}

@test "promote fails for unknown type" {
  init_project
  run "$SCRIPT" promote badtype foo -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"unknown type"* ]]
}

@test "promote fails when source artifact does not exist" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  run "$SCRIPT" promote rule no-such-rule -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"not found"* ]]
}

@test "promote accepts plural type aliases" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add rule alias-rule -d "$PROJECT_DIR"
  run "$SCRIPT" promote rules alias-rule -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ -f "$GLOBAL_ROOT/rules/alias-rule.md" ]
}

# ---------------------------------------------------------------------------
# promote — path form
# ---------------------------------------------------------------------------

@test "promote path form auto-detects rule type" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add rule path-rule -d "$PROJECT_DIR"

  # Path form expects a path relative to project root; run from $PROJECT_DIR.
  run bash -c "cd '$PROJECT_DIR' && '$SCRIPT' promote .agents/rules/path-rule.md \
    --global-root '$GLOBAL_ROOT'"
  [ "$status" -eq 0 ]
  [ -f "$GLOBAL_ROOT/rules/path-rule.md" ]
}

@test "promote path form auto-detects skill type" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add skill path-skill -d "$PROJECT_DIR"

  run bash -c "cd '$PROJECT_DIR' && '$SCRIPT' promote .agents/skills/path-skill \
    --global-root '$GLOBAL_ROOT'"
  [ "$status" -eq 0 ]
  [ -f "$GLOBAL_ROOT/skills/path-skill/SKILL.md" ]
}

@test "promote path outside .agents/ is an error" {
  init_project
  run bash -c "cd '$PROJECT_DIR' && '$SCRIPT' promote /tmp/outside.md \
    --global-root '$GLOBAL_ROOT'"
  [ "$status" -ne 0 ]
}

# ---------------------------------------------------------------------------
# promote --sync composite
# ---------------------------------------------------------------------------

@test "promote --sync promotes and then runs global sync" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add rule sync-rule -d "$PROJECT_DIR"
  # Write frontmatter so it's passive → goes to claude/rules/
  cat > "$PROJECT_DIR/.agents/rules/sync-rule.md" <<'EOF'
---
invocable: false
---
# sync-rule
Content.
EOF

  run "$SCRIPT" promote rule sync-rule -d "$PROJECT_DIR" \
    --global-root "$GLOBAL_ROOT" --sync --sync-targets claude
  [ "$status" -eq 0 ]
  # Global root has the copy
  [ -f "$GLOBAL_ROOT/rules/sync-rule.md" ]
  # Claude symlink created by the embedded global sync
  [ -L "$CLAUDE_DIR/rules/sync-rule.md" ]
}

# ---------------------------------------------------------------------------
# global sync — Claude routing
# ---------------------------------------------------------------------------

@test "global sync: passive rule symlinked to claude/rules/" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "sec-rule"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [ -L "$CLAUDE_DIR/rules/sec-rule.md" ]
  # Symlink resolves to the global root source
  [[ "$(readlink "$CLAUDE_DIR/rules/sec-rule.md")" == *"$GLOBAL_ROOT"* ]]
}

@test "global sync: invocable rule symlinked to claude/commands/" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_invocable_rule "deploy-rule"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [ -L "$CLAUDE_DIR/commands/deploy-rule.md" ]
}

@test "global sync: invocable single-file skill symlinked to claude/skills/<name>/SKILL.md" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_single_skill "code-review"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [ -L "$CLAUDE_DIR/skills/code-review/SKILL.md" ]
}

@test "global sync: invocable workflow symlinked to claude/commands/" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_invocable_workflow "deploy-flow"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [ -L "$CLAUDE_DIR/commands/deploy-flow.md" ]
}

@test "global sync: passive skill to claude is skipped (no single-file destination)" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  mkdir -p "$GLOBAL_ROOT/skills/passive-skill"
  cat > "$GLOBAL_ROOT/skills/passive-skill/SKILL.md" <<'EOF'
---
invocable: false
---
# Passive Skill
EOF

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  # No symlink created for passive skill in claude
  [ ! -e "$CLAUDE_DIR/skills/passive-skill" ]
  [ ! -e "$CLAUDE_DIR/rules/passive-skill.md" ]
}

# ---------------------------------------------------------------------------
# global sync — Codeium routing
# ---------------------------------------------------------------------------

@test "global sync: passive rule lands in the codeium-rules region of global_rules.md" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "no-secrets"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets codeium
  [ "$status" -eq 0 ]
  [ "$(head -1 "$CODEIUM_DIR/memories/global_rules.md")" = "<!-- sync-agents:codeium-rules:start -->" ]
  grep -q "## no-secrets" "$CODEIUM_DIR/memories/global_rules.md"
}

@test "global sync: a legacy bannered global_rules.md becomes the region without --targets" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "no-secrets"
  mkdir -p "$CODEIUM_DIR/memories"
  printf '<!--\nGenerated by sync-agents — do not edit by hand.\n-->\n\n## old\n\nold\n' > "$CODEIUM_DIR/memories/global_rules.md"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [[ "$output" == *"Rewrote the old generated file as region codeium-rules"* ]]
  ! grep -q "## old" "$CODEIUM_DIR/memories/global_rules.md"
  grep -q "## no-secrets" "$CODEIUM_DIR/memories/global_rules.md"
}

@test "global sync: invocable single-file workflow symlinked to codeium/windsurf/global_workflows/" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_invocable_workflow "deploy-flow"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets codeium
  [ "$status" -eq 0 ]
  [ -L "$CODEIUM_DIR/global_workflows/deploy-flow.md" ]
}

@test "global sync: multi-file skill is skipped for codeium" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_multi_skill "big-skill"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets codeium
  [ "$status" -eq 0 ]
  [ ! -e "$CODEIUM_DIR/global_workflows/big-skill.md" ]
  [[ "$output" == *"skip"* ]] || [[ "$output" == *"Windsurf"* ]]
}

# ---------------------------------------------------------------------------
# global sync — Cursor, Copilot, Codex, opencode routing and legacy cleanup
# ---------------------------------------------------------------------------

@test "global sync: cursor gets no user-rules file and loses its legacy rule links" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "passive-r"
  mkdir -p "$CURSOR_DIR/rules"
  ln -s "$GLOBAL_ROOT/rules/passive-r.md" "$CURSOR_DIR/rules/passive-r.md"
  echo "user rule" > "$CURSOR_DIR/rules/mine.mdc"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets cursor
  [ "$status" -eq 0 ]
  [ ! -e "$CURSOR_DIR/rules/passive-r.md" ]
  [ -f "$CURSOR_DIR/rules/mine.mdc" ]

  run "$SCRIPT" global status --global-root "$GLOBAL_ROOT" --targets cursor
  [[ "$output" == *"[gap] cursor"* ]]
}

@test "global sync: copilot links ~/.copilot/instructions to ~/.agents/index/copilot.md" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "sec-rule"
  make_global_invocable_workflow "deploy-flow"
  mkdir -p "$COPILOT_DIR"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ "$(readlink "$COPILOT_DIR/instructions/sync-agents.instructions.md")" = "$GLOBAL_ROOT/index/copilot.md" ]
  grep -q '^applyTo: "\*\*"$' "$COPILOT_DIR/instructions/sync-agents.instructions.md"
  grep -q "## sec-rule" "$COPILOT_DIR/instructions/sync-agents.instructions.md"
  ! grep -q "deploy-flow" "$COPILOT_DIR/instructions/sync-agents.instructions.md"
}

@test "global sync: copilot is left alone when ~/.copilot does not exist" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "sec-rule"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ ! -e "$COPILOT_DIR" ]
  [ ! -e "$GLOBAL_ROOT/index/copilot.md" ]
}

@test "global sync: codex gets the codex-rules region; a file of yours needs --targets once" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "my-rule"
  mkdir -p "$CODEX_DIR"
  printf '# Mine\n\nKeep this.\n' > "$CODEX_DIR/AGENTS.md"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [[ "$output" != *"codex"* ]]
  ! grep -q "codex-rules" "$CODEX_DIR/AGENTS.md"
  run "$SCRIPT" global status --global-root "$GLOBAL_ROOT"
  [[ "$output" == *"[unmounted] codex"* ]]
  [[ "$output" == *"run once with --targets codex"* ]]

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets codex
  [ "$status" -eq 0 ]
  [ "$(head -1 "$CODEX_DIR/AGENTS.md")" = "# Mine" ]
  grep -q "Keep this." "$CODEX_DIR/AGENTS.md"
  grep -q "<!-- sync-agents:codex-rules:start -->" "$CODEX_DIR/AGENTS.md"
  grep -q "## my-rule" "$CODEX_DIR/AGENTS.md"
}

@test "global status: codex region shadowed by ~/.codex/AGENTS.override.md" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "my-rule"
  mkdir -p "$CODEX_DIR"
  echo "override" > "$CODEX_DIR/AGENTS.override.md"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT"

  run "$SCRIPT" global status --global-root "$GLOBAL_ROOT" --targets codex
  [ "$status" -eq 0 ]
  [[ "$output" == *"[shadowed] codex"* ]]
}

@test "global sync: opencode.json gets the absolute index path" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "my-rule"
  mkdir -p "$OPENCODE_DIR"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  grep -qF "\"$GLOBAL_ROOT/index/opencode.md\"" "$OPENCODE_DIR/opencode.json"
}

@test "global sync: removes bannered legacy instructions.md files, keeps yours" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "my-rule"
  mkdir -p "$(dirname "$LEGACY_COPILOT")" "$CODEX_DIR"
  printf '<!--\nGenerated by sync-agents — do not edit by hand.\n-->\n' > "$LEGACY_COPILOT"
  echo "my own notes" > "$CODEX_DIR/instructions.md"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ ! -e "$TEST_DIR/.github" ]
  [ -f "$CODEX_DIR/instructions.md" ]
  [[ "$output" == *"it does not start with the sync-agents banner"* ]]
}

# ---------------------------------------------------------------------------
# global sync — flags and contracts
# ---------------------------------------------------------------------------

@test "global sync --dry-run makes no filesystem changes" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "dry-rule"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --dry-run
  [ "$status" -eq 0 ]
  [[ "$output" == *"dry-run"* ]] || [[ "$output" == *"would"* ]]
  [ ! -e "$CLAUDE_DIR" ]
  [ ! -e "$CURSOR_DIR" ]
}

@test "global sync --targets filters to specified tools only" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "filtered-rule"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [ -L "$CLAUDE_DIR/rules/filtered-rule.md" ]
  [ ! -e "$CURSOR_DIR" ]
  [ ! -e "$COPILOT_DIR" ]
}

@test "global sync is idempotent — second run rewrites nothing" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "idem-rule"
  mkdir -p "$COPILOT_DIR" "$CODEX_DIR"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT"
  before="$(cat "$CODEX_DIR/AGENTS.md" "$GLOBAL_ROOT/index/copilot.md")"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  # The second run reports no write. This asserts idempotency directly
  # instead of relying on mtime, which can be coarse on CI filesystems.
  [[ "$output" != *"Regenerated"* ]]
  [[ "$output" != *"Created"* ]]
  [[ "$output" != *"Updated region"* ]]
  [[ "$output" != *"Linked"* ]]
  [ "$(cat "$CODEX_DIR/AGENTS.md" "$GLOBAL_ROOT/index/copilot.md")" = "$before" ]
}

@test "global sync repairs drifted symlink on next run" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "drift-rule"
  make_global_passive_rule "other-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude

  # Drift the link to another artifact INSIDE the global tree. Per
  # SPEC-011 Part A this is the "ours, drifted" case — the link points
  # back into the tree we manage, so sync repairs it in place.
  ln -sfn "$GLOBAL_ROOT/rules/other-rule.md" "$CLAUDE_DIR/rules/drift-rule.md"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [[ "$(readlink "$CLAUDE_DIR/rules/drift-rule.md")" == *"/rules/drift-rule.md" ]]
}

@test "global sync leaves a foreign symlink untouched without --force" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "keep-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude

  # A symlink pointing OUTSIDE the global tree is foreign wiring, not
  # ours to repair (SPEC-011 Part A hazard fix). Without --force sync
  # must leave it exactly as-is and warn rather than destroy it.
  ln -sfn /tmp/wrong "$CLAUDE_DIR/rules/keep-rule.md"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [[ "$(readlink "$CLAUDE_DIR/rules/keep-rule.md")" == "/tmp/wrong" ]]
  [[ "$output" == *"--force"* ]]
}

@test "global sync --force replaces a foreign symlink and backs it up" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "take-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude

  ln -sfn /tmp/wrong "$CLAUDE_DIR/rules/take-rule.md"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude --force
  [ "$status" -eq 0 ]
  # The link now points back into the managed tree...
  [[ "$(readlink "$CLAUDE_DIR/rules/take-rule.md")" == *"/rules/take-rule.md" ]]
  # ...and the displaced foreign link is preserved, never deleted.
  ls "$CLAUDE_DIR/rules/"take-rule.md.replaced-by-sync-agents* >/dev/null 2>&1
}

@test "global sync fails when global root does not exist" {
  run "$SCRIPT" global sync --global-root "$TEST_DIR/.no-such-root"
  [ "$status" -ne 0 ]
  [[ "$output" == *"does not exist"* ]] || [[ "$output" == *"global init"* ]]
}

@test "global sync: frontmatter stripped from channel entries" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "strip-rule"

  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets codex
  # the region must NOT contain raw YAML frontmatter delimiters
  run grep -c "^---$" "$CODEX_DIR/AGENTS.md"
  [ "$output" -eq 0 ]
  grep -q "Passive rule content for strip-rule." "$CODEX_DIR/AGENTS.md"
}

# ---------------------------------------------------------------------------
# global status
# ---------------------------------------------------------------------------

@test "global status shows [synced] for a correctly linked artifact" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "my-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude

  run "$SCRIPT" global status --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [[ "$output" == *"synced"* ]]
  [[ "$output" == *"my-rule"* ]]
}

@test "global status shows [missing] when symlink absent" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "miss-rule"
  # Do NOT sync — symlink will be absent

  run "$SCRIPT" global status --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [[ "$output" == *"missing"* ]]
}

@test "global status shows [drifted] for wrong-target symlink" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "drift-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  ln -sfn /tmp/wrong "$CLAUDE_DIR/rules/drift-rule.md"

  run "$SCRIPT" global status --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [[ "$output" == *"drifted"* ]]
}

@test "global status shows a synced channel row after sync" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "channel-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets copilot

  run "$SCRIPT" global status --global-root "$GLOBAL_ROOT" --targets copilot
  [ "$status" -eq 0 ]
  [[ "$output" == *"[synced] copilot -> $COPILOT_DIR/instructions/sync-agents.instructions.md"* ]]
}

@test "global status shows an unmounted channel row before sync" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "no-sync-rule"
  # Do NOT sync; Copilot is not installed

  run "$SCRIPT" global status --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [[ "$output" == *"[unmounted] copilot"* ]]
  [[ "$output" == *"tool not installed"* ]]
}

@test "global status --targets filters output to specified tools" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "any-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT"

  run "$SCRIPT" global status --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [[ "$output" == *"claude"* ]]
  # codex should not appear in filtered output
  [[ "$output" != *"codex"* ]]
}

@test "global status errors when global root does not exist" {
  run "$SCRIPT" global status --global-root "$TEST_DIR/.no-root"
  [ "$status" -ne 0 ]
}

# ---------------------------------------------------------------------------
# global clean
# ---------------------------------------------------------------------------

@test "global clean removes sync-agents symlinks from tool dirs" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "clean-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ -L "$CLAUDE_DIR/rules/clean-rule.md" ]

  run "$SCRIPT" global clean --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [ ! -e "$CLAUDE_DIR/rules/clean-rule.md" ]
}

@test "global clean removes the channels: link, region, config entry, index" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "rule-for-clean"
  mkdir -p "$COPILOT_DIR" "$CODEX_DIR" "$OPENCODE_DIR"
  echo "{}" > "$COPILOT_DIR/config.json"
  printf '# Mine\n' > "$CODEX_DIR/AGENTS.md"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets copilot,codex,opencode

  run "$SCRIPT" global clean --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ ! -e "$COPILOT_DIR/instructions" ]
  [ -f "$COPILOT_DIR/config.json" ]
  [ "$(cat "$CODEX_DIR/AGENTS.md")" = "# Mine" ]
  [ ! -e "$OPENCODE_DIR/opencode.json" ]
  [ ! -e "$GLOBAL_ROOT/index" ]
}

@test "global clean does NOT remove user-owned files (no banner)" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "owned-rule"
  mkdir -p "$(dirname "$LEGACY_COPILOT")"
  echo "# My custom instructions" > "$LEGACY_COPILOT"

  run "$SCRIPT" global clean --global-root "$GLOBAL_ROOT" --targets copilot
  [ "$status" -eq 0 ]
  # File must still exist — it's user-owned
  grep -q "My custom instructions" "$LEGACY_COPILOT"
}

@test "global clean does NOT remove user-owned symlinks (outside global root)" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  mkdir -p "$CLAUDE_DIR/rules"
  ln -sfn /tmp/user-rule.md "$CLAUDE_DIR/rules/user-rule.md"

  run "$SCRIPT" global clean --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [ -L "$CLAUDE_DIR/rules/user-rule.md" ]
}

@test "global clean --dry-run makes no changes" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "preserve-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude

  run "$SCRIPT" global clean --global-root "$GLOBAL_ROOT" --targets claude --dry-run
  [ "$status" -eq 0 ]
  [[ "$output" == *"dry-run"* ]] || [[ "$output" == *"would"* ]]
  [ -L "$CLAUDE_DIR/rules/preserve-rule.md" ]
}

@test "global clean --targets filters to specified tools only" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "multi-tool-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude,cursor

  run "$SCRIPT" global clean --global-root "$GLOBAL_ROOT" --targets cursor
  [ "$status" -eq 0 ]
  [ ! -e "$CURSOR_DIR/rules/multi-tool-rule.md" ]
  # Claude symlink must still exist (not cleaned)
  [ -L "$CLAUDE_DIR/rules/multi-tool-rule.md" ]
}

@test "global clean is idempotent" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "idem-clean"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude

  "$SCRIPT" global clean --global-root "$GLOBAL_ROOT" --targets claude
  run "$SCRIPT" global clean --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
}

@test "global clean does not remove .agents/ source tree" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_passive_rule "src-rule"
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  "$SCRIPT" global clean --global-root "$GLOBAL_ROOT"

  [ -f "$GLOBAL_ROOT/rules/src-rule.md" ]
}

@test "global clean errors when global root does not exist" {
  run "$SCRIPT" global clean --global-root "$TEST_DIR/.no-root"
  [ "$status" -ne 0 ]
}

# ---------------------------------------------------------------------------
# SYNC_AGENTS_GLOBAL_ROOT environment variable
# ---------------------------------------------------------------------------

@test "SYNC_AGENTS_GLOBAL_ROOT env var used when --global-root not set" {
  SYNC_AGENTS_GLOBAL_ROOT="$GLOBAL_ROOT" run "$SCRIPT" global init
  [ "$status" -eq 0 ]
  [ -d "$GLOBAL_ROOT/rules" ]
}

@test "--global-root flag takes precedence over SYNC_AGENTS_GLOBAL_ROOT" {
  OTHER_ROOT="$TEST_DIR/.other-agents"
  SYNC_AGENTS_GLOBAL_ROOT="$OTHER_ROOT" \
    run "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ -d "$GLOBAL_ROOT/rules" ]
  [ ! -d "$OTHER_ROOT" ]
}

# ---------------------------------------------------------------------------
# Full round-trip: init → promote → global sync → global status → global clean
# ---------------------------------------------------------------------------

@test "full round-trip: project rule promoted, synced, verified, and cleaned" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"

  # Create passive project rule
  cat > "$PROJECT_DIR/.agents/rules/rtt-rule.md" <<'EOF'
---
invocable: false
---
# RTT Rule
Round-trip test content.
EOF

  # Promote to global
  "$SCRIPT" promote rule rtt-rule \
    -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"
  [ -f "$GLOBAL_ROOT/rules/rtt-rule.md" ]

  # Sync to Claude
  "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ -L "$CLAUDE_DIR/rules/rtt-rule.md" ]

  # Status must report synced
  status_out="$("$SCRIPT" global status --global-root "$GLOBAL_ROOT" --targets claude)"
  [[ "$status_out" == *"synced"* ]]
  [[ "$status_out" == *"rtt-rule"* ]]

  # Clean removes symlink
  "$SCRIPT" global clean --global-root "$GLOBAL_ROOT" --targets claude
  [ ! -e "$CLAUDE_DIR/rules/rtt-rule.md" ]

  # Source must still be in global root
  [ -f "$GLOBAL_ROOT/rules/rtt-rule.md" ]
}

# ---------------------------------------------------------------------------
# agents bucket (SPEC-004 Part B)
# ---------------------------------------------------------------------------

make_global_agent() {
  local name="$1"
  mkdir -p "$GLOBAL_ROOT/agents"
  cat > "$GLOBAL_ROOT/agents/$name.md" <<AGENTEOF
---
name: $name
description: Reviews code. Use when a review is requested.
---
You are the $name subagent.
AGENTEOF
}

@test "promote agent copies agent to global root" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add agent my-reviewer -d "$PROJECT_DIR"

  run "$SCRIPT" promote agent my-reviewer -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ -f "$GLOBAL_ROOT/agents/my-reviewer.md" ]
}

@test "global sync: agent symlinked to claude/agents/" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_agent "reviewer"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude
  [ "$status" -eq 0 ]
  [ -L "$CLAUDE_DIR/agents/reviewer.md" ]
  [[ "$(readlink "$CLAUDE_DIR/agents/reviewer.md")" == *"$GLOBAL_ROOT"* ]]
}

@test "global sync: agent is skipped for non-claude tools" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  make_global_agent "reviewer"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets cursor,codex
  [ "$status" -eq 0 ]
  [[ "$output" == *"no subagent surface"* ]]
  [ ! -e "$CURSOR_DIR/rules/reviewer.md" ]
  if [ -f "$CODEX_DIR/AGENTS.md" ]; then
    ! grep -q "reviewer" "$CODEX_DIR/AGENTS.md"
  fi
}

# ---------------------------------------------------------------------------
# plans + specs buckets (SPEC-004 Part D)
# ---------------------------------------------------------------------------

@test "global sync: plan symlinked to claude/plans/, skipped elsewhere" {
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  mkdir -p "$GLOBAL_ROOT/plans"
  printf -- '---\nname: roadmap\ndescription: Roadmap. Use when planning.\n---\n# Roadmap\n' > "$GLOBAL_ROOT/plans/roadmap.md"

  run "$SCRIPT" global sync --global-root "$GLOBAL_ROOT" --targets claude,cursor
  [ "$status" -eq 0 ]
  [ -L "$CLAUDE_DIR/plans/roadmap.md" ]
  [ ! -e "$CURSOR_DIR/rules/roadmap.md" ]
}

@test "promote spec copies spec to global root" {
  init_project
  "$SCRIPT" global init --global-root "$GLOBAL_ROOT"
  "$SCRIPT" add spec my-spec -d "$PROJECT_DIR"

  run "$SCRIPT" promote spec my-spec -d "$PROJECT_DIR" --global-root "$GLOBAL_ROOT"
  [ "$status" -eq 0 ]
  [ -f "$GLOBAL_ROOT/specs/my-spec.md" ]
}
