# ADRs (Architecture Decision Records)

How the `adrs/` bucket encodes decision status by subdirectory and keeps rejected decisions from being re-proposed.

## Status by subdirectory

ADRs live in `.agents/adrs/` with **status encoded by subdirectory**:
`proposed/`, `accepted/`, `denied/`. `add adr <name>` scaffolds into
`proposed/`; `sync-agents adr accept|deny|propose <name>` moves a
record between statuses (nested grouping subdirs are preserved) and
regenerates the index.

## Indexing rules

Only **accepted and proposed** records appear in `AGENTS.md`. Denied
records are kept on disk and the index carries a standing note telling
agents to check `.agents/adrs/denied/` before proposing a new ADR — so
already-rejected decisions don't get re-proposed.

## Usage

```bash
sync-agents add adr use-postgres      # → .agents/adrs/proposed/use-postgres.md
sync-agents adr accept use-postgres   # → .agents/adrs/accepted/, reindexed
sync-agents adr deny use-postgres     # → .agents/adrs/denied/, dropped from index
```

## See also

- [Topology & configuration](./topology.md)
- [`sync-agents index`](./commands/index.md)
