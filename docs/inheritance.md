# Inheritance

How rules from a team, an org, or your whole machine reach a project,
and what the `## Inherits` section in `AGENTS.md` does and does not do.

## What layers content

Two mechanisms deliver rule content from outside a project. Both put
the rule bodies where each tool reads them.

1. **The user-level tree, `~/.agents/`.** Rules here apply to every
   project on the machine. `sync-agents global sync` delivers them:
   Claude through `~/.claude/rules/`, the other tools through their
   user-level channels, and Cursor through each project's `cursor.mdc`.
   Copy a project rule there with [`promote`](commands/promote.md).
2. **Sources, `.agents/sources.yaml`.** A project declares rules,
   skills, or whole trees from another repo. `sync-agents pull` installs
   them SHA-pinned, and `sync` delivers them like local rules. A linked
   source keeps a live checkout authoritative. See
   [sources.md](sources.md) and [linked-sources.md](linked-sources.md).

For an org or team layer, keep the shared rules in a repo and declare
it as a source in each project, or link it with a linked source.

```bash
sync-agents promote rule security          # project rule -> ~/.agents/rules/
sync-agents global sync                    # deliver ~/.agents/ to every tool

sync-agents source add rule:myorg/agent-rules@v2/rules/go-standards
sync-agents pull
sync-agents sync
```

## `## Inherits` is plain text

Before 2.0, `AGENTS.md` was generated, `sync-agents inherit` managed an
`## Inherits` list of links to parent `AGENTS.md` files, and the
generator preserved that section. The links delivered nothing: no tool
follows Markdown links in `AGENTS.md`.

- Codex, opencode, and OpenClaw read `AGENTS.md` as plain text.
- Claude does not follow links. It does expand `@path` imports in an
  `AGENTS.md` it reads natively (Claude Code docs, "AGENTS.md").

In 2.0:

- `AGENTS.md` is your file. The migration keeps an existing
  `## Inherits` section byte for byte (see
  [migration-v2.md](migration-v2.md)).
- The `inherit` command is removed. Running it prints a pointer here.
- An `## Inherits` section is ordinary text. Edit it by hand, keep it as
  a note for humans, or delete it.

If you want Claude to load a parent file, write an `@` import instead of
a link. This works for Claude only:

```markdown
## Inherits

@../AGENTS.md
```

## Why upward links did not work

A Markdown link is a pointer an agent may or may not open. A rule that
must apply in every session has to be in a file the tool loads. That is
what the two mechanisms above do: they put the rule bodies into each
tool's native read path.

## See also

- [`promote`](commands/promote.md) and
  [`global sync`](commands/global-sync.md)
- [sources.md](sources.md) and [linked-sources.md](linked-sources.md)
- [delivery-channels.md](architecture/delivery-channels.md)
- [Global root resolution](architecture/global-root-resolution.md)
- SPEC-013 (AGENTS.md is not an index), SPEC-002 (global scope),
  SPEC-003 (sources)
