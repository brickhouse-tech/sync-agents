---
id: SPEC-007
title: "Linked (editable) sources: npm-link / go-replace for .agents artifacts"
status: Implemented
owner: nmccready
created: 2026-07-06
updated: 2026-07-07
related: SPEC-003, SPEC-005
---

# [SPEC-007] Feature: Linked (editable) sources

## Overview

Add an **editable / linked** source mode to the SPEC-003 source system,
modeled on `npm link` and `go mod replace`. Today a source entry resolves
to an immutable, SHA-pinned tarball snapshot fetched over HTTP — a one-way
copy. A **linked** source instead points a managed artifact in `.agents/`
at a **live local git checkout** via a relative-path symlink, so:

1. **Edits flow both ways.** The artifact and the upstream working copy are
   the same inode; editing either edits both.
2. **The local copy reaps upstream updates** by `git pull` in the checkout
   (or a managed clone), with no re-fetch/re-stage step.
3. **Changes can be paid forward upstream** by branching / committing / PRing
   from that same checkout.

Linking is recorded declaratively in `sources.yaml` and `sources.lock` so
the intent is committed, reviewable, and reproducible — replacing today's
undocumented manual `ln -s`.

## Motivation

The current model is snapshot-only. When you are *actively developing* a
skill that lives in another repo (or developing the upstream repo itself and
consuming it here), the SPEC-003 flow forces a painful loop:
`edit upstream → commit → push → sync-agents update → test → repeat`.

Developers work around this with a hand-rolled `ln -s`, which is:

- **Invisible** — not in `sources.yaml`/`sources.lock`, so `pull`/`update`
  and the AGENTS.md index don't know the artifact is a link; a later `pull`
  may clobber it.
- **Non-portable** — an absolute symlink or lock path (`/Users/tars/...`)
  breaks the instant the repo is cloned on another machine or by another
  engineer. `sources.lock` is git-tracked, so this is a correctness bug, not
  just an annoyance.
- **One-off** — no lifecycle: no `list` visibility, no clean `detach`, no
  re-pin back to a remote snapshot.

`npm link` and `go mod replace` exist for exactly this. We adopt the
`replace` model: keep the upstream identity entry, redirect resolution to a
**relative** local path.

## Goals

- `sync-agents source add --link <path> <entry>` declares a linked source:
  writes the upstream `<entry>` to `sources.yaml` **and** an override
  redirecting it to a relative `file:` path, then wires the symlink.
- Bare `sync-agents source add --link <entry>` (no path) **manages a clone**:
  clones the upstream repo into `.agents/.sources/<owner>-<repo>/`, then links
  the artifact out of it. `update` runs `git pull` in that clone.
- `sync-agents source add --link <path>` (path, no entry) derives
  `owner/repo` (and type/in-repo path) from the checkout's git remote +
  layout.
- `pull` / `update` **skip the HTTP fetcher** for linked entries (nothing to
  download); `update` optionally `git pull`s a managed clone.
- `index` **follows the symlink** so AGENTS.md still enumerates the artifact.
- `source list` marks linked entries (`link → ../foo-skill`).
- `detach` converts a link into a real vendored copy (materialize the files,
  drop the override) — the inverse of linking.
- All persisted paths are **relative** (`file:` scheme); the on-disk symlink
  is created **relative** too. Absolute paths are rejected at parse time.

## Non-goals

- **Watching / copy-on-change.** We symlink; we do not run a file watcher.
  Symlink *is* the bidirectional link (same as `npm link`).
- **Cross-VCS.** Managed clones are git-only in v1.
- **Automatic pay-forward.** Committing/pushing/PRing upstream stays a manual
  git action in the checkout — we do not wrap `git push`/`gh pr`.
- **Re-running SPEC-005 quarantine per sync.** See Security.

## Path grammar & the `file:` scheme

Linked paths use npm's `file:` convention, resolved **relative to the
`.agents/` directory** (the `sources.yaml` location):

```
file:../foo-skill          # sibling of the repo root (common: checkout in ~/code)
file:./vendor/foo-skill    # inside the project
```

- **Absolute paths are a parse error** (`file:/Users/...`, `file:/abs`) — they
  do not round-trip through git. Error names the fix (use a `file:../` relative
  path).
- The literal after `file:` is `path.Clean`ed and must stay a relative path
  (may traverse up with `..`).
- Portability across machines requires the same sibling layout on each — this
  is inherent to relative links and shared with `go mod replace`. Documented,
  not worked around.

## Manifest & lock schema

Linking reuses the existing `Override` mechanism (`match` glob), adding one
field — a link is conceptually "pin to a local path instead of a SHA":

`sources.yaml`
```yaml
version: 1
sources:
  - skill:nmccready/foo-skill          # upstream identity — unchanged
overrides:
  - match: skill:nmccready/foo-skill*
    link: file:../foo-skill            # relative to .agents/; mutually exclusive with pin_sha
```

