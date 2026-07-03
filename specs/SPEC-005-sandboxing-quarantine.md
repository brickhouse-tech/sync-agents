---
id: SPEC-005
title: "Supply-chain safety by default: fetch hardening, install quarantine + static scan, sandboxed skill exec"
status: Parts A+B Implemented (Part C future)
owner: nmccready
created: 2026-07-02
updated: 2026-07-02
related: SPEC-003, SPEC-004
---

# [SPEC-005] Feature: Sandboxing & install quarantine

## Overview

Make remote artifact installation safe **by default** on macOS and
Linux. Three layers, in dependency order:

1. **Fetch hardening** (Part A) — the download/extract path itself can
   neither execute content nor escape the target directory.
2. **Quarantine + static scan** (Part B) — remotely-fetched artifacts
   land in a holding area, get scanned, and require explicit approval
   before entering the live `.agents/` tree.
3. **Sandboxed exec wrapper** (Part C, stretch) — an opt-in runner that
   executes skill scripts under an OS sandbox (macOS `sandbox-exec`,
   Linux `bwrap`/Landlock) with a default-deny profile.

sync-agents does not itself execute skills — harnesses do — so the
highest-leverage safety work is at **install time** (Parts A + B).
Part C covers the residual risk for skills that ship scripts.

## Motivation

- `CmdImport` today shells out to `curl -fsSL <url> -o <dest>` and
  drops the result directly into the live tree: no integrity check, no
  review gate, no record of origin. One malicious or typo'd URL and a
  hostile rule/skill is in every synced harness on the machine.
- SPEC-003 adds manifest + lockfile + sha256 integrity (tamper
  detection), but integrity ≠ safety: a correctly-hashed artifact can
  still contain `curl | bash`, credential exfiltration, or prompt
  injection. Treat skill registries and "tap" repos as a hostile
  supply chain, like npm.
- Agent harnesses increasingly auto-load whatever is in their config
  dirs. The install pipeline is the last human checkpoint.

## Goals

- No code path in sync-agents ever executes fetched content.
- Remote installs (`import`, SPEC-003 `pull`/`update`) default to
  quarantine; promotion to the live tree is an explicit, logged action.
- `sync-agents scan` produces a human-readable findings report per
  artifact; findings recorded in the SPEC-003 lockfile on approve.
- Escape hatches exist (`--trust`, config default) but are loud.
- Part C runner works on macOS and Linux; other unixes get best-effort
  (no sandbox available → refuse unless `--unsandboxed`).

## Non-Goals

- Sandboxing the harnesses themselves (Claude/Cursor own their exec).
- Signature verification / sigstore — future; sha256 pinning only
  (SPEC-003).
- Windows sandboxing (AppContainer) — deferred.
- LLM-based semantic scanning — heuristics only in v1.

## Part A — Fetch hardening

- Replace the `curl` subprocess in `CmdImport` with native Go
  `net/http`: TLS verification on, redirects capped (5), https-only by
  default (`--insecure-http` to override), timeout + size cap
  (default 10 MiB/artifact, configurable).
- SPEC-003 tarball extraction: reject path traversal (`..`, absolute
  paths, symlinks pointing outside the extract root), reject
  hardlinks, strip setuid/setgid/exec bits on extract, entry-count and
  total-size caps (zip-bomb guard).
- Downloads land in a temp dir and move into place only after all
  checks pass (already SPEC-003's atomic-write requirement; restated
  here as a security invariant, not just a crash-safety one).

## Part B — Quarantine + static scan

### Flow

```
import/pull ──▶ .agents/.quarantine/<bucket>/<name>/   (+ _origin.json)
                      │
                      ▼
              sync-agents scan [name]     # automatic after fetch
                      │  findings report (stdout + .findings.json)
                      ▼
              sync-agents approve <name>  # human gate
                      │  moves into .agents/<bucket>/, records
                      ▼  scan verdict + content hash in lockfile
              live tree (synced to harnesses on next sync)
```

