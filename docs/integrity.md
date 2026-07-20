# Full-context integrity lock (`agents.lock` + `agents.sum`)

`sources.lock` records only *remotely-sourced* manifest entries.
Locally-authored rules, hand-written skills, imports, linked checkouts,
hooks, agents, plans, specs and ADRs have no provenance record and no
post-install integrity check. The integrity lock (SPEC-008) closes that
gap for the **entire `.agents/` tree**.

Two committed files live at the `.agents/` root:

- **`agents.lock`** — *provenance* (YAML). One entry per artifact:
  `path`, `bucket`, `name`, `origin`, resolved SHA (where one exists),
  and the SPEC-003 `tree_hash`. Answers *where did this come from?*
- **`agents.sum`** — *integrity* (go.sum-style text). One line per
  file, `<path> sha256:<hex>`, sorted and LF-joined. Answers *is it
  still exactly what we locked?* A symlink contributes a
  `<path> link:<target>` line so a re-pointed link is caught even when
  the target contents match.

`agents.sum` also sums `sources.yaml`, `sources.lock`, the `.agents/`
`config`, and `agents.lock` itself — so the sum file is the single root
of trust, protected by git review like `go.sum`.

## Origins

Every artifact carries one of four origins:

| origin | meaning |
|---|---|
| `local` | authored here — no manifest entry, no `_origin.json` |
| `source:<entry>` | pulled from a `sources.yaml` manifest entry (SPEC-003) |
| `linked:file:<path>` | a live local checkout (SPEC-007 linked source) |
| `imported:<url>` | a manual `import <url>` one-off |

## Commands

```bash
sync-agents lock [--global] [--dry-run] [--json]
```

Walks the tree, resolves each artifact's origin, computes hashes, and
writes both files (`agents.lock` first, then hashed into `agents.sum`).
Idempotent: an unchanged tree yields byte-identical files, and
`generated_at` only advances when some other byte changed. `--dry-run`
prints the added/removed/rehashed delta without writing.

```bash
sync-agents verify [--global] [--strict] [--json] [--explain <path>]
```

Re-walks the tree and compares it against `agents.lock` + `agents.sum`.

- **Exit codes:** `0` clean (INFO-only allowed), `1` on any
  WARN/ERROR, `2` on usage/IO error. No lock on disk ⇒ exit `0` with a
  `not locked` notice, so you can add `verify` to CI before adopting
  the lock.
- **Severity policy:** a local edit to a *pulled* (`source:` /
  `imported:`) artifact is an **ERROR** (supply-chain tamper signal); a
  new file that skipped every gate is an **ERROR**; a hand-edited
  `agents.lock` is an **ERROR** (caught via its sum line); drift in a
  **linked** checkout is **INFO** by default (that's what links are
  for) and escalates to ERROR under `--strict`; a swapped symlink is
  always an ERROR.
- `--json` emits `{status, findings, counts}` — a stable schema for a
  cron/CI drift sentinel.
- `--explain <path>` reports one artifact's origin, locked-vs-on-disk
  hashes, and a per-file match/modified breakdown.

## Security framing

SPEC-005 quarantine gates **entry**: nothing reaches the live tree
without a scan verdict and a human approve. The integrity lock gates
**residence**: from the moment of approval, every byte is pinned in
`agents.sum`, so post-install tampering — a malicious edit, an injected
file, a swapped symlink, or an artifact hand-copied past quarantine — is
caught by the next `verify`. A scheduled `sync-agents verify --json`
(daily cron or per-PR CI) is PLEX-style drift detection for the full
AI-context picture, with the git history of `agents.lock`/`agents.sum`
as the audit trail.

## CI / cron recipe

```yaml
# Fail the build if the AI context drifted from what was reviewed.
- run: sync-agents verify --json --strict
```

```bash
# Daily drift sentinel (cron): page a human only on unexplained drift.
sync-agents verify --json || notify "agents/ integrity drift detected"
```

## Rollout & scope

The feature is **inert until you run `sync-agents lock`** — with no
`agents.lock` on disk, `verify` is a friendly no-op. It ships additively
in the 1.x line.

**Implemented:** the `lock` and `verify` commands (including `--json`,
`--strict`, `--explain`), the origin/hash writer, coverage of every
registered bucket (including OS-scoped subdirs), and the exclusion
rules (`.quarantine/`, `.sources/`, unshared `STATE_*.md`, origin
metadata).

**Not yet automatic:** mutating commands (`pull`, `update`, `approve`,
`detach`, `add`, `import`, `source remove`) do **not** yet refresh the
lock as their final step — re-run `sync-agents lock` after a sync until
that integration lands. Tracked in the SPEC-008 issue.