`sources.lock`
```yaml
version: 1
entries:
  - entry: skill:nmccready/foo-skill
    link: file:../foo-skill            # relative, echoing the override
    resolved_sha: 9f3a…                # HEAD of the checkout at link time — INFORMATIONAL only
    content_hash: ""                   # empty: a live checkout has no stable content hash
    fetched_at: 2026-07-06T23:00:00Z
    managed_clone: false               # true when sync-agents owns the clone under .agents/.sources/
```

Schema changes:

- `Override.Link string` (yaml `link,omitempty`). Mutually exclusive with
  `PinSHA` — validated on load (a link cannot also be SHA-pinned).
- `LockEntry.Link string`, `LockEntry.ManagedClone bool`. `ResolvedSHA` for a
  linked entry is informational (records provenance at link time; not used for
  drift/verification). `ContentHash` is empty for links.

## Behavior

### `source add --link`

| invocation | behavior |
|---|---|
| `--link <path> <entry>` | write `<entry>` to `sources`; write override `link: file:<rel>` (path made relative to `.agents/`); create relative symlink `.agents/<bucket>/<name> → <path>/<in-repo-path>`; write lock. |
| `--link <entry>` (no path) | clone `<owner>/<repo>` → `.agents/.sources/<owner>-<repo>/`; link artifact out; `managed_clone: true`. |
| `--link <path>` (no entry) | read `<path>/.git` remote to derive `owner/repo`; infer type + in-repo path from layout; proceed as row 1. |

`--link` refuses if an absolute path is supplied. If the target symlink
already exists as a real dir/file, refuse unless `--force`.

### `pull`

For each entry: if it has a `link` override (or a `link` lock record),
**skip the fetcher entirely**. Verify the symlink exists and resolves; if the
target is missing (checkout moved/deleted), emit a clear warning and leave the
dangling link (do not silently re-fetch a remote snapshot — that would mask a
broken dev setup). Non-linked entries: unchanged SPEC-003 path.

### `update`

Linked, `managed_clone: true`: `git -C .agents/.sources/<owner>-<repo> pull
--ff-only`; refresh `resolved_sha` in lock (informational). Linked,
`managed_clone: false` (user checkout): **no-op** — the user drives their own
git; we only refresh the informational `resolved_sha` from current HEAD.
Non-linked: unchanged.

### `index`

`DiscoverArtifacts` must `filepath.EvalSymlinks` / stat-through the link so a
linked skill's `SKILL.md` is read and enumerated in AGENTS.md exactly like a
vendored one. (Today it walks real files; confirm symlinked dirs are
followed.)

### `source list`

Linked entries render with a `link → file:../foo-skill` annotation and, for
managed clones, the clone's short SHA. Snapshot entries unchanged.

### `detach`

Extend existing `detach` to handle links: replace the symlink with a real
copy of the current target contents, remove the `link` override, and record a
normal snapshot lock entry (content-hash the materialized files). This is the
"freeze my linked dev copy into a vendored artifact" operation.

## Security (SPEC-005 interaction)

A linked source is a **local checkout the operator is actively developing** —
running the quarantine scanner and hardened-extraction gate on every `pull`
is noise, and there is no tarball to extract. Therefore:

- **Linking a local `--link <path>` is trusted by default** — no quarantine,
  no scan on link or on subsequent `pull`.
- **Managed-clone linking (`--link <entry>`) fetches remote code**, so the
  **initial clone is scanned once** through the SPEC-005 scanner (same
  heuristics as tarball pull). Findings gate the link unless `--trust`.
  Subsequent `git pull`s during `update` are **not** re-scanned (the operator
  now owns the checkout) — but `sync-agents check`/scan can be run on demand.
- Rationale: matches the [ClawHub supply-chain] posture — untrusted *remote
  first contact* is scanned; an operator's own working tree is not policed.

## Test plan (bats + Go)

- Parse: `file:../x` ok; `file:/abs` rejected; `file:./x` ok; empty rejected.
- `Override.Link` + `PinSHA` together → load error.
- `add --link <path> <entry>`: symlink created relative; manifest+lock written
  relative; re-`pull` is a no-op and does not clobber the link.
- `add --link <entry>`: managed clone created + scanned; `managed_clone: true`.
- Dangling target → `pull` warns, does not re-fetch.
- `index`: linked skill appears in AGENTS.md.
- `detach`: link → real copy, override removed, content_hash populated.
- Portability: lock/symlink contain no absolute paths (grep gate in test).

## Rollout

Additive, backwards-compatible: no `link` override ⇒ identical to today.
Ships in 1.x (per the standing no-2.0 decision). New `.agents/.sources/`
dot-dir is invisible to sync/index (same convention as `.quarantine`).
