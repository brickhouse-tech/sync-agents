# `sync-agents sync`

Links every bucket in the project's `.agents/` tree into each target tool directory, merging into directories a tool already created instead of replacing them.

## Synopsis

```text
sync-agents sync [--targets t1,t2] [--dry-run] [--overwrite] [--dir PATH]
```

## What it does

For every active target (from `--targets`, else `.agents/config`, else
`claude,windsurf,cursor,copilot`) and every bucket that exists under
`.agents/`, sync makes `<tool>/<bucket>` resolve to `.agents/<bucket>`.
A target name maps to `.<name>/` at the project root (`copilot` maps to
`.github/copilot/`), so a custom target such as `wave` syncs into
`.wave/`.

After the buckets, sync also:

1. Links `CLAUDE.md` to `AGENTS.md`.
2. Merges `.agents/hooks/*.json` into `.claude/settings.json` (when the
   `claude` target is active).
3. Updates `.gitignore`.

These steps run even when a bucket reported a conflict.

## Fold or drill

Sync picks one of two layouts per bucket, based on what is already at
`<tool>/<bucket>`:

- **Fold.** Nothing is there. Sync creates one symlink for the whole
  bucket, for example `.claude/skills -> ../.agents/skills`.
- **Drill.** A real directory is there (the tool, a plugin installer,
  or a person created it). Sync keeps the directory and links each
  artifact inside it, for example
  `.wave/skills/code-review -> ../../.agents/skills/code-review`.
  Entries the tool owns stay where they are.

The states below follow the ownership taxonomy in SPEC-010.

| State | What is on disk | What sync does |
|---|---|---|
| `missing` | Nothing at `<tool>/<bucket>`, or nothing at `<tool>/<bucket>/<name>` inside a drilled directory | Creates the symlink (fold for a bucket, entry link for a drilled directory) |
| `synced` | Symlink at the expected path that points at the canonical bucket or artifact | Nothing |
| `folded` | Artifact path resolves through the bucket symlink | Nothing. The bucket symlink covers every artifact in it. |
| `drifted` | Symlink at the expected path that points somewhere else | Repoints it at `.agents/`. No flag needed. |
| `conflict` | Real file or directory at `<tool>/<bucket>/<name>` for an artifact `.agents/` claims, or a real file where the bucket directory belongs | Warns and leaves it byte-identical. `--overwrite` moves it aside. |
| `foreign` | Entry in a drilled directory that no `.agents/` artifact claims | Nothing. Not reported. |

A conflict prints a warning of this form:

```text
[warn] conflict: <path> ... resync with --overwrite to move it aside
```

## Safety guarantee

- Sync never deletes, moves, or renames a directory a tool created.
- Sync never deletes a file or directory.
- The only move sync performs is under `--overwrite`. It renames one
  conflicting entry to `<path>.replaced-by-sync-agents`, then creates
  the symlink. If that name is taken, the backup is
  `<path>.replaced-by-sync-agents.<unix-seconds>`.
- Without `--overwrite`, a conflicting entry stays exactly as it was.

