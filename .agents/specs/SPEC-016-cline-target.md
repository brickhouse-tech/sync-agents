---
id: SPEC-016
title: "`cline` target"
status: 📝 draft
owner: nmccready
created: 2026-11-17
updated: 2026-11-17
related: SPEC-002, SPEC-013
---

# [SPEC-016] Feature: `cline` target

## Overview

Cline has the largest open-source IDE install base (5,964,041 VS Code
installs, marketplace, 2026-11-17) and is the named successor for Roo
Code users. It reads `AGENTS.md` and `.agents/skills` natively but not
`.agents/rules` or `.agents/workflows`. This spec adds a `cline`
target that delivers those two.

## Vendor surface (read 2026-11-17)

Docs: <https://docs.cline.bot/features/cline-rules>,
<https://docs.cline.bot/customization/skills.md>,
<https://docs.cline.bot/getting-started/config>. Source:
`cline/cline` `main`, `sdk/packages/shared/src/storage/paths.ts` and
`sdk/packages/core/src/extensions/config/user-instruction-config-loader.ts`.

| Surface | Project | User |
| --- | --- | --- |
| Always-on | `AGENTS.md` (root only, native) | `~/.agents/AGENTS.md` (native) |
| Rules | `.cline/rules/`, `.clinerules/` (deprecated in source) | `~/.cline/rules`, `~/Documents/Cline/Rules`, `~/Cline/Rules` |
| Skills | `.agents/skills`, `.cline/skills`, `.claude/skills` (native) | `~/.agents/skills`, `~/.cline/skills` (native) |
| Workflows | `.cline/workflows/`, `.clinerules/workflows/` | `~/.cline/workflows`, `~/Documents/Cline/Workflows` |
| Subagents | `.cline/agents/` (format undocumented) | `~/.cline/agents/` |

Loader facts from source: rule and workflow files end in `.md`,
`.markdown` or `.txt`; directories are scanned one level deep;
symlinks are followed (`stat`, not `lstat`) and broken links skipped.
Rule frontmatter supports only `paths:` (glob list). No frontmatter
means always on; `paths: []` means never. `CLINE_DIR` replaces
`~/.cline`.

## Requirements (EARS)

- **E1.** The registry shall contain `cline` with local dir `.cline`
  and global dir `.cline`, the global dir replaced by `$CLINE_DIR`
  when set (a `ResolveGlobalDir` like Codex's).
- **E2.** When sync targets `cline`, it shall fold
  `.cline/rules -> ../.agents/rules` at project scope and
  `~/.cline/rules -> ~/.agents/rules` at user scope.
- **E3.** Rules in OS-scoped subdirectories shall be linked flat into
  a real `.cline/rules/` directory instead of the fold, because Cline
  scans one level deep. (Same drill-versus-fold choice SPEC-010 makes
  for other tools.)
- **E4.** Workflows shall be folded `.cline/workflows ->
  ../.agents/workflows` at both scopes.
- **E5.** Skills shall be skipped at both scopes with reason
  "Cline loads .agents/skills natively" (`skillsNative`).
- **E6.** The agents bucket shall be skipped with reason "Cline's
  agents/ format is undocumented".
- **E7.** `cline` shall be opt-in: not added to `init` default
  targets, matching opencode (SPEC-011 Open Question 1).
- **E8.** When a project has a real `.clinerules/` directory, sync
  shall leave it alone and report that Cline reads both it and
  `.cline/rules`.

## Edge audit

- **Duplicate skills.** Cline reads `.claude/skills` and
  `.agents/skills`. A project targeting `claude` and `cline` makes
  every skill appear twice. Cline documents no dedupe beyond "global
  beats project". Report it in `status`; do not stop linking
  `.claude/skills` (Claude needs it).
- **`trigger:` frontmatter.** Canonical rules may carry `trigger:`
  for Devin and Antigravity (SPEC-015 C1). Cline documents only
  `paths:`; other keys are assumed ignored. Unverified.
- **`.windsurfrules` / `.cursorrules`.** Docs say Cline reads them;
  the SDK path resolver does not list them. Not sync-agents' concern
  since it writes neither.

## Open questions

1. Does the VS Code extension read the same paths as the SDK, or
   does `apps/vscode` add its own? Inferred same (SDK is the engine
   for CLI, VS Code and Desktop). Not traced.
2. How is a workflow's slash-command name derived, frontmatter `name`
   or file name? Affects whether `.agents/workflows/<file>.md` needs
   a `name:` key.
