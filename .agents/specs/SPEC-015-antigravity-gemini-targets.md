---
id: SPEC-015
title: "`antigravity` and `gemini` targets, plus rule `trigger` lint"
status: 📝 draft
owner: nmccready
created: 2026-11-17
updated: 2026-11-17
related: SPEC-004, SPEC-013, SPEC-014
---

# [SPEC-015] Feature: `antigravity` and `gemini` targets

## Overview

Google moved consumer Gemini CLI users to Antigravity CLI on
2026-06-18; Gemini CLI keeps enterprise and paid-API users
(<https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli>).
Antigravity (App and CLI) reads the canonical `.agents/` tree
natively at project scope, so sync-agents' work there is validation,
not delivery. User scope needs writes under `~/.gemini/`.

Two targets, because the two products read different files:
`antigravity` (the default for new users) and `gemini` (Gemini CLI,
opt-in).

## Vendor surface (read 2026-11-17)

Antigravity: <https://antigravity.google/docs/rules>,
`/docs/skills`, `/docs/subagents`, `/docs/customization-discovery`.
Gemini CLI: <https://geminicli.com/docs/cli/gemini-md/>,
`/docs/cli/skills/`, `/docs/core/subagents/`.

| Surface | Antigravity project | Antigravity user | Gemini CLI project | Gemini CLI user |
| --- | --- | --- | --- | --- |
| Always-on | `AGENTS.md`, `.agents/AGENTS.md` (native) | `~/.gemini/AGENTS.md`, `~/.gemini/config/AGENTS.md` | `GEMINI.md` (`context.fileName` configurable) | `~/.gemini/GEMINI.md` |
| Rules | `.agents/rules/*.md` (native, flat, `trigger:` **required**) | `~/.gemini/config/rules/*.md` | none | none |
| Skills | `.agents/skills/` (native) | `~/.gemini/config/skills/` | `.agents/skills` (native alias) | `~/.agents/skills` (native alias) |
| Subagents | `.agents/agents/` (native) | `~/.gemini/config/agents/` | `.gemini/agents/*.md` | `~/.gemini/agents/*.md` |
| Workflows | deprecated, use skills | deprecated | `.gemini/commands/*.toml` | `~/.gemini/commands/*.toml` |

Limits: Antigravity truncates one instructions file past 24,000 bytes
and shares a 20,000-token budget across global plus `always_on`
content. A rule with missing or invalid frontmatter is **dropped with
no warning**. Symlink following is not documented for either tool.

## Requirements (EARS)

### Part A: `antigravity` target

- **A1.** The registry shall contain `antigravity` with no local dir
  and global dir `.gemini/config`. Local sync shall report
  `antigravity reads .agents/ natively` and write nothing.
- **A2.** When global sync targets `antigravity`, sync shall link each
  passive rule to `~/.gemini/config/rules/<name>.md`. Rules in
  subdirectories (OS-scoped) shall be linked flat, because Antigravity
  scans only the top level.
- **A3.** Skills shall link to `~/.gemini/config/skills/<name>/`
  (the App does not read `~/.agents/skills`).
- **A4.** The agents bucket shall link to `~/.gemini/config/agents/`.
- **A5.** Invocable single-file rules and workflows shall be skipped
  with reason "Antigravity workflows are deprecated; make it a skill".
- **A6.** `init` and `global init` default targets shall include
  `antigravity`.

### Part B: `gemini` target (opt-in)

- **B1.** The registry shall contain `gemini` with local dir `.gemini`
  and global dir `.gemini`. It shall not appear in default targets.
- **B2.** When global sync targets `gemini`, sync shall write the
  passive index into a `gemini-rules` region of `~/.gemini/GEMINI.md`,
  creating the file if absent.
- **B3.** When local sync targets `gemini`, sync shall write the
  passive index into a `gemini-rules` region of `GEMINI.md` at the
  project root only when `GEMINI.md` already exists; otherwise it
  shall report that Gemini CLI reads `AGENTS.md` only when
  `context.fileName` lists it, and write nothing.
- **B4.** Agents shall link to `.gemini/agents/` and
  `~/.gemini/agents/`. Skills shall be skipped (native alias).
- **B5.** Workflows shall be skipped with reason "TOML commands are
  not generated". (Converting Markdown to TOML is out of scope.)

### Part C: rule `trigger` lint

- **C1.** `lint` shall report **E010** for a rule in `.agents/rules/`
  whose frontmatter lacks `trigger:` or has a value outside
  `always_on | model_decision | glob | manual`.
- **C2.** `lint --fix` shall set `trigger: always_on` when the key is
  missing, preserving every other key. An invalid value is report
  only.
- **C3.** `lint` shall report **W104** for `trigger: model_decision`
  without `description`, and `trigger: glob` without `globs`.
- **C4.** `sync` targeting `antigravity` or `devin` shall print one
  warning listing rules that fail C1, since those tools drop them
  silently.

## Edge audit

- Four rules in Nick's `~/.agents/rules/` have no `trigger:` today
  (`confidence-audit`, `no-ai-commit-attribution`, `pstack-default`,
  `skill-install`). Antigravity drops them at both scopes. C2 fixes
  them with one `lint --fix`.
- Users targeting both `claude` and `antigravity` already get global
  rules twice only if Antigravity reads `~/.claude/` (not documented).
- Antigravity CLI also reads `~/.gemini/GEMINI.md`. A user targeting
  both `antigravity` and `gemini` would load global rules twice
  (rules dir plus the `gemini-rules` region). sync shall print a
  warning when both are targeted globally.
- Symlinked rules: not documented. If Antigravity does not follow
  them, A2 to A4 change to copies like SPEC-013's fallback.

## Open questions

1. **Symlinks.** Verify on a real install that Antigravity follows a
   symlinked rule and skill. Blocks A2 to A4 shipping as links.
2. **Budget.** Should the `gemini-rules` region carry a budget like
   OpenClaw's? No limit is documented for Gemini CLI, so this spec
   says no.