To recover a backup, see [Recovering a backup](#recovering-a-backup).

## Flags

| Flag | Default | Effect |
|---|---|---|
| `--targets <list>` | `.agents/config`, else `claude,windsurf,cursor,copilot` | Comma-separated targets to sync. |
| `--dry-run` | false | Print each planned link without writing. |
| `--overwrite` | false | Rename each conflicting entry to `<path>.replaced-by-sync-agents` and link in its place. Never deletes. |
| `--force` | false | **Deprecated on `sync`.** Prints a deprecation warning and behaves as `--overwrite`. Nothing is deleted. Will be removed from `sync` in a later release. |
| `-d`, `--dir <path>` | current directory | Project root. |

All five are persistent flags inherited from the root command.
`--force` keeps its own meaning on `add`, `approve`, `promote`, `pull`,
`update`, `source add`, and `global sync`.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | Every bucket and target synced with no conflicts. Last line is `Sync complete.` |
| non-zero | At least one conflict. Sync still processes every other bucket and target, merges hooks, and updates `.gitignore`. Last line is `Sync finished with N conflict(s); nothing was deleted`. |
| non-zero | `.agents/` does not exist. |

CI jobs and scripted installs can treat a non-zero exit as "a tool
directory needs attention", then rerun with `--overwrite` or resolve by
hand.

## Status output

`sync-agents status` reports one line per bucket per target:

| Line | Meaning |
|---|---|
| `[synced] <bucket> -> <link>` | Bucket is folded (one symlink). |
| `[merged] <bucket> (<linked>/<total> linked)` | Bucket is a real directory that holds sync-agents entry symlinks (drilled). |
| `[merged] <bucket> (<linked>/<total> linked, N conflict(s))` | Same, with N real entries shadowing claimed artifacts. |
| `[local] <bucket> (not symlinked)` | Real directory with no sync-agents entry symlinks. |
| `[missing] <bucket>` | Nothing at the bucket path. |

## Examples

### A tool already created its skills directory

The project targets `wave`. Wave created `.wave/skills/` and installed
its own `wave-native` skill there. `.agents/skills/` holds
`code-review` and `debugging`.

```bash
sync-agents sync --targets wave
```

```text
[info] Linked: .wave/skills/code-review -> ../../.agents/skills/code-review
[info] Linked: .wave/skills/debugging -> ../../.agents/skills/debugging
[info] Sync complete.
```

`.wave/skills/wave-native` is untouched and not reported. Status shows
the merge:

```text
.wave/
  [merged] skills (2/2 linked)
```

When the project root is `$HOME`, the same applies to `~/.wave/skills`.

### A real entry shadows a claimed artifact

Start from the same project, except Wave also created its own
`.wave/skills/debugging/` directory before the first sync.

```bash
sync-agents sync --targets wave; echo "exit=$?"
```

```text
[info] Linked: .wave/skills/code-review -> ../../.agents/skills/code-review
[warn] conflict: .wave/skills/debugging ... resync with --overwrite to move it aside
[info] Sync finished with 1 conflict(s); nothing was deleted
exit=1
```

```text
.wave/
  [merged] skills (1/2 linked, 1 conflict(s))
```

### Move the conflict aside

```bash
sync-agents sync --targets wave --overwrite
```

`.wave/skills/debugging` becomes
`.wave/skills/debugging.replaced-by-sync-agents`, and
`.wave/skills/debugging` becomes a symlink to
`../../.agents/skills/debugging`. The exit code is `0`.

## Recovering a backup

1. Remove the symlink sync created:

   ```bash
   rm .wave/skills/debugging
   ```

2. Move the backup back:

   ```bash
   mv .wave/skills/debugging.replaced-by-sync-agents .wave/skills/debugging
   ```

The next `sync` without `--overwrite` reports the entry as a conflict
again. To keep the tool's copy permanently, rename or remove the
artifact in `.agents/skills/`.

## Trade-offs

Drill creates one symlink per artifact instead of one per bucket.
Two costs follow:

- `status` lists a `[merged]` line with counts instead of one
  `[synced]` line, and the drilled directory holds many entries.
- Renaming or removing an artifact in `.agents/` leaves an orphan
  entry symlink in the drilled directory until `sync-agents clean`
  runs.

`global sync` already pays the same price. Fold stays the default for
any bucket path that is empty, so a pristine project still gets one
symlink per bucket.

## See also

- [`sync-agents fix`](./fix.md)
- [`sync-agents global sync`](./global-sync.md)
- [`sync-agents global clean`](./global-clean.md)
- [Command reference](./README.md)
- [Topology & configuration](../topology.md)
- [SPEC-010](../../specs/SPEC-010-conformance-audit-unified-sync.md) (ownership taxonomy, fold/drill design)
