---
id: SPEC-005
title: "Supply-chain safety by default: fetch hardening, install quarantine + static scan, sandboxed skill exec"
status: Parts A+B Shipped (v1.1.0–v1.2.0); Part C (sandboxed exec) open
owner: nmccready
created: 2026-07-02
updated: 2026-07-09 (rev 2: Parts A+B trimmed to a shipped summary; doc now tracks only the open Part C)
related: SPEC-003, SPEC-004
---

# [SPEC-005] Feature: Sandboxing & install quarantine

## Overview

Make remote artifact installation safe **by default** on macOS and
Linux. Three layers, in dependency order:

1. **Fetch hardening** (Part A) — the download/extract path itself can
   neither execute content nor escape the target directory. **Shipped.**
2. **Quarantine + static scan** (Part B) — remotely-fetched artifacts
   land in a holding area, get scanned, and require explicit approval
   before entering the live `.agents/` tree. **Shipped.**
3. **Sandboxed exec wrapper** (Part C, stretch) — an opt-in runner that
   executes skill scripts under an OS sandbox (macOS `sandbox-exec`,
   Linux `bwrap`/Landlock) with a default-deny profile. **Open.**

sync-agents does not itself execute skills — harnesses do — so the
highest-leverage safety work was at **install time** (Parts A + B).
Part C covers the residual risk for skills that ship scripts.

## Shipped (Parts A + B)

Landed in v1.1.0 (fetch hardening rode in with the SPEC-003 pull
pipeline, PR #62) and v1.2.0 (quarantine + scanner, PR #64, issue #52).
User-facing behavior is documented in
[docs/quarantine.md](../docs/quarantine.md); this list is the
requirements-level summary:

- **Fetch hardening:** native Go `net/http` (no `curl` subprocess),
  TLS verification, capped redirects, https-only by default, timeout +
  size caps; tarball extraction rejects path traversal, absolute
  paths, escaping symlinks, and hardlinks; strips exec/setuid bits;
  entry-count and total-size caps (zip-bomb guard); temp-dir download
  with atomic move-into-place.
- **Quarantine flow:** `pull`/`update`/`import` land remote content in
  `.agents/.quarantine/` (never synced, indexed, or imported);
  `quarantine` lists findings; `approve <name>|--all` promotes;
  `reject <name>|--all` deletes.
- **Static scanner (heuristic v1):** shell-execution surface,
  network-then-execute chains (`curl | bash` etc.), credential-read +
  network exfiltration combos, obfuscation (long base64/hex,
  zero-width/bidi Unicode), prompt-injection phrasing, frontmatter
  anomalies. Severity info/warn/critical.
- **Escape hatches, loud and audited:** critical findings block
  `approve` unless `--force` (recorded in `sources.lock` as
  `approved_with_findings`); `--trust` skips the gate for one
  invocation but still scans and prints; `quarantine = off` in
  `.agents/config` disables the gate. Local authoring (`add`,
  `promote`) is never quarantined.

## Open — Part C: Sandboxed exec wrapper (stretch)

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
- Other unixes get best-effort (no sandbox available → refuse unless
  `--unsandboxed`). Windows sandboxing (AppContainer) remains deferred.

### Part C test plan

- Sandboxed process cannot read a canary file outside the allow-list.
- Cannot open a socket when network is denied.
- Permission frontmatter round-trips into the generated profile.
- CI: macOS + ubuntu runners.

### Part C rollout

Single PR series: runner on macOS first, then Linux.

## Non-Goals (unchanged)

- Sandboxing the harnesses themselves (Claude/Cursor own their exec).
- Signature verification / sigstore — future; sha256 pinning only
  (SPEC-003, retired to git history).
- LLM-based semantic scanning — heuristics only in v1.
