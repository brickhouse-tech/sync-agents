---
id: SPEC-017
title: "`wave` target"
status: 📝 draft
owner: nmccready
created: 2026-11-23
updated: 2026-11-23
related: SPEC-002, SPEC-010, SPEC-011, SPEC-013
---

# [SPEC-017] Feature: `wave` target

## Overview

Through 4.3.0, `wave` works only as an unregistered custom target. A
project `sync --targets wave` folds rules, skills, and workflows into
`.wave/`, but not agents, because the agents bucket goes only to
tools with a subagent surface. `global sync --targets wave` fails with
`unknown target "wave"; skipping`.

Wave uses the same layout as Claude Code. This spec registers `wave`
as a tool and routes it the way sync-agents routes Claude, minus the
features that are specific to Claude Code.

## Vendor surface (asserted by the owner, 2026-11-23)

| Surface | Project | User |
| --- | --- | --- |
| Rules (always on) | `.wave/rules/` | `~/.wave/rules/` |
| Skills | `.wave/skills/<name>/SKILL.md` | `~/.wave/skills/<name>/SKILL.md` |
| Subagents | `.wave/agents/<name>.md` | `~/.wave/agents/<name>.md` |
| Slash commands | `.wave/commands/<name>.md` | `~/.wave/commands/<name>.md` |

No vendor documentation was read for this table (Open question 1).

## Requirements (EARS)

- **E1.** The registry shall contain `wave` with local dir `.wave` and
  global dir `.wave`.
- **E2.** When local sync targets `wave`, it shall link the `rules`,
  `skills`, `workflows`, and `agents` buckets into `.wave/`, by fold
  or drill as SPEC-010 describes.
- **E3.** Local sync shall not give `wave` what only Claude Code reads:
  the `plans` and `specs` buckets, hooks merged into `settings.json`,
  and the `CLAUDE.md` policy.
- **E4.** When global sync targets `wave`, it shall route each artifact
  the way it routes Claude's, under `~/.wave`:
  - a passive rule or workflow to `rules/<name>.md`;
  - an invocable skill to `skills/<name>/SKILL.md`;
  - an invocable rule or workflow to `commands/<name>.md`;
  - an agent to `agents/<name>.md`;
  - a passive skill is skipped, as for Claude.
- **E5.** Global sync shall not give `wave` plans, specs, or hooks, and
  shall not version-gate it or edit a `CLAUDE.md` for it. `wave` has no
  delivery channel, because it reads its rules directory natively.
- **E6.** `global status` shall report each `wave` link and each
  orphaned link in `~/.wave/{rules,skills,commands,agents}`, and
  `global clean` shall remove them.
- **E7.** `wave` shall be opt-in. It is not in the default targets that
  `init` and `global init` write, and `global sync` without a
  `targets =` line does not create `~/.wave`. The generated configs
  name it among the available targets.
- **E8.** When `.wave/skills/` is a real directory that Wave created,
  local sync shall keep linking each artifact into it (drill), as it
  did when `wave` was unregistered.

One routing function serves Claude and Wave, parameterized by the
tool's directory, so the two cannot drift.

## Edge audit

- **OS-scoped rules.** Global sync links `macos/brew` as
  `~/.wave/rules/macos/brew.md`, nested, as for Claude. This works only
  if Wave reads `rules/` recursively like Claude does. Not verified.
- **Frontmatter dialect.** Agent and skill frontmatter passes through
  untranslated (SPEC-004 Part E). Wave is assumed to accept Claude's
  keys or ignore unknown ones.
- **`$HOME` project.** In `$HOME`, local sync folds `~/.wave/rules`
  while global sync links into the same directory, exactly as it does
  for `~/.claude`. Wave inherits Claude's behavior here instead of
  adding a `$HOME` special case.
- **Configs from older versions.** A project whose config already
  names `wave` now also gets `.wave/agents`. Nothing else changes for
  it.

## Open questions

1. The layout above is the owner's statement, not vendor documentation.
   Which product "Wave" is, and whether its docs agree, is not
   recorded here. Confirm against vendor docs or source before this
   spec ships.
2. Does Wave read `rules/` recursively (OS-scoped rules), and does it
   load `.agents/skills` or `~/.agents/skills` natively? If it loads
   them natively, linking skills into `.wave/skills` loads each one
   twice, and the skills bucket belongs in `skillsNative` instead.
3. Does Wave read an always-on file such as `AGENTS.md` or a
   `WAVE.md`? This spec delivers no such file.
