# `sync-agents index [--no-fix]`

Regenerates the per-tool delivery files in `.agents/index/` from the
contents of `.agents/`. It does not write `AGENTS.md`.

## What it writes

One file per active tool that has a project channel:

| File | Tool | Read through |
|---|---|---|
| `.agents/index/cursor.mdc` | Cursor | `.cursor/rules/sync-agents.mdc` |
| `.agents/index/copilot.md` | Copilot | `.github/instructions/sync-agents.instructions.md` |
| `.agents/index/codex.md` | Codex | `AGENTS.override.md` |
| `.agents/index/opencode.md` | opencode | `opencode.json` `instructions` |

Each file carries the bodies of your passive rules, workflows, and
skills, in name order, inside the frame the tool's format needs. The
format, budget, and mount of each file are in
[delivery-channels.md](../architecture/delivery-channels.md). A file
whose rendered bytes equal the file on disk is not rewritten.

`index` refreshes only. It rewrites files inside `.agents/index/` and
re-splices regions and config entries that already exist. It does not
create a link, a region, or a config entry in a tool directory, and it
does not remove legacy placements. `sync` does that.

Claude and Windsurf have no file here: they read `.agents/rules`
through the `.claude/rules` and `.windsurf/rules` links.

## AGENTS.md

`AGENTS.md` is your file. `index` runs the one-time migration of a
1.x generated `AGENTS.md` (see [migration-v2.md](../migration-v2.md)),
then never writes it again. A file that is not generated, or is already
migrated, is left byte-identical.

The Codex index copies `AGENTS.md` (with every `sync-agents:*` region
removed), because Codex reads `AGENTS.override.md` instead of
`AGENTS.md`. After you edit `AGENTS.md`, run `index` or `sync`, or keep
`watch` running; until then `status` reports the Codex channel as
`stale`.

## Skill frontmatter backfill

Before regenerating, `index` runs the [`lint --fix`](lint.md) engine
over the skills bucket: missing frontmatter blocks are injected, `name`
is derived from the skill directory, `description` from the first body
paragraph.

- Unfixable findings (reserved-word names, unterminated frontmatter)
  are warned about but never fail `index`.
- `--no-fix` skips the backfill.
- `watch`, `add`, `adr`, `import`, and the source commands refresh the
  index without the backfill, so nothing rewrites a file while you are
  editing it.

## See also

- [delivery-channels.md](../architecture/delivery-channels.md): what
  each tool receives, budgets, and the index policy
- [`sync`](sync.md): mounts the index files and removes legacy links
- [`lint`](lint.md): the strict, CI-gating version of the backfill
- [migration-v2.md](../migration-v2.md)
- SPEC-013 (AGENTS.md is not an index)
