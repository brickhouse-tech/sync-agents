---
id: SPEC-014
title: "`devin` target replaces `windsurf`/`codeium` (deprecate in 4.x, remove in 5.0)"
status: 📝 draft
owner: nmccready
created: 2026-11-17
updated: 2026-11-17  # rev 2: duplicate-load and global-path decisions
related: SPEC-002, SPEC-011, SPEC-013
---

# [SPEC-014] Feature: `devin` target replaces `windsurf`/`codeium`

## Overview

On 2026-06-02 Windsurf became Devin Desktop. Its agent is now Devin
Local, which shares a harness with Devin CLI. Cascade, the agent that
read every path sync-agents writes for `codeium` today, is legacy and
starts no new conversations. Source:
<https://docs.devin.ai/desktop/devin-desktop-faq.md>,
<https://docs.devin.ai/desktop/cascade.md>.

This spec adds a `devin` tool, deprecates `windsurf` and `codeium` as
aliases for it in a 4.x minor, and removes them, with cleanup of what
they wrote, in 5.0.0.

## Vendor surface (docs.devin.ai, read 2026-11-17)

| Surface | Project | User |
| --- | --- | --- |
| Always-on instructions | root `AGENTS.md` (also `CLAUDE.md`) | `~/.config/devin/AGENTS.md` (CLI doc); `~/.devin/global_rules.md` (Local doc) |
| Rules | `.devin/rules/*.md`, `trigger:` frontmatter; Cursor `alwaysApply` accepted | `~/.devin/rules/*.md` (Local doc) |
| Skills | `.devin/skills/`, **`.agents/skills/`** | `~/.config/devin/skills/`, **`~/.agents/skills/`** |
| Subagents | `.devin/agents/`, **`.agents/agents/`** | `~/.config/devin/agents/` |
| Workflows | not imported by Devin Local/CLI | not imported |
| Legacy fallback | `.windsurf/rules`, `.windsurf/skills` imported by default | `~/.codeium/<channel>/skills` imported |

No character limit is documented for Devin Local rules. The 6,000
character `global_rules.md` cap was Windsurf's.

## Requirements (EARS)

### Part A: `devin` tool (4.x minor)

- **E1.** The registry shall contain a tool `devin` with local dir
  `.devin` and global dir `.config/devin`.
- **E2.** When local sync targets `devin`, sync shall fold
  `.devin/rules -> ../.agents/rules`, the same way it folds
  `.claude/rules`.
- **E2a.** While `.windsurf/rules` serves rules from `.agents/rules`,
  sync shall skip the `.devin/rules` fold (Devin imports
  `.windsurf/rules` by default, so both would load every rule twice).
  It serves them when it is a symlink that resolves into
  `.agents/rules` (the fold), reported as
  `devin rules served by legacy .windsurf/rules link`, or a real
  directory holding at least one symlink that resolves into
  `.agents/rules` (the per-rule links an earlier sync drilled into a
  directory Windsurf created), reported as
  `devin rules served by legacy links in .windsurf/rules`.
- **E2b.** While the project's `.agents/` is the global root (the
  project is `$HOME`), local sync shall skip the `.devin/rules` fold
  and report `devin rules: skipped at project scope`, with the same
  reason sync prints for a channel it defers to global sync.
  `~/.devin/rules` is Devin's user rules directory, and E3 already
  delivers the same rules through `~/.config/devin/AGENTS.md`.
- **E3.** When global sync targets `devin`, sync shall write the
  passive index into a `devin-rules` region of
  `~/.config/devin/AGENTS.md`, creating the file if absent, with no
  character budget.
- **E4.** Skills shall be skipped for `devin` at both scopes with
  reason "Devin loads .agents/skills natively" (`skillsNative`).
- **E5.** The agents bucket shall route to `~/.config/devin/agents/`
  at global scope and be skipped at local scope (Devin reads
  `.agents/agents` natively).
- **E6.** Invocable single-file rules and workflows shall be skipped
  for `devin` with reason "Devin has no workflow surface; make it a
  skill".
- **E6a.** Local `clean` shall remove under `.devin/` only the
  `.devin/rules` link, the one link sync places there, and for every
  tool shall remove a `<tool>/<bucket>` symlink only when it resolves
  into `.agents/`. Any other link is the user's.
- **E7.** `init` and `global init` shall write `devin` in place of
  `windsurf`/`codeium` in the default `targets =` line.

### Part B: deprecation (same 4.x minor)

- **E8.** When a target list (flag or `.agents/config`) names
  `windsurf` or `codeium`, sync shall resolve it to `devin` and print
  one warning per run to stderr:
  `warning: target "windsurf" is deprecated; Windsurf is now Devin. Use "devin". Removed in 5.0.0.`
- **E9.** A target list naming both `devin` and a deprecated alias
  shall sync `devin` once.
- **E10.** Paths written by earlier versions for `codeium`
  (`~/.codeium/windsurf/global_workflows/*` links, the `codeium-rules`
  region, `.windsurf/rules` fold) shall be left in place during 4.x.
  Legacy Cascade still reads them.
- **E11.** `status` and `global status` shall list each surviving
  `codeium` placement as `deprecated (removed in 5.0.0)`. In a project
  that is each `.windsurf/<bucket>` fold and each link drilled into a
  real `.windsurf/<bucket>/` that resolves into `.agents/<bucket>`;
  the user's own files there are not listed.

### Part C: removal (5.0.0)

- **E12.** `windsurf` and `codeium` shall no longer resolve. A target
  list naming either shall fail with
  `unknown target "windsurf": Windsurf is now Devin; use "devin"`.
- **E13.** When sync targets `devin`, it shall remove, under the
  existing legacy proofs (`deliver_legacy.go`):
  - links under `~/.codeium/windsurf/global_workflows/` that resolve
    into the global root (`proofLinksIntoTree`);
  - the `codeium-rules` region from
    `~/.codeium/windsurf/memories/global_rules.md`, deleting the file
    only when the region was its whole content;
  - the `.windsurf/rules` fold symlink when it points into `.agents/`.
- **E14.** User-authored files under `.windsurf/` or `~/.codeium/`
  shall never be removed.
- **E15.** The `codeium` code paths (`codeiumDestination`,
  `windsurfGlobalBudget`, `CodeiumRulesRegion`, audit row, channel
  row) shall be deleted in the same release.

## Edge audit

- A repo on 4.x keeps `windsurf` in `.agents/config` forever: E8 warns
  every run, E12 fails loudly at 5.0 with the fix in the message.
- A user still on Windsurf (not updated): unsupported upstream ("every
  release but the latest is deprecated"). They pin sync-agents 4.x.
- `.windsurf/rules` and `.devin/rules` both folded during 4.x: Devin
  imports both, so rules load twice. E2 plus E10 cause this. Accept
  for the deprecation window, or skip the `.devin/rules` fold when the
  `.windsurf/rules` fold exists. **Decided 2026-11-17: skip (E2a).**
- Global rules reach Devin twice if it also reads `~/.claude/CLAUDE.md`
  (CLI doc) and the user targets `claude`. Same exposure Codex has.

## Open questions

1. **Global instructions path.** CLI docs name
   `~/.config/devin/AGENTS.md`; Devin Local docs name
   `~/.devin/global_rules.md`. **Decided 2026-11-17: the CLI path
   (E3)**, because both agents share a harness. Still unverified on a
   real install; if Devin Local ignores it, E3 adds the second host.
2. **Rule frontmatter.** `.agents/rules/*.md` files carry no
   `trigger:`. Confirm Devin treats a rule without `trigger:` as
   `always_on`; if not, E2 needs a rendered dialect like Cursor's.