- Quarantined artifacts are **invisible to `sync`** — nothing under
  `.agents/.quarantine/` is ever symlinked, indexed, or imported.
- `sync-agents approve <name> [--all]` promotes; `sync-agents reject
  <name>` deletes with its findings.
- `--trust` on `import`/`pull` skips the gate but still runs the scan
  and prints findings (loud escape hatch, for CI bootstrap of
  already-reviewed manifests). `.agents/config` gains
  `quarantine = on|off` (default `on`) for teams that pin by sha and
  review in PRs instead.
- Local authoring (`add`, `promote`) is untouched — quarantine applies
  to remote content only.

### Scanner heuristics (v1)

Per-file findings with severity (info / warn / critical):

- Shell execution surface: shebang scripts in skill dirs, `Bash(`
  tool grants in frontmatter, `exec`/`eval`/`system` in embedded code.
- Network-then-execute: `curl … | sh`, `wget … | bash`, `iwr | iex`,
  base64-decode-then-exec chains.
- Exfiltration patterns: reads of `~/.ssh`, `~/.aws`, `.env`,
  `*_TOKEN`/`*_API_KEY` env references combined with network calls.
- Obfuscation: long base64/hex blobs, zero-width/bidi Unicode,
  homoglyph command names.
- Prompt-injection markers in markdown: "ignore previous
  instructions", hidden HTML comments with directives, instructions
  addressed to the agent to disable safety/review steps.
- Frontmatter anomalies: undeclared extra files in single-file
  buckets, `import: true` on remote reference docs (context
  preloading from an untrusted source).

Critical findings block `approve` unless `--force` (logged into the
lockfile entry as `approved_with_findings`).

## Part C — Sandboxed exec wrapper (stretch)

- `sync-agents run <skill> -- <cmd…>` executes a skill's script under:
  - macOS: `sandbox-exec` with a generated seatbelt profile.
  - Linux: `bwrap` (fallback: Landlock via `go-landlock`) — read-only
    project mount, writable scratch dir, no network.
- Default-deny: read project root + the skill dir, write a scratch
  dir, **no network**, no reads of `~/.ssh`/`~/.aws`/keychains.
- Skills loosen via declared frontmatter permissions, surfaced at
  scan/approve time so grants are reviewed, not discovered at runtime:

```yaml
permissions:
  network: ["api.github.com"]
  read: ["~/.config/tool/"]
  write: ["./output/"]
```

- Harness integration is out of scope; the wrapper exists so harness
  configs *can* route skill exec through it (documented recipe for
  Claude hooks: PreToolUse hook rewriting skill-script invocations).

## Backwards Compatibility

- `import` keeps its CLI shape; the only visible change is the
  quarantine step. `quarantine = off` restores v0.3.x behavior
  exactly.
- No new runtime dependencies for Parts A/B (pure Go + stdlib;
  scanner is regex/AST heuristics). Part C shells out to OS tools that
  are present by default (`sandbox-exec`) or clearly diagnosed when
  missing (`bwrap`).
- `.agents/.quarantine/` and `.findings.json` are gitignored via the
  existing `updateGitignore` mechanism.

## Test Plan

- Part A: traversal/symlink/hardlink/zip-bomb fixture tarballs → no
  writes outside root, loud errors; http→https redirect; size cap.
- Part B: fixture artifacts per heuristic class → expected findings;
  approve/reject state transitions; quarantined content never synced
  (run `sync`, assert absent); `--trust` still scans; idempotent
  re-pull of a quarantined name.
- Part C: sandboxed process cannot read a canary file outside the
  allow-list, cannot open a socket when network denied; permission
  frontmatter round-trips into the generated profile. CI: macOS +
  ubuntu runners.

## Rollout

1. PR 1 (Part A) — fetch hardening; lands independently, hardens
   existing `import` immediately.
2. PR 2+3 (Part B) — quarantine flow, then scanner heuristics.
   Sequenced after SPEC-003's pull pipeline (quarantine hooks into it).
3. PR 4 (Part C) — runner, macOS first, then Linux.
