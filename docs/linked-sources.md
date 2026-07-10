# Linked (editable) sources

The `npm link` / `go mod replace` of `.agents/` — symlink an artifact at a live local checkout instead of a fetched snapshot, so edits flow both ways.

## Why

When you're **actively developing** a skill that lives in another repo
(or the upstream repo itself), a SHA-pinned snapshot forces a slow
`edit → commit → push → update` loop. A **linked** source instead
symlinks the artifact at a live local checkout, so edits flow both ways
and `git pull` in the checkout reaps updates with no re-fetch.

## Usage

```bash
# Link a checkout you already have (path relative to cwd or absolute):
sync-agents source add --link=../foo-skill skill:me/foo-skill

# Derive the entry from the checkout's github remote + layout:
sync-agents source add --link=../foo-skill

# Managed clone: sync-agents clones the repo under .agents/.sources/ and links out of it
# (the initial clone is scanned once through the quarantine scanner; --trust to skip):
sync-agents source add --link skill:me/foo-skill
```

## Declarative and committed

The link is recorded declaratively as a `link:` override in
`sources.yaml` and echoed in `sources.lock`, so the intent is committed
and reviewable:

```yaml
overrides:
  - match: skill:me/foo-skill
    link: file:../foo-skill      # relative to .agents/; mutually exclusive with pin_sha
```

All persisted paths are **relative** (`file:` scheme) and the on-disk
symlink is created relative too — an absolute path would break the
instant another machine cloned the repo, so absolute `file:` paths are
rejected at parse time.

## Behavior

- `pull` verifies (and self-heals) the symlink and never re-fetches a
  snapshot over it; a missing checkout surfaces as a warning, not a
  silent revert.
- `update` advances a **managed clone** with `git pull --ff-only` (a
  checkout you own is yours to drive).
- `source list` shows linked entries as
  `[linked] … link → file:../foo-skill`.
- `source detach` **freezes** the live copy into an ordinary vendored
  snapshot (materialize the files, drop the `link` override, keep the
  upstream identity).
- Because a linked source is a local working tree you own, it is
  trusted by default — only the first fetch of a managed clone is
  scanned.

## See also

- [Source manifest, lockfile & provenance](./sources.md)
- [Quarantine (remote content review)](./quarantine.md)
