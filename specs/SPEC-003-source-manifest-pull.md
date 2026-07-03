---
id: SPEC-003
title: "Source manifest & pull: install rules, skills, and workflows from upstream repos"
status: Implemented
owner: nmccready
created: 2026-05-14
updated: 2026-05-14
related: SPEC-001, SPEC-002
inspired_by: https://github.com/knoxgraeme/skillfish
---

# [SPEC-003] Feature: Source manifest & pull for upstream artifacts

## Overview

Add declarative, reproducible installation of `.agents/` artifacts (rules,
skills, workflows) from upstream GitHub repositories, modeled on
[skillfish](https://github.com/knoxgraeme/skillfish) but extended beyond
skills.

A project (or a user's global `~/.agents/`) declares its upstream sources
in `.agents/sources.yaml`. `sync-agents pull` resolves and fetches each
entry into the appropriate `.agents/{rules,skills,workflows}/` bucket;
`sync-agents update` bumps refs and re-fetches; per-artifact origin
metadata is written inline so a future operator can see exactly where each
file came from and at what commit.

## Motivation

`sync-agents` today has `sync-agents import <url>` — single-artifact, no
manifest, no version pinning, no inverse, no integrity. That works for
one-off grabs but does not support:

1. **Reproducible team setup.** A team-mate `git clone`s your project and
   wants identical rules/skills/workflows without copy-paste. Today they
   re-run `import` for each. With a manifest they run one command.
2. **Pinning + drift detection.** Today there is no record of which
   commit an imported skill came from. If upstream changes, you don't
   know. Skillfish's `.skillfish.json` solves this for skills; we need
   the same for the full artifact set.
3. **Multi-artifact pulls from one repo.** `import <url>` is one URL =
   one file. A "tap" repo that publishes 10 rules + 3 skills currently
   requires 13 separate import commands. A `tree:owner/repo` entry pulls
   the whole `.agents/`-shaped subtree in one go.
4. **Rules and workflows, not just skills.** skillfish is skills-only.
   In our model, rules and workflows are first-class artifacts with their
   own buckets and (after SPEC-002) their own routing semantics. Anything
   the source-manifest system does for skills, it must do for rules and
   workflows too.

## Goals

- `sync-agents source add <entry>` appends an entry to
  `.agents/sources.yaml` and pulls it.
- `sync-agents source remove <name>` removes an entry and deletes the
  pulled artifact (with confirmation).
- `sync-agents pull` fetches every entry into the matching bucket.
  Idempotent.
- `sync-agents update [name]` bumps tracked refs to latest and re-pulls.
- `sync-agents source list` shows declared entries + their resolved
  commit SHAs.
- `sync-agents source bundle` (inverse of pull) emits a `sources.yaml`
  reflecting whatever is already installed manually (origin recovered
  from per-artifact metadata when available, else prompts).
- Every pulled artifact carries `_origin.json` next to it recording
  `{owner, repo, path, ref, sha, fetched_at, source: 'manifest'|'manual'}`.
- Pulls use sparse tarball fetch over the GitHub API, not full clones —
  no `git` binary required.

## Non-Goals

- **Hosted registry / search** — skillfish has `skill.fish` and
  `skillfish search`. We don't build a registry. Direct repo URLs only.
- **Cross-host SCM** — GitHub only in this spec. GitLab/Bitbucket
  deferred (the tarball fetch abstraction should leave room).
- **Conflict-with-local merge** — if a pulled artifact would overwrite a
  locally-modified file, fail with diff. No three-way merge.
- **Signing / SBOM** — checksum integrity yes, signing deferred.
- **Auto-pull on `sync-agents sync`** — pull is its own command; sync
  doesn't fetch. Compose them in CI or hooks.

## Definitions

| Term | Meaning |
|---|---|
| **Source** | A declared upstream: a line in `sources.yaml`. |
| **Entry** | The string form of a source: `<type>:<owner>/<repo>[@<ref>][/<path>]`. |
| **Artifact** | A pulled file or directory under `.agents/{rules,skills,workflows}/`. |
| **Origin metadata** | The `_origin.json` (or `_origin.yaml`) file written next to a pulled artifact, recording where it came from. |
| **Lock** | The resolved commit SHA + content hash for each entry, written into `.agents/sources.lock`. |

## Manifest schema

`.agents/sources.yaml`:

```yaml
version: 1

# Each entry: "<type>:<owner>/<repo>[@<ref>][/<path>]"
# type ∈ {skill, rule, workflow, tree}
sources:
  - skill:anthropic/skill-pack@v1.2.0/skills/code-review
  - rule:my-org/agent-norms@main/rules/security.md
  - workflow:my-org/agent-norms@v0.4.1/workflows/release.md
  - tree:my-org/team-agents@v2.0.0
  # "tree:" pulls every subdir of an upstream .agents/ that the user has
  # not excluded.

# Optional: per-entry overrides (rare; keeps the inline form simple)
overrides:
  - match: "skill:anthropic/skill-pack@*/skills/code-review"
    rename: "claude-code-review"
    pin_sha: "abc123…"  # belt-and-suspenders pin
    exclude_paths:
      - "tests/"
```

The lock file `.agents/sources.lock`:

```yaml
version: 1
entries:
  - entry: "skill:anthropic/skill-pack@v1.2.0/skills/code-review"
    resolved_sha: "abc1234567890abcdef…"
    content_hash: "sha256:1f3a…"
    fetched_at: "2026-05-14T02:31:00Z"
  - …
```

Each installed artifact gets an `_origin.json` (next to `SKILL.md` for
skills; next to the `.md` for rules/workflows, named `<name>.origin.json`):

```json
{
  "schema": 1,
  "owner": "anthropic",
  "repo": "skill-pack",
  "path": "skills/code-review",
  "ref": "v1.2.0",
  "sha": "abc1234567890abcdef…",
  "content_hash": "sha256:1f3a…",
  "fetched_at": "2026-05-14T02:31:00Z",
  "source": "manifest"
}
```

`source: "manifest"` means this artifact is governed by `sources.yaml`.
`source: "manual"` means it was added by `sync-agents import` or hand
copied. `bundle` flips manuals to manifest entries.

## Requirements

### Requirement: Source entry grammar

The system SHALL accept entries of the form
`<type>:<owner>/<repo>[@<ref>][/<path>]` where:

- `<type>` ∈ {`skill`, `rule`, `workflow`, `tree`}
- `<owner>/<repo>` is a GitHub repo
- `<ref>` is a tag, branch, or 40-char commit SHA; defaults to the repo's
  default branch
- `<path>` is the path inside the repo to the artifact. Defaults differ
  by type:
  - `skill:` — defaults to repo root (expects `SKILL.md` at root) or
    `skills/<name>` if `<name>` matches a top-level dir
  - `rule:` — required for rule
  - `workflow:` — required for workflow
  - `tree:` — defaults to repo root; the puller walks for any subdirs
    named `rules/`, `skills/`, `workflows/`

#### Scenario: Skill with pinned tag

- **GIVEN** the entry `skill:foo/bar@v1.0.0/skills/grep-helper`
- **WHEN** `sync-agents pull` runs
- **THEN** `.agents/skills/grep-helper/SKILL.md` exists
- **AND** `.agents/skills/grep-helper/_origin.json` records
  `{owner: foo, repo: bar, ref: v1.0.0, sha: <resolved>}`

#### Scenario: Rule with required path

- **GIVEN** the entry `rule:foo/bar@main/rules/security.md`
- **WHEN** `sync-agents pull` runs
- **THEN** `.agents/rules/security.md` exists with file contents
- **AND** `.agents/rules/security.origin.json` records the source

#### Scenario: Tree pull populates multiple buckets

- **GIVEN** the entry `tree:foo/team-agents@v2.0.0`
- **AND** the upstream repo has `.agents/rules/r1.md`, `.agents/skills/s1/SKILL.md`, `.agents/workflows/w1.md`
- **WHEN** `sync-agents pull` runs
- **THEN** `.agents/rules/r1.md`, `.agents/skills/s1/SKILL.md`, and
  `.agents/workflows/w1.md` all exist locally
- **AND** each has its `_origin.json` / `.origin.json`
- **AND** files OUTSIDE the three buckets are ignored

#### Scenario: Invalid entry rejected with guidance

- **GIVEN** the entry `notatype:foo/bar`
- **WHEN** `sync-agents source add notatype:foo/bar` runs
- **THEN** the command exits non-zero
- **AND** stderr names the valid type prefixes

---

### Requirement: Pull command

`sync-agents pull` SHALL fetch every entry in `sources.yaml`, resolve
each `<ref>` to a commit SHA via the GitHub API, sparse-fetch the entry's
path as a tarball, extract to the matching bucket, and write
`_origin.json` + update `sources.lock`.

Conflict handling (per artifact):

- Destination does not exist → write.
- Destination exists and the existing `_origin.json` matches the entry's
  resolved SHA → no-op (idempotent).
- Destination exists, `_origin.json` matches but content hash differs
  from the recorded one → local modification detected; fail without
  `--force`, print diff.
- Destination exists, no `_origin.json` → manual conflict; fail without
  `--force`.

#### Scenario: Pull is idempotent

- **GIVEN** `sources.yaml` lists one entry that was already pulled at
  the resolved SHA
- **WHEN** `sync-agents pull` runs again
- **THEN** no filesystem writes occur (verified via mtime)
- **AND** exit code is 0
- **AND** stdout summarizes "1 entry already current"

#### Scenario: Local modification blocks unintentional overwrite

- **GIVEN** a previously-pulled `.agents/rules/security.md` has been
  edited locally
- **WHEN** `sync-agents pull` runs
- **THEN** the command exits non-zero
- **AND** stderr shows the diff between local content and upstream
  content at the recorded SHA
- **AND** suggests `--force` or `sync-agents source detach <name>` to
  convert to a manual artifact

#### Scenario: --dry-run plans without writing

- **GIVEN** `sources.yaml` lists three entries, two unchanged and one
  with a newer SHA available
- **WHEN** `sync-agents pull --dry-run` runs
- **THEN** stdout lists each entry's plan
  (`current`/`would-update`/`would-add`) and no filesystem changes
  occur

---

### Requirement: Update command

`sync-agents update [name]` SHALL re-resolve `<ref>` for one or all
entries and pull any whose SHA has advanced. Pinned SHAs (entries whose
`<ref>` is a 40-char SHA) are skipped with a note. Pinned tags resolve to
that tag's current SHA — if a tag has been moved upstream, that's a
detected update.

#### Scenario: Update bumps a tag-pinned entry

- **GIVEN** `sources.yaml` has `skill:foo/bar@v1.0.0/skills/x`
- **AND** the previous pull recorded SHA `aaa…` for `v1.0.0`
- **AND** upstream has retagged `v1.0.0` to point at `bbb…`
- **WHEN** `sync-agents update` runs
- **THEN** the artifact is re-fetched
- **AND** `sources.lock` records `bbb…`
- **AND** stdout calls out the moved tag

#### Scenario: Update is a no-op for SHA-pinned entries

- **GIVEN** `sources.yaml` has
  `skill:foo/bar@abc1234567890abcdef0123456789abcdef012345/skills/x`
- **WHEN** `sync-agents update` runs
- **THEN** the entry is skipped with `[skip] SHA-pinned`
- **AND** no network call beyond a HEAD check is made

---

### Requirement: Source list, add, remove, bundle, detach

- `sync-agents source add <entry>` — append entry to `sources.yaml`, run
  `pull` for it.
- `sync-agents source remove <name>` — remove from `sources.yaml`,
  delete pulled artifact (with `--keep` to leave the artifact as
  manual).
- `sync-agents source list` — print each entry, its resolved SHA, and
  whether the local artifact matches the lock (`[ok]` / `[outdated]` /
  `[modified]` / `[missing]`).
- `sync-agents source bundle` — scan `.agents/` for artifacts with
  `_origin.json` and emit/refresh `sources.yaml` from them. Artifacts
  without origin metadata prompt for a source URL or are skipped with a
  warning.
- `sync-agents source detach <name>` — flip an artifact's `_origin.json`
  to `source: "manual"` and remove its entry from `sources.yaml`. The
  artifact stays in `.agents/`.

#### Scenario: Add appends to manifest and pulls

- **GIVEN** `.agents/sources.yaml` has 1 entry
- **WHEN** `sync-agents source add rule:foo/bar@main/rules/sec.md` runs
- **THEN** `sources.yaml` has 2 entries
- **AND** `.agents/rules/sec.md` exists and has origin metadata

#### Scenario: Bundle picks up manual artifact with origin

- **GIVEN** `.agents/skills/cool/SKILL.md` was added via
  `sync-agents import` and has `_origin.json` with `source: "manual"`
- **WHEN** `sync-agents source bundle` runs
- **THEN** `sources.yaml` gains a corresponding entry
- **AND** the artifact's `_origin.json` flips `source` to `"manifest"`

---

### Requirement: Tarball fetch (no git binary required)

The puller SHALL fetch upstream content via the GitHub REST API's
tarball endpoint (`/repos/{owner}/{repo}/tarball/{ref}`), extract
selectively, and never shell out to `git`. This keeps the tool usable on
machines without `git`, in CI containers, and lets us cache by SHA.

GitHub auth resolution (matches skillfish):

1. `$SYNC_AGENTS_GITHUB_TOKEN`
2. `$GITHUB_TOKEN`
3. `$GH_TOKEN`
4. `gh auth token` if `gh` is on PATH

Anonymous requests are allowed; rate-limit errors hint at auth.

A local cache lives at `$XDG_CACHE_HOME/sync-agents/sources/<host>/<owner>/<repo>/<sha>.tar.gz`
(falls back to `~/.cache/sync-agents/...`). Re-pulls hit the cache by
SHA.

#### Scenario: Cache hit avoids the network

- **GIVEN** `sources.yaml` has an entry resolving to SHA `abc…`
- **AND** the cache holds `…/abc….tar.gz`
- **WHEN** `sync-agents pull --offline` runs
- **THEN** the artifact is extracted from cache, no network calls made
- **AND** if a needed SHA is not in cache, the command exits non-zero

---

### Requirement: Per-artifact origin metadata

Every pulled artifact SHALL have an origin file co-located with it:

- For a skill dir: `.agents/skills/<name>/_origin.json`
- For a rule file: `.agents/rules/<name>.origin.json`
- For a workflow file: `.agents/workflows/<name>.origin.json`

The schema is fixed (see "Manifest schema" above). Tools downstream
(SPEC-002 promote, the AGENTS.md indexer) MAY read these files but MUST
NOT depend on them — an artifact without origin is valid and untracked.

`.gitignore` is **not** updated by sync-agents. Origin files are
intended to be committed so a `git clone` of the project still records
provenance.

#### Scenario: Skill origin file format

- **GIVEN** a skill freshly pulled from `foo/bar@v1.0.0/skills/x` at
  SHA `abc…`
- **WHEN** `sync-agents pull` completes
- **THEN** `.agents/skills/x/_origin.json` is a valid JSON document
  with `schema: 1`, `owner: "foo"`, `repo: "bar"`, `ref: "v1.0.0"`,
  `sha: "abc…"`, `source: "manifest"`, and a UTC `fetched_at` timestamp

---

### Requirement: Integrity verification

Each pulled artifact's content hash (sha256 over the artifact's file
tree, deterministic) SHALL be recorded in both `_origin.json` and
`sources.lock`. On every `pull`/`update`, the puller re-hashes after
extraction and compares to `sources.lock`. Mismatch fails the operation
with a clear error citing both hashes; nothing under the destination is
written until the hash matches.

A future signing layer can compose with this; in this spec we only
guarantee content equivalence against what was recorded at first pull.

---

### Requirement: Interop with SPEC-002 (global)

All `source *` commands SHALL honor `--global-root` and
`$SYNC_AGENTS_GLOBAL_ROOT` (defined in SPEC-002). When invoked with
`--global`, they operate against `$HOME/.agents/sources.yaml` and pull
into `$HOME/.agents/`. Combined flow:

```bash
sync-agents source add --global skill:foo/bar@v1/skills/x
sync-agents global sync   # then fan out per SPEC-002 routing
```

#### Scenario: Pull into global root

- **GIVEN** `$SYNC_AGENTS_GLOBAL_ROOT=/tmp/g`
- **WHEN** `sync-agents pull --global` runs
- **THEN** every artifact lands under `/tmp/g/.agents/...`
- **AND** `$HOME/.agents/` is not touched

---

### Requirement: Pulled rules/workflows carry semantic frontmatter (SPEC-002 compat)

When pulling a rule or workflow that does not declare `invocable:` in its
frontmatter, the puller SHALL leave it unchanged — the SPEC-002
bucket-default resolution handles it. The puller MUST NOT inject
`invocable:` frontmatter into pulled files; provenance metadata lives
exclusively in `_origin.json` / `.origin.json`.

---

## Acceptance Criteria

- **AC-1**: `sync-agents source add <entry>` appends to `sources.yaml`
  and pulls the artifact into the correct bucket.
- **AC-2**: `sync-agents pull` is idempotent (no writes on second run
  with no upstream movement) and writes per-artifact origin files.
- **AC-3**: `sync-agents update` bumps tag-pinned entries when upstream
  moves the tag; skips SHA-pinned entries.
- **AC-4**: `sync-agents source remove <name>` removes the manifest
  entry and deletes the artifact unless `--keep` is passed.
- **AC-5**: `sync-agents source list` reports `[ok]` / `[outdated]` /
  `[modified]` / `[missing]` per entry.
- **AC-6**: `sync-agents source bundle` reconstructs `sources.yaml`
  from `_origin.json` files and flips `source` to `"manifest"`.
- **AC-7**: Tarball fetcher does not invoke `git`. Cache hits avoid
  network entirely.
- **AC-8**: Token resolution order matches skillfish:
  `SYNC_AGENTS_GITHUB_TOKEN > GITHUB_TOKEN > GH_TOKEN > gh auth token`.
- **AC-9**: Local modifications block `pull` without `--force`, with a
  clear diff.
- **AC-10**: Content-hash mismatch (tampered tarball, corrupted cache)
  aborts the operation before any destination writes.
- **AC-11**: `tree:` entries fan out to multiple buckets in one
  resolved tarball fetch.
- **AC-12**: `--global` (and `$SYNC_AGENTS_GLOBAL_ROOT`) reroute every
  operation under the global root; `$HOME` is untouched in tests.
- **AC-13**: `go test ./...` passes with ≥70% coverage on
  `internal/agent/source` (the new package).

## Technical Design

### Module layout

```
internal/agent/source/
  manifest.go      # parse/write sources.yaml, sources.lock
  entry.go         # entry grammar + parser
  origin.go        # _origin.json schema + read/write
  fetcher.go       # GitHub tarball fetch + cache
  hash.go          # deterministic content hash over a file tree
  pull.go          # pull/update orchestration
  bundle.go        # reverse: scan tree → sources.yaml
```

### Entry parser

```go
type EntryType int
const (
    EntrySkill EntryType = iota
    EntryRule
    EntryWorkflow
    EntryTree
)

type Entry struct {
    Type   EntryType
    Owner  string
    Repo   string
    Ref    string  // "" → repo default branch
    Path   string  // "" → root or type-specific default
    Raw    string  // original string
}

func ParseEntry(s string) (Entry, error)
```

### Fetcher

```go
type Fetcher interface {
    // ResolveRef returns the commit SHA for a tag/branch/SHA ref.
    ResolveRef(ctx context.Context, owner, repo, ref string) (sha string, err error)
    // Fetch returns a tarball reader for repo@sha, hitting the cache first.
    Fetch(ctx context.Context, owner, repo, sha string) (io.ReadCloser, fromCache bool, err error)
}

type GitHubFetcher struct {
    HTTPClient *http.Client
    CacheDir   string  // resolved $XDG_CACHE_HOME/sync-agents/sources
    TokenFn    func() (string, error)
}
```

`TokenFn` resolution: env var chain → `gh auth token` shellout (best-
effort; absence is non-fatal).

### Pull orchestration

```go
type PullOpts struct {
    Force    bool
    DryRun   bool
    Offline  bool
    Only     []string  // empty = all
    Global   bool
}

func (a *App) Pull(opts PullOpts) (PullReport, error)
```

`PullReport` enumerates per-entry result (`Added`, `Updated`, `Current`,
`Skipped`, `Failed`) for both human and `--json` output.

### CLI surface

```
sync-agents pull                           [--dry-run] [--offline] [--force] [--only NAME] [--global]
sync-agents update [NAME]                  [--dry-run] [--force] [--global]
sync-agents source add <ENTRY>             [--global]
sync-agents source remove <NAME>           [--keep] [--global]
sync-agents source list                    [--json] [--global]
sync-agents source bundle                  [--global]
sync-agents source detach <NAME>           [--global]
```

`import <url>` (existing) stays as a thin shim: parses `<url>`, calls
`source add` under the hood. Behavior identical for users who don't
care about the manifest.

### Manifest format choice

YAML for `sources.yaml` because the schema has nested overrides and
arrays-of-objects that JSON makes painful by hand. The lock file is
also YAML for consistency. Origin files are JSON because they're written
by code, never hand-edited, and JSON has better Go std-lib support for
deterministic emission.

### Hash function

`content_hash` = sha256 over a sorted, normalized manifest of the
extracted file tree:

```
sha256(
  for each file in sorted order:
    "{relative-path}\t{size}\t{sha256(content)}\n"
)
```

Excludes the `_origin.json` itself (would create a chicken-and-egg).
Deterministic across platforms (LF-only, no atime/mtime).

### Cache layout

```
$XDG_CACHE_HOME/sync-agents/sources/
  github.com/
    <owner>/
      <repo>/
        <sha>.tar.gz
        <sha>.json     # cached metadata (ref → sha resolution, ttl 1h)
```

Cache eviction is out of scope; document a `sync-agents source
cache prune` follow-up.

## Test Plan

### Go unit tests (`go test ./...`)

- [ ] `TestParseEntry_Skill`, `_Rule`, `_Workflow`, `_Tree` — each grammar form.
- [ ] `TestParseEntry_PinnedSHA` — 40-char ref recognised as SHA.
- [ ] `TestParseEntry_Invalid` — bad type prefix, missing required path.
- [ ] `TestContentHash_Deterministic` — same tree → same hash across runs.
- [ ] `TestContentHash_LineEndings` — CRLF vs LF normalized.
- [ ] `TestFetcher_CacheHit` — mock HTTP layer, second fetch hits cache.
- [ ] `TestFetcher_RefResolution` — tag, branch, SHA all resolve.
- [ ] `TestPull_Idempotent` — second pull writes nothing.
- [ ] `TestPull_DetectsLocalModification` — diff returned, no write.
- [ ] `TestPull_TreeFanout` — one tarball → multiple buckets populated.
- [ ] `TestUpdate_MovedTag` — SHA changes, artifact re-fetched.
- [ ] `TestUpdate_SHAPinnedSkipped` — no fetch.
- [ ] `TestBundle_RoundTrip` — manual artifact with origin → manifest
      entry; subsequent pull is a no-op.
- [ ] `TestTokenResolution_Order` — env vars short-circuit `gh` shellout.
- [ ] `TestGlobalRoot_Routing` — `--global` writes under global root only.
- [ ] `TestIntegrityMismatch_Aborts` — tampered tarball → no writes.

### Bats integration (with a fake GitHub via local file server)

- [ ] End-to-end pull → list → modify upstream → update → list shows
      `[outdated]` then `[ok]`.
- [ ] `source remove` deletes artifact unless `--keep`.
- [ ] `--offline` honors cache and errors on miss.

### Manual smoke

- [ ] Real GitHub pull of skillfish's own repo as a `tree:` entry,
      verifying our fetcher matches what `git archive` would produce.

## Rollout

1. New package `internal/agent/source/` lands in one PR with all
   commands + Go unit tests + bats fixtures.
2. README gains a "Source manifest" section under Commands.
3. CHANGELOG: `feat: declarative source manifest and pull command for
   upstream rules, skills, and workflows`.
4. Existing `sync-agents import <url>` becomes a thin shim around
   `source add` — no behavior change for current users.

### Suggested implementation order

1. `entry.go` + parser + tests.
2. `hash.go` + tests.
3. `fetcher.go` with mocked HTTP + cache + tests; then real GitHub.
4. `origin.go` + `manifest.go` parse/write + tests.
5. `pull.go` orchestration + idempotency + local-mod detection.
6. `update`, `bundle`, `detach`.
7. CLI wiring in `main.go`.
8. `--global` integration with SPEC-002 root resolver.
9. `import` shim refactor.

## Open Questions

- **Q1**: `sources.yaml` vs `sources.json` vs extending existing
  `.agents/config` (INI-ish) — picked YAML. Confirm.
- **Q2**: Per-artifact origin filename — `_origin.json` next to
  `SKILL.md` for skills, `<name>.origin.json` for files. Alternative:
  always under `.agents/.origins/<rel-path>.json` (out-of-band). Picked
  in-band for discoverability when reading a skill dir.
- **Q3**: `tree:` entry semantics — currently walks `.agents/{rules,
  skills,workflows}/` only. Should it also pick up `.agents/AGENTS.md`
  inheritance pointers? Defer.
- **Q4**: Should `pull` regenerate `AGENTS.md` automatically on success
  (mirroring SPEC-002 behavior)? Lean yes; confirm.
- **Q5**: Auth for private repos relies on the user's token. Should we
  surface a `sync-agents source whoami` debug subcommand to print which
  token source is being used? Lean yes, cheap.
- **Q6**: Compatibility with skillfish's `skillfish.json` — should
  `sync-agents source import-skillfish` import a `skillfish.json` into
  our `sources.yaml`? Trivial and friendly; recommend yes as a follow-up.

## Future Work

- **GitLab/Bitbucket fetchers** — `Fetcher` interface leaves room.
- **Cache prune** — `sync-agents source cache prune --older-than 30d`.
- **Registry integration** — could point at skill.fish's registry for
  discovery without depending on it for installation.
- **Skillfish import** — read `skillfish.json` and emit `sources.yaml`
  entries (with type `skill:` for each).
- **Signed sources** — `sigstore`/cosign verification before extraction.

## References

- skillfish: https://github.com/knoxgraeme/skillfish
- skillfish manifest schema (project-manifest.ts) — `skills: string[]`
  with `owner/repo[@ref][/path]` grammar.
- SPEC-001 — first-class `go install`.
- SPEC-002 — promote/global sync, semantic-aware routing.
