# ADRs (Architecture Decision Records)

How the `adrs/` bucket encodes decision status by subdirectory and keeps rejected decisions from being re-proposed.

## Status by subdirectory

ADRs live in `.agents/adrs/` with **status encoded by subdirectory**:
`proposed/`, `accepted/`, `denied/`. `add adr <name>` scaffolds into
`proposed/`; `sync-agents adr accept|deny|propose <name>` moves a
record between statuses (nested grouping subdirs are preserved).

## Where ADRs reach

ADRs are reference documents. Claude gets them through the
`.claude/adrs/` link and reads one when it is `@`-mentioned or relevant.
Other tools open `.agents/adrs/` when asked. No tool loads ADRs on its
own, and `import: true` has no effect (see [lint W201](./commands/lint.md)).

Denied records stay on disk so a rejected decision is not proposed
again. Before 2.0 the generated `AGENTS.md` told agents to check
`.agents/adrs/denied/`. To keep that instruction always-on now, put it
in a rule:

```markdown
Before proposing an ADR, check .agents/adrs/denied/ for a rejected
decision on the same topic.
```

## Usage

```bash
sync-agents add adr use-postgres      # → .agents/adrs/proposed/use-postgres.md
sync-agents adr accept use-postgres   # → .agents/adrs/accepted/
sync-agents adr deny use-postgres     # → .agents/adrs/denied/
```

## See also

- [Topology & configuration](./topology.md)
- [Semantic routing](./architecture/semantic-routing.md): the
  `reference` category
- SPEC-004 Part F (ADRs), SPEC-013 (AGENTS.md is not an index)
