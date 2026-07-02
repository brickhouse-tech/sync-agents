# `sync-agents lint [skills] [--fix]`

Validates every `.agents/skills/<dir>/SKILL.md` against Claude's
[skill authoring rules](https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices)
and, with `--fix`, mechanically amends what's fixable (SPEC-004 Part E).

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

## Behavior

- Exits non-zero when unfixed E-level findings remain — wire it into CI.
  Warnings (W-level) never affect the exit code.
- `--fix` rewrites **only the frontmatter block**: the body is untouched
  and unknown frontmatter keys, comments, and nested YAML survive the
  round-trip verbatim. Writes are atomic; a second run reports clean
  (idempotent).
- Skill directories without a `SKILL.md` are treated as scratch dirs and
  skipped, matching artifact discovery.
- v1 lints the skills bucket; the check set is organized per-bucket so
  rules/workflows/agents can gain their own schemas incrementally.

## Relationship to `index`

`sync-agents index` runs the same fix engine by default (the
"backfill") before regenerating `AGENTS.md`, but never fails on
unfixable findings — see [index.md](index.md). Use `lint` when you want
the strict, CI-gating report.
