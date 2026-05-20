# sync-agents v0.3.0 — Local QA Test Plan

Manual walkthrough covering every command in the Go binary. Run these steps
in order; each section builds on the previous. Expected output is shown where
it matters for verification.

---

## 0. Install locally from source

```bash
cd ~/code/brickhouse-tech/sync-agents
git checkout feat/v0.3.0
git pull --ff-only

# Build and install to a local bin dir (stays off your $PATH by default)
npm run build       # produces bin/sync-agents

# Quick smoke-test — must print version, not "dev"
./bin/sync-agents --version
# Expected: sync-agents v0.2.5  (or whatever package.json says)

# Optional: put it on PATH for the session
export PATH="$(pwd)/bin:$PATH"
```

> **`go install` note:** If you want to test the module-proxy version path:
> `go install github.com/brickhouse-tech/sync-agents@v0.2.5`
> then `sync-agents --version` should also print `v0.2.5` (from BuildInfo).

---

## 1. Pre-flight — scratch workspace

Create a clean test directory so nothing touches real projects:

```bash
export QA_DIR=$(mktemp -d)   # e.g. /tmp/tmp.abc123
export SA=~/code/brickhouse-tech/sync-agents/bin/sync-agents
cd "$QA_DIR"
git init --quiet             # needed for hook + project-root detection
echo "QA workspace: $QA_DIR"
```

All commands below assume `$QA_DIR` is the cwd unless stated otherwise.

---

## 2. Version & help

```bash
$SA --version
# sync-agents v0.2.5

$SA -v
# sync-agents v0.2.5

$SA version
# sync-agents v0.2.5

$SA --help
# Must list all commands: init, sync, status, add, index, clean, watch,
# import, hook, fix, inherit, promote, global, version

$SA global --help
# Must list: init, sync, status, clean

$SA promote --help
# Must show: <type> <name> | <path>, --sync, --sync-targets, --dry-run, --force
```

---

## 3. `init`

```bash
$SA init
# Prints tree: .agents/{rules,skills,workflows}, AGENTS.md created

ls .agents/
# rules  skills  workflows  config

cat .agents/config
# targets = claude,windsurf,cursor,copilot

cat .agents/rules/state.md | head -5
# frontmatter + State rule template

ls AGENTS.md
# AGENTS.md exists

# Idempotency — run again, must not error or overwrite
$SA init
# Prints warnings that files exist, does not fail
```

---

## 4. `add`

```bash
# Add a rule
$SA add rule no-secrets
# Created .agents/rules/no-secrets.md

cat .agents/rules/no-secrets.md
# Must contain "no-secrets" in the title/heading

# Add a skill (creates directory layout)
$SA add skill code-review
# Created .agents/skills/code-review/SKILL.md

ls .agents/skills/code-review/
# SKILL.md

# Add a workflow
$SA add workflow pr-checklist
# Created .agents/workflows/pr-checklist.md

# Singular aliases work
$SA add skills linting
# Created .agents/skills/linting/SKILL.md

# Unknown type errors
$SA add foobar baz 2>&1
# Error: unknown type; must be one of: rule, skill, workflow

# Missing name errors
$SA add rule 2>&1
# Error: missing args

# Duplicate without --force errors
$SA add rule no-secrets 2>&1
# Error: file already exists

# Duplicate with --force succeeds
$SA add rule no-secrets --force
# Overwrites .agents/rules/no-secrets.md — no error
```

---

## 5. `index`

```bash
$SA index

cat AGENTS.md
# Must contain:
# ---
# trigger: always_on
# ---
# ## Rules
#   - [no-secrets](.agents/rules/no-secrets.md)
#   - [state](.agents/rules/state.md)
# ## Skills
#   - [code-review](.agents/skills/code-review/SKILL.md)
#   - [linting](.agents/skills/linting/SKILL.md)
# ## Workflows
#   - [pr-checklist](.agents/workflows/pr-checklist.md)

# Add another rule then re-index
$SA add rule no-eval
$SA index

grep "no-eval" AGENTS.md
# Must match: - [no-eval](.agents/rules/no-eval.md)
```

---

## 6. `sync`

