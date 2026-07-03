# `sync-agents index [--no-fix]`

Regenerates `AGENTS.md` from the contents of `.agents/`.

## Sections

- **Rules / Skills / Workflows** — always present (with "none yet"
  placeholders when empty).
- **Agents / Plans / Specs** — appear only when the optional bucket has
  content. Plans and specs are listed **recursively**, so documents
  grouped per effort in subdirectories (`plans/auth-effort/rollout.md`)
  index correctly.
- **State** — `STATE_*.md` snapshots.
- **Inherits** — preserved verbatim across regenerations.
- **Managed Claude import block** — `@`-imports for passive rules (and
  explicitly-passive workflows) so Claude actually loads them; fully
  regenerated each run, removed when no passive rules remain.

Every entry renders as `- [name](path) — <description>` when the
artifact declares a frontmatter `description`. Scaffold TODO stubs and
multi-line YAML scalars are suppressed; display is truncated at 140
characters.

## Skill frontmatter backfill

Before indexing, `index` runs the [`lint --fix`](lint.md) engine over
the skills bucket: missing frontmatter blocks are injected, `name` is
derived from the skill directory, `description` from the first body
paragraph. Backfilled descriptions flow straight into the regenerated
index in the same run.

- Unfixable findings (reserved-word names, unterminated frontmatter)
  are warned about but **never fail indexing**.
- `--no-fix` skips the backfill entirely.
- The `watch` and `import` code paths regenerate the index **without**
  backfill, so nothing rewrites a file while you're editing it.
