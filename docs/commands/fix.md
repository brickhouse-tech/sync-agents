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
```

A reproducible demo is available in
[`examples/fix/`](../../examples/fix/):

```bash
bash examples/fix/run-demo.sh
```

## See also

- [Topology & configuration](../topology.md)
- [Command reference](./README.md)