```bash
$SA sync

# Verify symlinks created for all targets
ls -la .claude/
# rules -> ../../.agents/rules
# skills -> ../../.agents/skills
# workflows -> ../../.agents/workflows

ls -la .windsurf/ .cursor/ .github/copilot/
# Same symlink structure for each

ls -la CLAUDE.md
# CLAUDE.md -> AGENTS.md  (symlink)

# Dry-run — shows intent but makes no changes
rm -rf .claude/ .windsurf/ .cursor/ .github/ CLAUDE.md

$SA sync --dry-run
# Prints "would link ..." lines
# Nothing actually created:
ls .claude 2>&1 | grep "No such"

# Now really sync
$SA sync

# Verify a file change is visible via symlink
echo "# extra rule" >> .agents/rules/no-secrets.md
cat .claude/rules/no-secrets.md | tail -1
# extra rule   (symlink, so reflects change immediately)

# Target filtering
rm -rf .windsurf/ .cursor/
$SA sync --targets claude,cursor
# Only .claude/ and .cursor/ touched; .windsurf/ stays absent
ls .windsurf 2>&1 | grep "No such"
ls .cursor/rules
```

---

## 7. `status`

```bash
$SA status
# Prints version
# .agents/: [ok]
# AGENTS.md: [ok]
# CLAUDE.md: [ok] (symlink to AGENTS.md)
# For each target: [synced] or [missing]

# Break a symlink and re-check
rm -rf .cursor/
$SA status
# .cursor/rules: [missing]
# .cursor/skills: [missing]
# .cursor/workflows: [missing]

# Restore
$SA sync
$SA status
# All [synced]
```

---

## 8. `clean`

```bash
# Dry-run first
$SA clean --dry-run
# Prints "would remove ..." for each symlink

$SA clean
# Removes .claude/rules .claude/skills .claude/workflows CLAUDE.md etc.

ls .claude/ 2>/dev/null || echo "empty or gone"
ls -la CLAUDE.md 2>&1 | grep "No such"

# Clean is idempotent — no error if already clean
$SA clean
# Exits 0, prints nothing or "nothing to clean"

# Restore
$SA sync
```

---

## 9. `hook`

```bash
cat .git/hooks/pre-commit 2>/dev/null || echo "(not yet)"

$SA hook
# Installed pre-commit hook

cat .git/hooks/pre-commit
# Must contain:
# --- sync-agents start ---
# sync-agents sync
# sync-agents index
# git add AGENTS.md CLAUDE.md ...
# --- sync-agents end ---

# file must be executable
ls -la .git/hooks/pre-commit | awk '{print $1}'
# -rwxr-xr-x

# Idempotent
$SA hook
# Prints "already installed" or similar, exits 0

# Verify hook appears only once in file
grep -c "sync-agents start" .git/hooks/pre-commit
# 1

# Non-git directory errors
$SA hook -d /tmp 2>&1
# Error: Not a git repository
```

---

## 10. `watch`

*(Requires `fswatch` on macOS — `brew install fswatch` if needed)*

```bash
# Verify fswatch present first
which fswatch || echo "install fswatch"

# Run watch in background, touch a file, check index updated
$SA watch &
WATCH_PID=$!
sleep 1

touch .agents/rules/watch-test.md
sleep 1   # give watcher time to react

grep "watch-test" AGENTS.md && echo "WATCH OK"

kill $WATCH_PID 2>/dev/null
rm .agents/rules/watch-test.md
$SA index   # restore clean index

# Without fswatch, must print install hint
# (test only if fswatch is absent on the machine)
```

---

## 11. `import`

```bash
# Import from a real URL (uses examples in the repo)
RULES_RAW="https://raw.githubusercontent.com/brickhouse-tech/sync-agents/feat/v0.3.0/examples/rules"
$SA import "$RULES_RAW/no-secrets.md"
# Download + added to .agents/rules/no-secrets.md (overwrites with --force)
# AGENTS.md regenerated automatically

ls .agents/rules/no-secrets.md

# Type auto-detection from URL path segment
$SA import "$RULES_RAW/no-secrets.md" --force
# Re-imports; type detected as "rule" from URL

# Invalid URL fails
$SA import "https://example.invalid/no-such-file.md" 2>&1
# Error: Failed to download
```

---

## 12. `fix`

```bash
# Set up a legacy layout scenario
mkdir -p "$QA_DIR/legacy_test"
cd "$QA_DIR/legacy_test"
git init --quiet

# Create top-level (legacy) skills dir
mkdir -p skills
echo "# Legacy Skill" > skills/old-skill.md

# Create flat skill file
mkdir -p .agents/skills
echo "# Flat Skill" > .agents/skills/flat-skill.md

$SA fix skills

# flat-skill.md must be converted to dir layout
ls .agents/skills/flat-skill/
# SKILL.md

# No-clobber: existing items skipped
$SA fix skills --no-clobber
# Skips existing entries

# Legacy STATE.md migration
mkdir -p .agents
cat > .agents/STATE.md <<'EOF'
---
trigger: always_on
---
# State
## STATE HISTORY BELOW
### 20260101 STATE: test
Did some work.
EOF

$SA fix
# Legacy STATE.md removed
ls .agents/STATE.md 2>&1 | grep "No such"
# A STATE_legacy-history_*.md file created
ls .agents/STATE_legacy-history_*.md

# Symlink repair: break a synced symlink then fix
cd "$QA_DIR"
rm .cursor/rules
$SA fix
# .cursor/rules symlink recreated

$SA status
# .cursor/rules: [synced]

# Unknown type errors
$SA fix foobar 2>&1
# Error: Unknown type: foobar

cd "$QA_DIR"
```

