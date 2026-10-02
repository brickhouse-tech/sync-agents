# `sync-agents lint [skills|all] [--fix]`

Validates every `.agents/skills/<dir>/SKILL.md` against Claude's
[skill authoring rules](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices)
and, with `--fix`, mechanically amends what's fixable (SPEC-004 Part E).

Claude discovers skills through their `SKILL.md` YAML frontmatter, with
published requirements: `name` (≤64 chars, lowercase
letters/numbers/hyphens, no reserved words) and `description`
(non-empty, ≤1024 chars, third person, says what the skill does *and
when to use it*). `lint --fix` amends what it can — injecting a missing
frontmatter block, deriving `name` from the directory, deriving
`description` from the first body paragraph, truncating overlong
values, and stripping XML tags — while preserving all other frontmatter
keys verbatim. Reserved-word names are reported but never auto-renamed.

## Checks

| Code | Rule | `--fix` action |
| ---- | ---- | -------------- |
| E001 | frontmatter block missing | inject one |
| E002 | `name` missing/empty | derive from dir name (slugified) |
| E003 | `name` has uppercase/invalid chars (must be `[a-z0-9-]`) | slugify |
| E004 | `name` > 64 chars | truncate at hyphen boundary |
| E005 | `name` contains a reserved word (`anthropic`, `claude`) | report only — rename the skill |
| E006 | `name` ≠ skill directory name | set to dir slug |
| E007 | `description` missing/empty | derive from first body paragraph, else TODO stub |
| E008 | `description` > 1024 chars | truncate |
| E009 | XML tags in `name`/`description` | strip |
| W101 | first-person description ("I can…", "You can…") | report only (third person required) |
| W102 | description lacks a when-to-use clause | report only |
| W103 | SKILL.md body > 500 lines | report only |
| W201 | `import: true` in the frontmatter of a plan, spec, or ADR | report only (see below) |

## Behavior

- Exits non-zero when unfixed E-level findings remain — wire it into CI.
  Warnings (W-level) never affect the exit code.
- `--fix` rewrites **only the frontmatter block**: the body is untouched
  and unknown frontmatter keys, comments, and nested YAML survive the
  round-trip verbatim. Writes are atomic; a second run reports clean
  (idempotent).
- Skill directories without a `SKILL.md` are treated as scratch dirs and
  skipped, matching artifact discovery.
- `lint` and `lint all` run every check. `lint skills` runs only the
  E0xx and W1xx skill checks.

## W201: `import: true` has no effect

Before SPEC-013, `import: true` in a plan, spec, or ADR added the doc to
a managed `@`-import block in `CLAUDE.md`, so Claude loaded it in every
session. SPEC-013 removed that block. The key is now inert: the doc is
still linked under `.claude/<bucket>/` and can be `@`-mentioned, but no
tool loads it on its own.

To make the content always-on, move it into a rule and delete the
`import:` line. A rule reaches every tool as content: Claude through
`.claude/rules/`, the other tools through `.agents/index/`. See
[delivery-channels.md](../architecture/delivery-channels.md).

```bash
git mv .agents/plans/rollout.md .agents/rules/rollout.md
sync-agents sync
```

`add rule <name> --from <path>` copies the doc instead of moving it.
`--link` refuses a source that is already inside `.agents/`.

The check walks `.agents/plans`, `.agents/specs`, and `.agents/adrs`
recursively, so ADR status directories and OS-scoped subdirectories are
covered. It never changes the exit code.

## Relationship to `index`

`sync-agents index` runs the same fix engine by default (the
"backfill") before it refreshes `.agents/index/`, but never fails on
unfixable findings. See [index.md](index.md). Use `lint` when you want
the strict, CI-gating report.
