# sync-agents

One set of agent rules to rule them all. `sync-agents` keeps your AI coding agent configurations in a single `.agents/` directory and syncs them to agent-specific directories (`.claude/`, `.windsurf/`, `.cursor/`, `.github/copilot/`) via symlinks. This ensures all agents follow the same rules, skills, workflows, subagents, plans, and specs without duplicating files.

AGENTS.md serves as an auto-generated index of everything in `.agents/` and is symlinked to CLAUDE.md for Claude compatibility.

## Installation

### npm (recommended for Node.js projects)

Ships native Go binaries via per-platform optional packages — no build step required.

```bash
npm install -g @brickhouse-tech/sync-agents
```

Or as a project devDependency:

```bash
npm install --save-dev @brickhouse-tech/sync-agents
```

### go install (no Node.js required)

```bash
go install github.com/brickhouse-tech/sync-agents@latest
```

Requires Go 1.21+. The binary is placed in `$GOPATH/bin` (or `$HOME/go/bin`). Version is read from the module proxy at install time via `debug.ReadBuildInfo`.

### Homebrew

```bash
brew install brickhouse-tech/tap/sync-agents
```

The tap is updated automatically on every release via GoReleaser.

### GitHub Releases (pre-built binaries)

Download the archive for your platform from the [Releases page](https://github.com/brickhouse-tech/sync-agents/releases), extract, and place the binary on your `PATH`:

```bash
# Example: macOS arm64
curl -fsSL https://github.com/brickhouse-tech/sync-agents/releases/latest/download/sync-agents_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/').tar.gz | tar -xz
sudo mv sync-agents /usr/local/bin/
```

SHA-256 checksums are published alongside each release as `checksums.txt`.

## Topology

`.agents/` is the source of truth. It contains all rules, skills, workflows, and state for your agents:

```
.agents/
  ├── config              # sync targets (claude, windsurf, cursor, copilot)
  ├── rules/
  │   ├── rule1.md
  │   ├── rule2.md
  │   └── ...
  ├── skills/
  │   ├── skill1/
  │   │   └── SKILL.md
  │   ├── skill2/
  │   │   └── SKILL.md
  │   └── ...
  ├── workflows/
  │   ├── workflow1.md
  │   ├── workflow2.md
  │   └── ...
  ├── agents/              # optional: Claude subagent definitions
  │   └── reviewer.md
  ├── plans/               # optional: per-effort implementation plans (how/when)
  │   └── auth-effort/
  │       └── rollout.md
  ├── specs/               # optional: durable design/requirements docs (what/why)
  │   └── SPEC-001.md
  ├── adrs/                # optional: Architecture Decision Records, status = subdirectory
  │   ├── proposed/
  │   │   └── adopt-grpc.md
  │   ├── accepted/
  │   │   └── use-postgres.md
  │   └── denied/          # kept but never indexed — prevents re-proposing rejected decisions
  │       └── use-mongo.md
  └── STATE.md
```

> **Note:** Skills use a directory layout (`skills/name/SKILL.md`) rather than flat files. This allows skills to include supporting files alongside their definition. The `fix` command can convert legacy flat skill files to the directory layout automatically.

> **Optional buckets:** `agents/`, `plans/`, `specs/`, `hooks/`, and `adrs/` activate only when their directory exists — `init` does not create them, `add agent|plan|spec|hook|adr <name>` does. They sync to Claude only (`.claude/agents`, `.claude/plans`, `.claude/specs`); other tools consume plans/specs through the `AGENTS.md` index and have no subagent surface. `plans/` and `specs/` share plumbing but differ in lifecycle: specs are durable what/why documents, plans are per-effort how/when documents that retire when the effort lands.

Running `sync-agents sync` creates symlinks from `.agents/` subdirectories into `.claude/`, `.windsurf/`, `.cursor/`, and `.github/copilot/`. Any changes to `.agents/` are automatically reflected in the target directories because they are symlinks, not copies.

AGENTS.md is also symlinked to CLAUDE.md so that Claude reads the index natively.

## STATE.md

`.agents/STATE.md` tracks the current state of your project from the agent's perspective. It serves as a resumption point after failures or interruptions -- the agent can read STATE.md to determine where it left off and what tasks remain. Update it regularly to keep agents in sync with progress.

## Commands

| Command | Description |
|---|---|
| `init` | Initialize the `.agents/` directory structure with `rules/`, `skills/`, `workflows/`, `STATE.md`, and generate `AGENTS.md` |
| `sync` | Create symlinks from `.agents/` into all target directories, and symlink `AGENTS.md` to `CLAUDE.md` |
| `watch` | Watch `.agents/` for changes and auto-regenerate `AGENTS.md` |
| `import <url>` | Import a rule/skill/workflow from a URL |
| `pull [--dry-run\|--offline\|--force\|--only NAME\|--global]` | Fetch every `sources.yaml` entry, verify integrity, install into the matching buckets |
| `update [NAME]` | Re-resolve refs and re-pull entries whose upstream moved; SHA-pinned entries are skipped |
| `source add <entry>` | Append an entry to `sources.yaml` and pull it |
| `source add --link[=<path>] [<entry>]` | Declare a **linked (editable)** source — symlink a live local checkout instead of a fetched snapshot (SPEC-007) |
| `source remove <name> [--keep]` | Remove the manifest entry and delete the artifact (`--keep` converts it to manual) |
| `source list [--json]` | Show each entry's local state: `ok` / `outdated` / `modified` / `missing` / `linked` |
| `source bundle` | Rebuild `sources.yaml` from installed artifacts' origin metadata |
| `source detach <name>` | Un-manage an artifact: flip its origin to manual and drop the manifest entry (for a **linked** source, freeze the live copy into a vendored snapshot) |
| `quarantine` | List remotely-fetched artifacts awaiting review, with their scan findings |
| `approve <name>\|--all [--force]` | Promote a quarantined artifact into `.agents/` (`--force` accepts critical findings, recorded in the lock) |
| `reject <name>\|--all` | Delete a quarantined artifact without installing it |
| `git-hook` | Install a pre-commit git hook for auto-sync (`hook` remains as a deprecated alias) |
| `inherit <label> <path>` | Add an inheritance link to AGENTS.md |
| `inherit --list` | List current inheritance links |
| `inherit --remove <label>` | Remove an inheritance link by label |
| `status` | Show the current sync status of all targets and symlinks |
| `add <type> <name>` | Add a new artifact from a template (type is `rule`, `skill`, `workflow`, `agent`, `plan`, `spec`, `hook`, or `adr`) |
| `index [--no-fix]` | Regenerate `AGENTS.md` by scanning `.agents/`. Backfills fixable skill frontmatter first (`--no-fix` skips the backfill) |
| `adr <accept\|deny\|propose> <name>` | Move an ADR between status directories, update its `status:` frontmatter, and reindex |
| `lint [skills] [--fix]` | Validate SKILL.md frontmatter against [Claude's skill authoring rules](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices); `--fix` amends fixable findings in place |
| `clean` | Remove all synced symlinks and empty target directories (does not remove `.agents/`) |
| `fix [type]` | Migrate legacy dirs into `.agents/`, convert flat skill files to directory layout, and repair broken symlinks. Type: any bucket dir, or `all` (default) |
| `promote <type> <name>` | Copy an artifact from the project's `.agents/` to the user-level global store (`~/.agents/`) |
| `global init` | Initialize the global `~/.agents/` store |
| `global sync` | Fan the global store out to each tool's user-level config dir with semantic-aware routing |
| `global status` | Show per-artifact sync state across global tool dirs |
| `global clean` | Remove global symlinks/concat files owned by sync-agents |

## Options

| Option | Description |
|---|---|
| `-h`, `--help` | Show help message |
| `-v`, `--version` | Show version |
| `-d`, `--dir <path>` | Set project root directory (default: current directory) |
| `--targets <list>` | Comma-separated list of sync targets (default: `claude,windsurf,cursor,copilot`) |
| `--dry-run` | Show what would be done without making changes |
| `--force` | Overwrite existing files and symlinks |
| `--no-clobber` | (fix only) Skip items that already exist in `.agents/` instead of merging |
| `--fix` | (lint only) Amend fixable frontmatter findings in place |
| `--no-fix` | (index only) Skip the skill frontmatter backfill |
| `--trust` | (pull/update only) Bypass the quarantine gate; the scan still runs and prints findings |

## Configuration

`sync-agents init` creates `.agents/config` with default sync targets:

```
# sync-agents configuration
# Comma-separated list of sync targets (available: claude, windsurf, cursor, copilot)
targets = claude,windsurf,cursor,copilot
```

Edit this file to limit which targets `sync` writes to by default. The `--targets` flag on any command overrides the config.

## Fix

The `fix` command handles three scenarios:

1. **Legacy directory migration** — Moves top-level `skills/`, `rules/`, or `workflows/` directories into `.agents/` and replaces them with symlinks.
2. **Flat skill conversion** — Converts `.agents/skills/name.md` flat files to the directory layout `.agents/skills/name/SKILL.md`.
3. **Symlink repair** — Recreates missing or broken symlinks in target directories (`.claude/`, `.windsurf/`, etc.) and the `CLAUDE.md` symlink.

```bash
# Fix everything (all types)
sync-agents fix

# Fix only skills
sync-agents fix skills

# Preview without changing anything
sync-agents fix --dry-run

# Don't overwrite items already in .agents/
sync-agents fix --no-clobber skills
```

A reproducible demo is available in [`examples/fix/`](examples/fix/):

```bash
bash examples/fix/run-demo.sh
```

## Skill frontmatter (lint & backfill)

Claude discovers skills through their `SKILL.md` YAML frontmatter, with [published requirements](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices): `name` (≤64 chars, lowercase letters/numbers/hyphens, no reserved words) and `description` (non-empty, ≤1024 chars, third person, says what the skill does *and when to use it*).

`sync-agents lint` checks every skill against those rules; `lint --fix` mechanically amends what it can — injecting a missing frontmatter block, deriving `name` from the directory, deriving `description` from the first body paragraph, truncating overlong values, and stripping XML tags — while preserving all other frontmatter keys verbatim. Reserved-word names are reported but never auto-renamed.

`sync-agents index` runs the same backfill by default before regenerating `AGENTS.md`, so legacy skills upgrade themselves as part of the normal index cycle (`--no-fix` opts out). Unfixable findings warn but never fail indexing.

```bash
# report compliance issues (exit non-zero on errors — CI-friendly)
sync-agents lint

# amend fixable findings in place
sync-agents lint --fix
```

## Source manifest (pull, lockfile & provenance)

Declare upstream rules, skills, workflows — or whole `.agents/` trees — in `.agents/sources.yaml` and install them reproducibly:

```yaml
version: 1
sources:
  - skill:anthropic/skill-pack@v1.2.0/skills/code-review
  - rule:my-org/agent-norms@main/rules/security.md
  - tree:my-org/team-agents@v2.0.0   # fans out to every bucket in the upstream .agents/
```

`sync-agents pull` resolves each ref to a commit SHA via the GitHub API, fetches the repo tarball (cached by SHA under `$XDG_CACHE_HOME/sync-agents/`, no `git` binary needed), verifies a deterministic sha256 content hash against `.agents/sources.lock`, and only then installs — a tampered tarball or corrupted cache aborts before anything is written. Every installed artifact carries provenance (`_origin.json` inside skill dirs, `<name>.origin.json` beside flat files) recording owner/repo/ref/SHA/hash; commit these so clones keep their provenance.

Safety rules: locally-edited artifacts are never overwritten without `--force`; a destination without origin metadata is treated as a manual conflict. Private repos use your token (`SYNC_AGENTS_GITHUB_TOKEN` > `GITHUB_TOKEN` > `GH_TOKEN` > `gh auth token`). All commands accept `--global` to operate on `~/.agents/` instead of the project.

```bash
sync-agents source add skill:anthropic/skill-pack@v1.2.0/skills/code-review
sync-agents source list          # ok / outdated / modified / missing per entry
sync-agents update               # bump tag-tracked entries when upstream moves
sync-agents pull --offline       # cache-only, for CI or airplanes
```

## Quarantine (remote content review)

Remote installs are treated like a hostile supply chain. By default, everything `pull`/`update` fetches lands in `.agents/.quarantine/` — invisible to `sync` and the index — after a static scan for network-then-execute patterns (`curl | bash`), credential access combined with network calls, obfuscation (long base64, zero-width Unicode), and prompt-injection phrasing aimed at your agent.

```bash
sync-agents pull                  # → 1 quarantined (run `sync-agents quarantine`)
sync-agents quarantine            # review findings per artifact
sync-agents approve code-review   # promote into .agents/ (blocked on CRITICAL unless --force)
sync-agents reject sketchy-rule   # delete without installing
```

Critical findings block `approve`; overriding with `--force` is recorded in `sources.lock` as `approved_with_findings` so the decision is auditable. `--trust` on `pull`/`update` skips the gate for one invocation (findings still print), and `quarantine = off` in `.agents/config` disables it for teams that review via pinned SHAs in PRs instead.

## Linked (editable) sources

When you're **actively developing** a skill that lives in another repo (or the upstream repo itself), a SHA-pinned snapshot forces a slow `edit → commit → push → update` loop. A **linked** source is the `npm link` / `go mod replace` for `.agents/` — it symlinks the artifact at a live local checkout, so edits flow both ways and `git pull` in the checkout reaps updates with no re-fetch (SPEC-007).

```bash
# Link a checkout you already have (path relative to cwd or absolute):
sync-agents source add --link=../foo-skill skill:me/foo-skill

# Derive the entry from the checkout's github remote + layout:
sync-agents source add --link=../foo-skill

# Managed clone: sync-agents clones the repo under .agents/.sources/ and links out of it
# (the initial clone is scanned once through the quarantine scanner; --trust to skip):
sync-agents source add --link skill:me/foo-skill
```

The link is recorded declaratively as a `link:` override in `sources.yaml` and echoed in `sources.lock`, so the intent is committed and reviewable:

```yaml
overrides:
  - match: skill:me/foo-skill
    link: file:../foo-skill      # relative to .agents/; mutually exclusive with pin_sha
```

All persisted paths are **relative** (`file:` scheme) and the on-disk symlink is created relative too — an absolute path would break the instant another machine cloned the repo, so absolute `file:` paths are rejected at parse time. `pull` verifies (and self-heals) the symlink and never re-fetches a snapshot over it; a missing checkout surfaces as a warning, not a silent revert. `update` advances a **managed clone** with `git pull --ff-only` (a checkout you own is yours to drive). `source list` shows linked entries as `[linked] … link → file:../foo-skill`, and `source detach` **freezes** the live copy into an ordinary vendored snapshot (materialize the files, drop the `link` override, keep the upstream identity). Because a linked source is a local working tree you own, it is trusted by default — only the first fetch of a managed clone is scanned.

## ADRs (Architecture Decision Records)

ADRs live in `.agents/adrs/` with **status encoded by subdirectory**: `proposed/`, `accepted/`, `denied/`. `add adr <name>` scaffolds into `proposed/`; `sync-agents adr accept|deny|propose <name>` moves a record between statuses (nested grouping subdirs are preserved) and regenerates the index.

Only **accepted and proposed** records appear in `AGENTS.md`. Denied records are kept on disk and the index carries a standing note telling agents to check `.agents/adrs/denied/` before proposing a new ADR — so already-rejected decisions don't get re-proposed.

```bash
sync-agents add adr use-postgres      # → .agents/adrs/proposed/use-postgres.md
sync-agents adr accept use-postgres   # → .agents/adrs/accepted/, reindexed
sync-agents adr deny use-postgres     # → .agents/adrs/denied/, dropped from index
```

## Inheritance

Projects can inherit agent rules from parent directories (org, team, global) using a convention-based approach. This enables hierarchical rule sharing without duplicating files.

### How It Works

Add an `## Inherits` section to your project's `AGENTS.md` that links to parent-level agent configs:

```markdown
## Inherits
- [global](../../AGENTS.md)
- [team](../AGENTS.md)
```

AI agents (Claude, Codex, etc.) follow markdown links natively — when they read your project's `AGENTS.md`, they'll traverse the inheritance chain and apply rules from all levels.

### Hierarchy Example

```
~/code/                     # Global: security norms, universal rules
  ├── .agents/
  ├── AGENTS.md
  └── org/                  # Org-level: coding standards, shared workflows
      ├── .agents/
      ├── AGENTS.md
      └── team/             # Team-level: language-specific rules
          ├── .agents/
          ├── AGENTS.md
          └── project/      # Project: project-specific rules + inherits
              ├── .agents/
              └── AGENTS.md  → ## Inherits links to team, org, global
```

**Inheritance is upward-only.** A project declares what it inherits from. Parent directories don't need to know about their children — when an agent works at the org level, it already has access to org-level rules.

### Managing Inheritance

```bash
# Add an inheritance link
sync-agents inherit global ../../AGENTS.md
sync-agents inherit team ../AGENTS.md

# List current inheritance links
sync-agents inherit --list

# Remove an inheritance link
sync-agents inherit --remove global
```

The `## Inherits` section is preserved across `sync-agents index` regenerations.

### Full Example

Set up a three-level hierarchy: global rules → org standards → project config.

```bash
# 1. Create global rules (e.g. ~/code/.agents/)
cd ~/code
sync-agents init
sync-agents add rule security
cat > .agents/rules/security.md << 'EOF'
---
trigger: always_on
---
# Security
- Never commit secrets or API keys
- Validate all external input
- Use parameterized queries for database access
EOF

# 2. Create org-level rules (e.g. ~/code/myorg/.agents/)
cd ~/code/myorg
sync-agents init
sync-agents add rule go-standards
cat > .agents/rules/go-standards.md << 'EOF'
---
trigger: always_on
---
# Go Standards
- Use `gofmt` and `golangci-lint` on all Go files
- Prefer table-driven tests
- Export only what consumers need
EOF

# 3. Create project with inheritance
cd ~/code/myorg/api-service
sync-agents init
sync-agents add rule api-conventions

# Link to parent levels
sync-agents inherit org ../AGENTS.md
sync-agents inherit global ../../AGENTS.md

# Sync to agent directories
sync-agents sync
```

The project's `AGENTS.md` now looks like:

```markdown
## Inherits
- [org](../AGENTS.md)
- [global](../../AGENTS.md)

## Rules
- [api-conventions](.agents/rules/api-conventions.md)

## Skills
_No skills defined yet._

## Workflows
_No workflows defined yet._
```

When an AI agent reads this file, it follows the `## Inherits` links and applies rules from all three levels — project-specific API conventions, org-wide Go standards, and global security rules.

### Verifying Inheritance

```bash
# Check what's inherited
sync-agents inherit --list
# Output:
# - [org](../AGENTS.md)
# - [global](../../AGENTS.md)

# Remove a link if no longer needed
sync-agents inherit --remove global

# Re-add with a different path
sync-agents inherit global ../../AGENTS.md
```

## Examples

The [`examples/`](examples/) directory contains ready-to-use rules, skills, and workflows. Import them directly:

```bash
sync-agents import https://raw.githubusercontent.com/brickhouse-tech/sync-agents/main/examples/rules/no-secrets.md
sync-agents import https://raw.githubusercontent.com/brickhouse-tech/sync-agents/main/examples/skills/code-review.md
sync-agents import https://raw.githubusercontent.com/brickhouse-tech/sync-agents/main/examples/workflows/pr-checklist.md
```

See [examples/README.md](examples/README.md) for the full list.

## Usage

```bash
# Initialize .agents/ structure in the current project
sync-agents init

# Add a new rule
sync-agents add rule no-eval

# Add a new skill
sync-agents add skill debugging

# Add a new workflow
sync-agents add workflow deploy

# Add a Claude subagent / a plan / a spec (creates the bucket on demand)
sync-agents add agent reviewer
sync-agents add plan q3-roadmap
sync-agents add spec sso-login

# Validate + upgrade skill frontmatter
sync-agents lint
sync-agents lint --fix

# Sync to all targets (.claude/ and .windsurf/)
sync-agents sync

# Sync to a specific target only
sync-agents sync --targets claude

# Preview sync without making changes
sync-agents sync --dry-run

# Force overwrite existing symlinks
sync-agents sync --force

# Check sync status
sync-agents status

# Regenerate the AGENTS.md index
sync-agents index

# Remove all synced symlinks
sync-agents clean

# Fix legacy layouts and broken symlinks
sync-agents fix

# Fix only skills (migrate + convert flat files + repair symlinks)
sync-agents fix skills

# Work in a different directory
sync-agents sync --dir /path/to/project
```
