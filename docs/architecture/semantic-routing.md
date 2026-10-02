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
| **`reference`** | Neither preloaded nor trigger-dispatched: read on demand (@-mention, file read). Used by the `plans/`, `specs/`, and `adrs/` buckets (SPEC-004 Part D). Never enters a delivery channel or any invocable surface; `import: true` has no effect (SPEC-013). |

These are independent of the bucket directory the artifact lives in.

## Why bucket ≠ semantic

Different AI tools treat the same directory name as a different
semantic:

| Bucket | Claude | Windsurf / Codeium | Cursor | Copilot / Codex |
|---|---|---|---|---|
| `rules/` | passive | passive (region in `memories/global_rules.md`) | passive (`.mdc` channel) | passive (instructions channel) |
| `skills/` | **invocable** | **passive** (auto-loaded as memory) | passive | passive |
| `workflows/` | passive (reference doc) | **invocable** (slash flow) | passive | passive |
| `agents/` | **invocable** (`agents/<name>.md` subagent) | — (skip: no subagent surface) | — (skip) | — (skip) |
| `plans/` | reference (`plans/<name>.md`) | — (skip: opened from `.agents/plans/` when asked) | — (skip) | — (skip) |
| `specs/` | reference (`specs/<name>.md`) | — (skip: opened from `.agents/specs/` when asked) | — (skip) | — (skip) |

Agents, plans, and specs route **independently of semantic** — the
per-tool destination is a property of the bucket (Claude-only), so a
frontmatter `invocable:` override never reroutes them into commands/
or a delivery channel. In particular, reference docs are never inlined
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

## What about Cursor, Copilot, Codex, and opencode?

These tools read aggregated rule text, not a rules directory of
per-artifact files that sync-agents can link. Passive content reaches
them through one generated file per tool: the delivery channel (see
[Delivery channels](./delivery-channels.md)). At user scope they have
no command surface that sync-agents writes, so invocable rules and
workflows are skipped. Codex, Cursor, opencode, and OpenClaw load
`~/.agents/skills` natively, so skills are not linked into their trees.

## Multi-file invocable skills

Windsurf workflows are single `.md` files. A Claude skill that's a
*directory* with supporting files cannot cleanly become a Windsurf
workflow. When `global sync` encounters one targeting codeium, it
prints a warning naming the skill and the reason, then skips that
target only. Claude still receives the skill, and the overall exit code
stays 0.

## Delivery channels

Passive content for Windsurf (user scope), Cursor, Copilot, Codex,
opencode, and OpenClaw is not linked per artifact. Each tool gets one
generated file in `.agents/index/` or `~/.agents/index/`, mounted as a
link, a marked region, or a config entry at the path the tool reads,
and sized to the tool's limit. Source artifacts stay authoritative; the
generated files are derived and rewritten only when their bytes change.
The full table is in [Delivery channels](./delivery-channels.md).

## See also

- SPEC-002 §Semantic categories (bucket ≠ semantic) (shipped; spec retired to git history)
- SPEC-002 §Requirement: Semantic-aware routing (shipped; spec retired to git history)
- [Scope and target directories](./scope-and-targets.md)
- [Delivery channels](./delivery-channels.md)
- SPEC-013 (delivery channels)
- `internal/agent/semantic.go` — `Semantic`, `BucketDefaultSemantic`,
  `ParseFrontmatterInvocable`, `ResolveSemantic`.
