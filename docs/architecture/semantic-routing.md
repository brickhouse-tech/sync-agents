# Semantic routing

Why an artifact's bucket (`rules/`, `skills/`, `workflows/`, `agents/`,
`plans/`, `specs/`) is not the same thing as its *behavioral category*,
and how `sync-agents` resolves the latter to route correctly into each
tool's per-semantic destination.

## The semantics

Every artifact has one of three semantics:

| Semantic | Meaning |
|---|---|
| **`invocable`** | Loaded only when triggered by name — a user slash command, or an AI model's tool-call decision based on the artifact's description. Not in baseline context. |
| **`passive`** | Always part of baseline context for every conversation. No trigger logic; always loaded. |
| **`reference`** | Neither preloaded nor trigger-dispatched: indexed in `AGENTS.md` and read on demand (@-mention, file read). Used by the `plans/` and `specs/` buckets (SPEC-004 Part D). Never enters the managed import block or any invocable surface. |

These are independent of the bucket directory the artifact lives in.

## Why bucket ≠ semantic

Different AI tools treat the same directory name as a different
semantic:

| Bucket | Claude | Windsurf / Codeium | Cursor | Copilot / Codex |
|---|---|---|---|---|
| `rules/` | passive | passive (concat → `memories/global_rules.md`) | passive | passive (concat → `instructions.md`) |
| `skills/` | **invocable** | **passive** (auto-loaded as memory) | passive | passive |
| `workflows/` | passive (reference doc) | **invocable** (slash flow) | passive | passive |
| `agents/` | **invocable** (`agents/<name>.md` subagent) | — (skip: no subagent surface) | — (skip) | — (skip) |
| `plans/` | reference (`plans/<name>.md`) | — (skip: read via AGENTS.md index) | — (skip) | — (skip) |
| `specs/` | reference (`specs/<name>.md`) | — (skip: read via AGENTS.md index) | — (skip) | — (skip) |

Agents, plans, and specs route **independently of semantic** — the
per-tool destination is a property of the bucket (Claude-only), so a
frontmatter `invocable:` override never reroutes them into commands/
or a concat file. In particular, reference docs are never concatenated
into always-on instruction files; that would preload reference
material into baseline context.

The conflicts are real and matter to users:

- A Claude **skill** is *invocable* — Claude loads it only when its
  trigger description matches the user's intent. Routing it into
  Windsurf's `skills/` would dump it into every Windsurf conversation
  as always-on context, polluting the prompt.
- A Windsurf **workflow** is *invocable* — the user types `/workflow`
  to load it. Routing it into Claude's `workflows/` makes it just a
  doc, never invoked.

`sync-agents` resolves this by routing on *semantic*, not bucket. A
Claude skill (invocable) lands in Windsurf's `global_workflows/`. A
Windsurf workflow (invocable) lands in Claude's `commands/`. The
authoring tool determines the semantic; the destination tool's
matching slot receives it.

## How resolution works

1. **Frontmatter wins**: an artifact may declare its semantic via YAML
   frontmatter:

   ```yaml
   ---
   invocable: true   # or false
   ---
   ```

   The only recognised values for `invocable:` are `true` and `false`
   (quoted forms `"true"` / `'false'` are also accepted). Anything
   else is a parse error so a typo (`invocable: yes`) doesn't silently
   flip routing.

2. **Bucket default**: if the frontmatter is absent or doesn't declare
   `invocable:`, the bucket determines the default:

   | Bucket | Default semantic |
   |---|---|
   | `rules/` | passive |
   | `skills/` | invocable |
   | `workflows/` | invocable |
   | `agents/` | invocable |
   | `plans/` | reference |
   | `specs/` | reference |

The defaults match each authoring tool's most common case — a rule is
usually always-on, a skill is usually triggered by name, a workflow is
usually slash-invoked.

## When to override

Most artifacts don't need an explicit `invocable:` declaration. Add
one when:

- You're writing a **long-form rule disguised as a skill** —
  `invocable: false` in the SKILL.md keeps it as always-on context
  even though it lives in `skills/`.
- You're writing an **invocable rule** — a rule that should only
  apply when explicitly requested (rare but real). `invocable: true`
  in the rule's frontmatter makes it routable into per-tool
  invocable slots.
- A **workflow is actually documentation** — `invocable: false`
  routes it to passive destinations so it stays as reference text
  rather than a slash command.

## What about Cursor / Copilot / Codex?

Cursor, Copilot, and Codex do not distinguish invocable from passive
at the filesystem level (Cursor uses a flat `rules/` directory;
Copilot and Codex merge everything into a single `instructions.md`).
The routing map still computes the destination by semantic for
consistency, but for these tools both semantics resolve to the same
path. No fan-out, no duplication.

## Multi-file invocable skills

Windsurf workflows are single `.md` files. A Claude skill that's a
*directory* with supporting files cannot cleanly become a Windsurf
workflow. When `global sync` encounters one targeting codeium, it
prints a warning naming the skill and the reason, then skips that
target only — the other tools (Claude, Cursor, Copilot, Codex) still
receive the skill normally and the overall exit code stays 0.

## Concat targets

Three destinations are *concat* rather than per-artifact symlinks:

- `~/.codeium/windsurf/memories/global_rules.md` — Windsurf's passive
  memory file. All passive artifacts targeting codeium are merged
  here, alphabetized by name, with `## <name>` headings and a
  generated-by banner.
- `~/.github/copilot/instructions.md` — Copilot's only file.
- `~/.codex/instructions.md` — Codex's only file.

These files are regenerated atomically (tmp file + rename) on every
`global sync` so partial writes can never leave a half-merged file.
Source artifacts in `~/.agents/rules/` stay authoritative; the concat
output is a derived artifact.

## See also

- [SPEC-002 §Semantic categories (bucket ≠ semantic)](../../specs/SPEC-002-promote-global-sync.md)
- [SPEC-002 §Requirement: Semantic-aware routing](../../specs/SPEC-002-promote-global-sync.md)
- [Scope and target directories](./scope-and-targets.md)
- `internal/agent/semantic.go` — `Semantic`, `BucketDefaultSemantic`,
  `ParseFrontmatterInvocable`, `ResolveSemantic`.
