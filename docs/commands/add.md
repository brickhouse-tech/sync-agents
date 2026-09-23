# `sync-agents add <type> <name>`

Create a new artifact in `.agents/` — from a template, or from an
artifact that already exists somewhere else.

## Scenarios

| You want to | Command |
| --- | --- |
| Start a new subagent from scratch | `sync-agents add agent reviewer` |
| Bring an existing definition under management | `sync-agents add agent reviewer --from ~/agents/reviewer.md` |
| Keep an existing definition where it lives | `sync-agents add agent tars --from ~/personas/tars.md --link` |
| Import a skill with its supporting files | `sync-agents add skill helper --from ~/skills/helper/` |

`<type>` is any registered bucket: `rule`, `skill`, `workflow`,
`agent`, `plan`, `spec`, `hook`, `adr`.

## Template mode (default)

With no flags, `add` writes the bucket's scaffold with `${NAME}`
substituted, creating the bucket directory if this is the first
artifact in it. Optional buckets (`agents/`, `plans/`, `specs/`,
`hooks/`, `adrs/`) activate exactly this way — see
[topology](../topology.md).

## Import mode (`--from`)

`--from <path>` seeds the artifact from an existing file or directory.
`~` is expanded; relative paths resolve against the working directory.

Two things are normalized on the way in, and nothing else:

- **`name:` is set to the name you imported as.** An artifact's name is
  its position in the tree, so a stale `name:` from wherever the file
  came from would leave it unaddressable. A rewrite is always reported.
- **`description:` must be present on an agent.** Every harness keys
  subagent delegation on it. A subagent without one installs cleanly
  and is then unreachable — a failure that surfaces nowhere later, so
  the import fails now instead.

Everything else survives byte-for-byte, including keys sync-agents does
not model: Claude's `tools:` and `model:`, Cursor's `readonly:` and
`is_background:`. Frontmatter is never translated between harnesses.

The source file is never modified.

For a directory-per-artifact bucket (`skill`), `--from` accepts either
the artifact directory or its `SKILL.md`. Given a directory, supporting
files are copied too — they are part of the artifact.

## Link mode (`--link`)

`--link` (which requires `--from`) makes the canonical path a symlink
instead of a copy. The source keeps ownership: edits there are live
everywhere the artifact syncs, and sync-agents never rewrites it.

Use it when the definition has another home that should stay
authoritative — a personas directory, a shared checkout outside the
repo.

Because link mode cannot rewrite the source, a `name:` that disagrees
with the name you are adding as is a **hard error** rather than a
warning. Copy mode can reconcile that; link mode could only propagate
it, leaving an agent that is silently unreachable in at least one
harness. Drop `--link`, or add it under the name its frontmatter
already declares.

The link is relative when the source is inside the project (so clones
stay portable) and absolute otherwise. A source already inside
`.agents/` is refused — that would be a cycle.

## Source references are not supported here

`--from team-agents:agents/critic.md` is recognized and rejected with a
pointer to the right route. Registered sources already deliver
artifacts into the bucket tree through `source add` + `pull`, and that
path applies the [quarantine](../quarantine.md) gate that makes
third-party content safe to install. A second import path would
duplicate the first while bypassing its safety check.

## Overwriting

An existing artifact at the canonical path is refused unless `--force`
is passed. This applies identically in all three modes. A dangling
symlink counts as existing — it is still something you put there.

## See also

- [Topology & configuration](../topology.md) — bucket layout and which
  buckets are optional
- [`sync-agents promote`](./promote.md) — move a project artifact up to
  user scope
- [Sources](../sources.md) and [quarantine](../quarantine.md) — the
  supported route for third-party artifacts
- `specs/SPEC-011` — import, selective load, and multi-tool subagent
  routing
