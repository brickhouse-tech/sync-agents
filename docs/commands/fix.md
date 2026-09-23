# `sync-agents fix [type]`

Migrates legacy layouts into the canonical `.agents/` structure and repairs broken symlinks.

## Scenarios

The `fix` command handles three scenarios:

1. **Legacy directory migration** — Moves top-level `skills/`,
   `rules/`, or `workflows/` directories into `.agents/` and replaces
   them with symlinks.
2. **Flat skill conversion** — Converts `.agents/skills/name.md` flat
   files to the directory layout `.agents/skills/name/SKILL.md`.
3. **Symlink repair** — Recreates missing or broken symlinks in target
   directories (`.claude/`, `.windsurf/`, etc.) and the `CLAUDE.md`
   symlink.

## Symlink repair

Symlink repair uses the same per-bucket fold or drill rules as
[`sync`](./sync.md#fold-or-drill):

| What is at `<tool>/<bucket>` | What `fix` does |
|---|---|
| Nothing | Creates one symlink for the whole bucket (fold). |
| Symlink to `.agents/<bucket>` | Nothing. |
| Symlink that points elsewhere | Repoints it at `.agents/<bucket>`. No flag needed. |
| Real directory | Keeps it and links each artifact as `<tool>/<bucket>/<name>` (drill). Entries the tool owns are left alone and not reported. |
| Real entry `<tool>/<bucket>/<name>` shadowing an artifact, or a real file at the bucket path | Conflict. Warns and leaves it byte-identical. |

`fix` never deletes or moves a directory a tool created. To replace a
conflicting entry, pass `--overwrite`. It renames the entry to
`<path>.replaced-by-sync-agents` (or
`<path>.replaced-by-sync-agents.<unix-seconds>` if that name is taken)
and then links. Recover by moving the backup back, as described in
[Recovering a backup](./sync.md#recovering-a-backup).

## Flags

| Flag | Default | Effect |
|---|---|---|
| `--dry-run` | false | Print planned moves, conversions, and links without writing. |
| `--no-clobber` | false | During legacy migration, skip items that already exist in `.agents/` instead of merging over them. |
| `--overwrite` | false | During symlink repair, rename each conflicting entry to `<path>.replaced-by-sync-agents` and link in its place. |
| `--force` | false | **Deprecated on `fix`.** Prints a deprecation warning and behaves as `--overwrite`. Nothing is deleted. Will be removed from `fix` in a later release. |
| `--targets <list>` | `.agents/config` | Targets to repair. |

## Usage

```bash
# Fix everything (all types)
sync-agents fix

# Fix only skills
sync-agents fix skills

# Preview without changing anything
sync-agents fix --dry-run

# Don't overwrite items already in .agents/
sync-agents fix --no-clobber skills

# Move conflicting real entries aside and link in their place
sync-agents fix --overwrite
```

A reproducible demo is available in
[`examples/fix/`](../../examples/fix/):

```bash
bash examples/fix/run-demo.sh
```

## See also

- [`sync-agents sync`](./sync.md)
- [Topology & configuration](../topology.md)
- [Command reference](./README.md)
- [SPEC-010](../../specs/SPEC-010-conformance-audit-unified-sync.md) (fold/drill design)