---

## 13. `inherit`

```bash
# Requires AGENTS.md (init already ran)

# Add inheritance links
$SA inherit global ~/.agents
# Adds - [global](~/.agents) to ## Inherits in AGENTS.md

$SA inherit org ../org/AGENTS.md
# Adds - [org](../org/AGENTS.md)

# List
$SA inherit --list
# - [global](~/.agents)
# - [org](../org/AGENTS.md)

# Duplicate label errors
$SA inherit global /some/other/path 2>&1
# Error: label already exists; use --remove first

# Remove
$SA inherit --remove org
$SA inherit --list
# - [global](~/.agents)  (org gone)

# Preserved across re-index
$SA index
grep "Inherits" AGENTS.md
# ## Inherits still there with global link

# No AGENTS.md errors
cd /tmp && $SA inherit global ~/.agents 2>&1
# Error: No AGENTS.md found
cd "$QA_DIR"
```

---

## 14. `global init`

```bash
export QA_GLOBAL="$QA_DIR/.qa-global-agents"

$SA global init --global-root "$QA_GLOBAL"

ls "$QA_GLOBAL"
# rules  skills  workflows  config

cat "$QA_GLOBAL/config"
# targets = claude,codeium,cursor,copilot,codex

# Idempotent
$SA global init --global-root "$QA_GLOBAL"
# No error, no overwrite of existing config

# Dry-run
export QA_GLOBAL2="$QA_DIR/.qa-global-dry"
$SA global init --global-root "$QA_GLOBAL2" --dry-run
ls "$QA_GLOBAL2" 2>&1 | grep "No such"
# Directory NOT created
```

---

## 15. `promote`

First, set up a rule and skill locally, then promote to global:

```bash
# Canonical form — rule
$SA promote rule no-secrets --global-root "$QA_GLOBAL"
# Promoted rule no-secrets to $QA_GLOBAL/rules/no-secrets.md

ls "$QA_GLOBAL/rules/no-secrets.md"
diff .agents/rules/no-secrets.md "$QA_GLOBAL/rules/no-secrets.md"
# No diff — exact copy

# Canonical form — skill (copies directory)
$SA promote skill code-review --global-root "$QA_GLOBAL"
# Promoted skill code-review to $QA_GLOBAL/skills/code-review/

ls "$QA_GLOBAL/skills/code-review/SKILL.md"

# Duplicate without --force fails
$SA promote rule no-secrets --global-root "$QA_GLOBAL" 2>&1
# Error: already exists; pass --force to overwrite

# --force overwrites
echo "# Updated" >> .agents/rules/no-secrets.md
$SA promote rule no-secrets --global-root "$QA_GLOBAL" --force
diff .agents/rules/no-secrets.md "$QA_GLOBAL/rules/no-secrets.md"
# No diff — updated

# Dry-run
$SA promote workflow pr-checklist --global-root "$QA_GLOBAL" --dry-run
# Prints "[dry-run] would copy ..."
ls "$QA_GLOBAL/workflows/pr-checklist.md" 2>&1 | grep "No such"

# Path form (auto-detects type)
$SA promote .agents/rules/no-eval.md --global-root "$QA_GLOBAL" --force
ls "$QA_GLOBAL/rules/no-eval.md"

# Unknown type
$SA promote badtype foo --global-root "$QA_GLOBAL" 2>&1
# Error: unknown type

# Source not found
$SA promote rule doesnotexist --global-root "$QA_GLOBAL" 2>&1
# Error: "doesnotexist" not found in .agents/rules/

# Promote + sync composite (promote and fan out in one step)
$SA promote workflow pr-checklist --global-root "$QA_GLOBAL" \
    --sync --sync-targets claude,cursor
# Promotes, then runs global sync for claude and cursor
```

---

## 16. `global sync`

Set up some rules and skills in the global root first:

