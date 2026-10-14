# Migrating to sync-agents 4.0

4.0 removes the `adrs/` bucket. ADRs record a project's design
decisions; they are not context an agent loads, so they are outside
sync-agents' scope. 3.x printed a deprecation warning for them.

## What is gone

- `sync-agents adr accept|deny|propose` (unknown command).
- `sync-agents add adr <name>` (unknown type).
- `.agents/adrs/` is no longer linked into `.claude/adrs/`, and lint
  W201 no longer walks it.

sync-agents never deletes `.agents/adrs/`. Your records stay where they
are until you move them.

## Do this after upgrading

1. Move the records into your repo docs:

   ```bash
   git mv .agents/adrs docs/adr
   ```

2. Run `sync-agents sync`. It removes the `.claude/adrs` link that 3.x
   created (only when it points at `.agents/adrs`).
3. If `~/.agents/adrs/` exists, move it out as well. Then run
   `sync-agents global status`: links 3.x left under `~/.claude/adrs/`
   show as `[orphaned]`, and `sync-agents global clean` followed by
   `sync-agents global sync` removes them.
4. If agents should check rejected decisions before proposing new ones,
   say so in a rule:

   ```markdown
   Before proposing an architecture decision, check docs/adr/denied/
   for a rejected decision on the same topic.
   ```