```bash
# Add a passive rule (invocable: false)
cat > "$QA_GLOBAL/rules/security.md" <<'EOF'
---
invocable: false
---
# Security
Never commit secrets.
EOF

# Add an invocable skill
cat > "$QA_GLOBAL/skills/code-review/SKILL.md" <<'EOF'
---
invocable: true
---
# Code Review Skill
Review code for quality.
EOF

# Add an invocable workflow
cat > "$QA_GLOBAL/workflows/deploy.md" <<'EOF'
---
invocable: true
---
# Deploy Workflow
Steps to deploy.
EOF

# Full sync
export QA_TOOL_ROOT="$QA_DIR/.qa-tool-root"
mkdir -p "$QA_TOOL_ROOT"

# Use --global-root to keep QA isolated; per-tool dirs go under $HOME by default.
# For QA, we'll verify actual tool dirs:
$SA global sync --global-root "$QA_GLOBAL"

# Verify Claude (passive rule → ~/.claude/rules/, invocable workflow → ~/.claude/commands/)
ls ~/.claude/rules/security.md
# Must be a symlink → $QA_GLOBAL/rules/security.md
readlink ~/.claude/rules/security.md | grep "$QA_GLOBAL"

ls ~/.claude/commands/deploy.md
# Must be a symlink

ls ~/.claude/skills/code-review/SKILL.md
# Symlink for invocable skill

# Verify Cursor (all → ~/.cursor/rules/)
ls ~/.cursor/rules/security.md
ls ~/.cursor/rules/deploy.md

# Verify Codeium/Windsurf concat files
cat ~/.codeium/windsurf/memories/global_rules.md | head -5
# Must contain sync-agents banner + content of security.md

cat ~/.codeium/windsurf/global_workflows/deploy.md 2>/dev/null || echo "(windsurf uses single-file workflows)"

# Verify Codex concat
ls ~/.codex/instructions.md && head -5 ~/.codex/instructions.md

# Verify Copilot concat
ls ~/.github/copilot/instructions.md && head -5 ~/.github/copilot/instructions.md

# Dry-run
$SA global sync --global-root "$QA_GLOBAL" --dry-run
# Prints "[dry-run] would link ..." and "[dry-run] would regenerate concat ..."
# No filesystem changes if already in sync

# Target filtering
$SA global sync --global-root "$QA_GLOBAL" --targets claude
# Only Claude symlinks touched

# Idempotency — second sync must print "already current" for concat files
$SA global sync --global-root "$QA_GLOBAL" 2>&1 | grep "already current"

# Missing global root errors
$SA global sync --global-root /tmp/doesnotexist 2>&1
# Error: global root ... does not exist; run `sync-agents global init` first
```

---

## 17. `global status`

```bash
$SA global status --global-root "$QA_GLOBAL"
# Prints per-artifact status lines:
# [synced] claude/rule/security -> ~/.claude/rules/security.md
# [synced] claude/skill/code-review -> ~/.claude/skills/code-review/SKILL.md
# [synced] claude/workflow/deploy -> ~/.claude/commands/deploy.md
# [concat ok] ~/.codeium/windsurf/memories/global_rules.md  (1 entries)
# etc.

# Break a symlink manually then check status
rm ~/.claude/rules/security.md
$SA global status --global-root "$QA_GLOBAL"
# [missing] claude/rule/security -> ~/.claude/rules/security.md

# Restore
$SA global sync --global-root "$QA_GLOBAL"

# Drifted symlink
ln -sfn /tmp/wrong ~/.claude/rules/security.md
$SA global status --global-root "$QA_GLOBAL"
# [drifted] claude/rule/security -> ~/.claude/rules/security.md

# Restore
$SA global sync --global-root "$QA_GLOBAL"

# Target filtering
$SA global status --global-root "$QA_GLOBAL" --targets claude
# Only claude entries printed

# No artifacts
mkdir -p "$QA_DIR/.empty-global"
$SA global status --global-root "$QA_DIR/.empty-global" 2>&1
# Error or "no artifacts ... nothing to sync"
```

---

## 18. `global clean`

```bash
# Dry-run first
$SA global clean --global-root "$QA_GLOBAL" --dry-run
# Prints "[dry-run] would remove ..." for each sync-agents-managed symlink/file

# Real clean
$SA global clean --global-root "$QA_GLOBAL"
# Removes all sync-agents symlinks from ~/.claude/ ~/.cursor/ etc.
# Removes sync-agents-owned concat files

ls ~/.claude/rules/security.md 2>&1 | grep "No such"
ls ~/.claude/commands/deploy.md 2>&1 | grep "No such"

# Must NOT remove ~/.agents/ source tree
ls "$QA_GLOBAL/rules/security.md"   # still there

# User-owned files (no banner) are left alone
echo "# my notes" > ~/.claude/rules/my-own-rule.md
$SA global sync --global-root "$QA_GLOBAL"
$SA global clean --global-root "$QA_GLOBAL"
ls ~/.claude/rules/my-own-rule.md   # must still exist

# Target filtering
$SA global sync --global-root "$QA_GLOBAL"
$SA global clean --global-root "$QA_GLOBAL" --targets cursor
ls ~/.cursor/rules/security.md 2>&1 | grep "No such"
ls ~/.claude/rules/security.md   # claude still synced

# Idempotent — clean twice is safe
$SA global clean --global-root "$QA_GLOBAL"
$SA global clean --global-root "$QA_GLOBAL"
# Exits 0 both times
```

---

## 19. Cross-cutting checks

### `--dir` flag

```bash
PROJ2=$(mktemp -d)
git init --quiet "$PROJ2"
$SA init -d "$PROJ2"
ls "$PROJ2/.agents/"

$SA add rule my-rule -d "$PROJ2"
ls "$PROJ2/.agents/rules/my-rule.md"
```

### `--dry-run` on all mutating commands

```bash
# Every command that writes should respect dry-run
# (spot-check: sync, clean, fix, promote, global sync, global clean)
$SA sync --dry-run        # no writes
$SA clean --dry-run       # no writes
$SA fix --dry-run         # no writes
$SA promote rule no-eval --global-root "$QA_GLOBAL" --dry-run  # no writes
$SA global sync --global-root "$QA_GLOBAL" --dry-run           # no writes
$SA global clean --global-root "$QA_GLOBAL" --dry-run          # no writes
```

### `--force` overwrites

```bash
# Create a real file at a symlink target, then force-sync
mkdir -p .claude2
echo "real file" > .claude2/rules
$SA sync --dir "$QA_DIR" --force
# .claude2/rules replaced with symlink (only applies if .claude2 is a target)
```

### `$SYNC_AGENTS_GLOBAL_ROOT` env var

```bash
export SYNC_AGENTS_GLOBAL_ROOT="$QA_GLOBAL"
$SA global status   # no --global-root flag needed; picks up env var
unset SYNC_AGENTS_GLOBAL_ROOT
```

---

## 20. Error paths

```bash
# Command on non-initialized project
EMPTY=$(mktemp -d)
git init --quiet "$EMPTY"
$SA sync -d "$EMPTY" 2>&1
# Error: .agents/ directory not found. Run 'sync-agents init' first.

$SA index -d "$EMPTY" 2>&1
# Error about missing .agents/

# Unknown command
$SA foobar 2>&1
# Error: unknown command

# Global commands without global init
$SA global sync --global-root "$QA_DIR/.no-such-global" 2>&1
# Error: global root ... does not exist; run `sync-agents global init` first

# promote: source missing
$SA promote rule no-such-rule --global-root "$QA_GLOBAL" 2>&1
# Error: "no-such-rule" not found in .agents/rules/
```

---

## 21. Teardown

```bash
# Clean up QA tool-dir artifacts
$SA global clean --global-root "$QA_GLOBAL" 2>/dev/null || true
rm -rf "$QA_DIR"
echo "QA complete"
```

---

## Checklist summary

| Area | Command(s) | Pass? |
|---|---|---|
| Version | `--version`, `-v`, `version` | ☐ |
| Init (idempotent) | `init` | ☐ |
| Add (types, duplicates, errors) | `add` | ☐ |
| Index (all sections, inherits preserved) | `index` | ☐ |
| Sync (symlinks, targets, dry-run) | `sync` | ☐ |
| Status (all states) | `status` | ☐ |
| Clean (dry-run, idempotent) | `clean` | ☐ |
| Hook (install, idempotent, non-git error) | `hook` | ☐ |
| Watch (change detection) | `watch` | ☐ |
| Import (URL, type detection, fail) | `import` | ☐ |
| Fix (flat→dir, legacy STATE, symlink repair) | `fix` | ☐ |
| Inherit (add, list, remove, preserved) | `inherit` | ☐ |
| Global init (idempotent, dry-run) | `global init` | ☐ |
| Promote (canonical, path, --force, --sync) | `promote` | ☐ |
| Global sync (routing, concat, dry-run, idempotent) | `global sync` | ☐ |
| Global status (synced/drifted/missing/concat) | `global status` | ☐ |
| Global clean (symlinks, concat, user-owned safe) | `global clean` | ☐ |
| Cross-cutting (`--dir`, `--dry-run`, `--force`, env var) | all | ☐ |
| Error paths | various | ☐ |
