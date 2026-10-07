---
id: SPEC-013
title: "AGENTS.md is not an index (per-tool delivery channels, version-gated CLAUDE.md)"
status: 🟡 implemented on feature/agents-md-regions; release gate (per-tool symlink canary) open
owner: nmccready
created: 2026-10-01
updated: 2026-10-02
related: SPEC-002, SPEC-004, SPEC-006, SPEC-010, SPEC-011, SPEC-012
---

# [SPEC-013] Feature: AGENTS.md is not an index

## Overview

`sync-agents index` rewrites `AGENTS.md` as a list of links into `.agents/`. No tool follows those links, so the list delivers no rule content to anyone. Claude already gets rules from `.claude/rules/`, which makes the `@`-import block on top of the list redundant. The per-tool paths that were supposed to carry content to the other tools are paths those tools do not read.

This spec makes three changes:

1. `AGENTS.md` becomes the user's file. sync-agents rewrites it once, to remove the old index, and never writes it again.
2. Every non-Claude tool receives rule bodies through a read path its vendor documents. The generated content lives in `.agents/index/<tool>.<ext>` and is gitignored by default.
3. `CLAUDE.md -> AGENTS.md` is created only when the installed Claude Code needs it (< 2.1.281).

It is a breaking release, 2.0.0. The reasoning is recorded here in full because the owner asked for it.

**Status (2026-10-02).** U1–U7 are implemented on `feature/agents-md-regions`. Current behavior is documented in `docs/architecture/delivery-channels.md` and `docs/migration-v2.md`; where this spec and those docs differ, the docs and code win (see Implementation reconciliation). The release gate is open: see Remaining before ship.

## Problem

### Verified evidence

The evidence comes from an adversarial review (3 models, with live tests) and was re-checked on the owner's machine on 2026-10-01.

| # | Finding | Source |
|---|---|---|
| E1 | No tool follows links in `AGENTS.md`. Codex reads it as plain text and follows neither links nor `@` imports. opencode does not follow links; extra files come only from `opencode.json` `instructions`. OpenClaw 2026.9.6 (`bootstrap.ts`) reads the workspace `AGENTS.md` as raw text, cuts it to 20,000 chars by keeping head and tail, and resolves nothing. Claude does not follow links. | Codex and opencode docs; OpenClaw source; review |
| E2 | Claude Code 2.1.286 auto-loads `.claude/rules/*.md` and `~/.claude/rules/*.md`, including rule files that are symlinks. | Live canary during the review; `claude --version` = `2.1.286 (Claude Code)` |
| E3 | The import block is stale and redundant. The managed block in `~/.claude/CLAUDE.md` imports 7 of the 15 files in `~/.claude/rules/`. All 15 load anyway, and none loads twice. | `grep` of `~/.claude/CLAUDE.md`, `ls ~/.claude/rules` |
| E4 | Claude Code >= 2.1.281 reads `AGENTS.md` natively when no `CLAUDE.md`, `.claude/CLAUDE.md`, or `CLAUDE.local.md` exists in the working directory or above. Before 2.1.281 some sessions (Amazon Bedrock, telemetry disabled) still could not. A `CLAUDE.md -> AGENTS.md` symlink is read once either way. Claude never reads `AGENTS.override.md` or anything under `.agents/`. | code.claude.com/docs/en/memory, fetched 2026-10-01 |
| E5 | Copilot does not read `.github/copilot/instructions.md` (today's target). It does read `.github/copilot-instructions.md`, `.github/instructions/**/*.instructions.md` (with `applyTo`), `~/.copilot/copilot-instructions.md`, `~/.copilot/instructions/**`, and `AGENTS.md`. | GitHub Copilot docs |
| E6 | Cursor ignores `.md` in `.cursor/rules/` and reads only `.mdc` (frontmatter `description`, `globs`, `alwaysApply`). User rules live in app settings and have no file. | Cursor docs |
| E7 | Codex does not document `~/.codex/instructions.md` (today's target). It reads `$CODEX_HOME/AGENTS.override.md`, else `AGENTS.md`, then one file per directory from the git root to the cwd (override first), up to 32 KiB combined (`project_doc_max_bytes`). | Codex docs |
| E8 | opencode reads `AGENTS.md`, `~/.config/opencode/AGENTS.md` (falling back to `~/.claude/CLAUDE.md`), and `instructions` in `opencode.json`. Today global sync skips every passive rule for opencode. | opencode docs; `destination.go:337-345` |
| E9 | Windsurf reads `.windsurf/rules/*.md` (12,000 chars per file) and `~/.codeium/windsurf/memories/global_rules.md` (6,000 chars, always on). Its UI edits `global_rules.md`, which today is a whole-file concat. | Devin desktop docs |
| E10 | Local `sync` never uses `destination.go`. It folds `.agents/rules` into `.cursor/rules` (ignored, E6), `.github/copilot/rules` (read by nothing), and `.codex/rules` (read by nothing). Only Claude and Windsurf get content locally. | grounding §0, `agent.go:389-422` |
| E11 | `AGENTS.md` has four writers: the index, global sync's `openclaw-rules` region, OpenClaw doctor's `## Tools`, and humans' `## Inherits`. The generator keeps two headings and deletes the rest. The owner's workspace `AGENTS.md` (lines 14-16) carries a warning a human had to write: "This section is the ONE part of this file that `sync-agents index` preserves … anything added elsewhere is silently deleted on the next index". v1.10.0 shipped one more fix of this kind (`22aec45`, "preserve foreign sync-agents regions and the ## Tools section"). | `~/.openclaw/workspace/AGENTS.md`; CHANGELOG |
| E12 | Sizes already exceed caps. The 15 global rules total 31,000 bytes, against OpenClaw's 20,000 chars, Windsurf's 6,000, and Codex's 32,768 bytes. The workspace project's rules total 45,930 bytes. | `cat … \| wc -c` |
| E13 | 46 generated `AGENTS.md` files exist under `~/code` and `~` (depth-bounded scan). The migration grammar below matches every line of their generated sections. They reduce to the stub alone (7 files) or the stub plus `## Inherits` (39 files). | A port of `generatedLine` run over the files, 2026-10-01 |

So every non-Claude tool receives no rule content today, the one tool that does (Claude) receives it from a path that does not need the index, and the generated file destroys hand-written text.

### Owner's directives

- Less is more. Remove the link index from `AGENTS.md`.
- Non-Claude tools must receive rule content.
- Check the installed Claude version: create `CLAUDE.md -> AGENTS.md` when it is needed (< 2.1.281), otherwise warn.
- Anything generated per tool lives in `.agents/index/<tool>.<ext>`, using the extension the tool reads, and may be gitignored. It does not live in `AGENTS.md`.
- Document the reasoning heavily: this spec, the ledger, docs, migration notes, CHANGELOG.

### Constraints

- `ManagedRegion` (`region.go:17-47`), `writeIfChanged` (`region.go:178`), and `placeLink` (`bucketlink.go:174`: never delete; move aside only with `--overwrite`) are sound and kept.
- No tool-version probe exists. `ToolEnv` (`openclaw.go:21`) is `{Getenv, ReadFile}` and needs an exec seam.
- `$HOME` is a project whose `.agents/` is the global root, so local and global output must not collide.
- OpenClaw counts JavaScript string length (UTF-16 units). Codex counts bytes.

## Usage

### Project scope

```bash
sync-agents init              # .agents/, config, AGENTS.md stub when absent, .gitignore
sync-agents add rule security # writes .agents/rules/security.md, refreshes .agents/index/
sync-agents sync              # bucket links + .agents/index/ + native mounts + CLAUDE.md policy
sync-agents sync --targets opencode   # once: consent to add our entry to an existing opencode.json
sync-agents index             # regenerate .agents/index/ only (watch and add call the same code)
sync-agents status            # buckets, AGENTS.md ownership, CLAUDE.md decision, one row per channel
sync-agents clean             # removes links, mounts, our config entry, .agents/index/
```

`.agents/config` keeps `targets` and gains two optional keys:

```ini
targets = claude,windsurf,cursor,copilot,codex,opencode
# claude-md = auto   # auto (default) | link | off
# index = local      # local (default: generated output gitignored) | commit
```

After `sync` in a project with two rules, `security` and `testing`:

```text
AGENTS.md                                    yours; sync-agents never rewrites it after migration
AGENTS.override.md -> .agents/index/codex.md                                   [codex]
CLAUDE.md -> AGENTS.md                       only when claude --version < 2.1.281   [claude]
opencode.json                                "instructions": [".agents/index/opencode.md"]  [opencode]
.agents/index/                               generated; gitignored under index = local
  codex.md                                   AGENTS.md text (regions stripped), then rules
  copilot.md                                 applyTo: "**", then rules
  cursor.mdc                                 alwaysApply: true, then project + global rules
  opencode.md                                rules
.agents/.sync/AGENTS.md.pre-spec013          one-time backup from the migration
.claude/rules -> ../.agents/rules            unchanged                                [claude]
.windsurf/rules -> ../.agents/rules          unchanged                                [windsurf]
.cursor/rules/sync-agents.mdc -> ../../.agents/index/cursor.mdc                       [cursor]
.github/instructions/sync-agents.instructions.md -> ../../.agents/index/copilot.md     [copilot]
```

`.agents/index/cursor.mdc`:

```markdown
---
description: Shared rules from .agents/ (generated by sync-agents)
alwaysApply: true
---
<!-- Generated by sync-agents from .agents/. Edit those files, then run `sync-agents index`. -->

## security

Never commit secrets...

## testing

...
```

First `sync` on a repo with a generated `AGENTS.md`:

```text
[info] AGENTS.md: replaced the generated header with a 3-line pointer to .agents/; removed Rules,
       Skills, Workflows, State and the claude-imports block; kept ## Inherits.
       Backup: .agents/.sync/AGENTS.md.pre-spec013. sync-agents will not rewrite this file again.
[info] cursor   .cursor/rules/sync-agents.mdc -> .agents/index/cursor.mdc (2 project + 15 global rules)
[info] copilot  .github/instructions/sync-agents.instructions.md -> .agents/index/copilot.md (2 rules)
[info] codex    AGENTS.override.md -> .agents/index/codex.md (AGENTS.md + 2 rules, 6.1 of 32 KiB)
[info] opencode opencode.json exists and is yours; run `sync-agents sync --targets opencode` once to add
       ".agents/index/opencode.md" to "instructions"
[info] claude   CLAUDE.md not created: Claude Code 2.1.286 reads AGENTS.md natively (>= 2.1.281)
[info] Sync complete.
```

### User scope

```bash
sync-agents global sync                  # tools whose home exists; never edits a file you own without consent
sync-agents global sync --targets codex  # consent: adds the codex-rules region to an existing ~/.codex/AGENTS.md
sync-agents global status
sync-agents global clean
```

```text
~/.agents/index/{codex,copilot,codeium,opencode,openclaw}.md                 generated
~/.claude/rules/<rule>.md -> ~/.agents/rules/<rule>.md                       unchanged
~/.copilot/instructions/sync-agents.instructions.md -> ~/.agents/index/copilot.md
~/.codex/AGENTS.md                            region codex-rules; your text outside it is kept
~/.codeium/windsurf/memories/global_rules.md  region codeium-rules; UI edits outside it are kept
~/.config/opencode/opencode.json              "instructions": ["/Users/me/.agents/index/opencode.md"]
<openclaw workspace>/AGENTS.md                region openclaw-rules (SPEC-012 consent, unchanged)
```

`global sync` also strips the `claude-imports` block from `~/.claude/CLAUDE.md` and keeps the rest. It deletes `~/.github/copilot/instructions.md` and `~/.codex/instructions.md` only when they start with the old generated banner, and removes `~/.cursor/rules/<n>.md` links that point into `~/.agents`.

## Per-tool delivery

"Consent" means: when the native file already exists with bytes sync-agents did not write, one run with `--targets <tool>` is required (see Shape, Mounts and consent).

| Tool | Scope | Native read path (vendor) | sync-agents writes | Mount | Cap | Consent |
|---|---|---|---|---|---|---|
| Claude | local | `.claude/rules/*.md`; `CLAUDE.md`, else `AGENTS.md` (>= 2.1.281) | `.claude/rules -> ../.agents/rules` (unchanged); `CLAUDE.md -> AGENTS.md` per policy | fold | none | no |
| Claude | global | `~/.claude/rules/*.md` | per-rule links (unchanged); one-time strip of `claude-imports` | per-artifact | none | no |
| Windsurf | local | `.windsurf/rules/*.md`, root `AGENTS.md` | `.windsurf/rules -> ../.agents/rules` (unchanged) | fold | 12,000 chars per file (lint warns) | no |
| Windsurf | global | `~/.codeium/windsurf/memories/global_rules.md` | `~/.agents/index/codeium.md`; region `codeium-rules` (host created if missing) | Region | 6,000 chars, whole file, UTF-16 | if file has other bytes |
| Cursor | local | `.cursor/rules/*.mdc`, `AGENTS.md` | `.agents/index/cursor.mdc`; link `.cursor/rules/sync-agents.mdc` | Link | none documented | no |
| Cursor | global | app settings only; no file | nothing; global rules ride in each project's `cursor.mdc` under `index = local` | gap | n/a | n/a |
| Copilot | local | `.github/instructions/**/*.instructions.md`, `.github/copilot-instructions.md`, `AGENTS.md` | `.agents/index/copilot.md`; link `.github/instructions/sync-agents.instructions.md` | Link | none documented | no |
| Copilot | global | `~/.copilot/instructions/**/*.instructions.md` | `~/.agents/index/copilot.md`; link `~/.copilot/instructions/sync-agents.instructions.md` | Link | none documented | no (home must exist) |
| Codex | local | per dir: `AGENTS.override.md`, else `AGENTS.md` | `.agents/index/codex.md` (AGENTS.md text + rules); link `AGENTS.override.md` | Link | 32 KiB bytes, combined with the global file (reserved) | no |
| Codex | global | `$CODEX_HOME/AGENTS.override.md`, else `AGENTS.md` | `~/.agents/index/codex.md`; region `codex-rules` in `~/.codex/AGENTS.md` | Region | 32 KiB bytes | if file has other bytes |
| opencode | local | `AGENTS.md`, `opencode.json` `instructions` | `.agents/index/opencode.md`; entry in `opencode.json` (created if absent) | ConfigList | none | if file exists |
| opencode | global | `~/.config/opencode/opencode.json` `instructions` | `~/.agents/index/opencode.md`; absolute-path entry | ConfigList | none | if file exists |
| OpenClaw | local | none (no project channel; SPEC-012 global-only) | nothing | n/a | n/a | n/a |
| OpenClaw | global | `<workspace>/AGENTS.md`, raw text | `~/.agents/index/openclaw.md`; region `openclaw-rules` (host never created) | Region | 20,000 `bootstrapMaxChars`, whole file, UTF-16 | yes (SPEC-012) |

Symlink-following is unverified for Codex, Cursor, Copilot, and opencode. None of those CLIs is installed on the author's machine, and their docs say nothing about symlinks. Cursor 2.2.17 broke symlinked `.mdc` and 2.5 fixed it (Cursor forum). A per-tool canary is a pre-release gate (U7). The fallback for a tool that fails is to write the rendered bytes at the native path instead of a symlink. That is about 25 LOC in `mount` (a `Copy` flag on `LinkMount`, ownership proved by the banner) plus a one-row change in `channelSpecs`. It is not built until a canary fails.

Skills, agents, plans, specs, ADRs, and hooks are out of scope. Codex, Cursor, opencode, and OpenClaw read `.agents/skills` natively; Claude reads `.claude/skills`.

## Shape

The names below come from the Go type sketch written with this spec. The sketch is not committed; it builds and vets inside a scratch copy of `internal/agent`, and U3 turns it into code.

### Data

```text
channelSpecs (static vendor facts)   map[tool]map[Scope]ChannelSpec{Format, Mount, Budget, ShadowedBy, MergeGlobal}
        | bindChannels(run): active targets, resolved homes, MountFacts -> mayMount, $HOME dedupe
        v
[]Channel {Tool, Scope, Spec, Tree, Index, Native, Home, Mountable, Why}     every path absolute
        |
discoverChannelArtifacts(Tree, allOS) -> passiveEntries -> []Entry{Name, Source, Display, Description, OnDemand}
        | (+ mergeEntries(project, global) for MergeGlobal channels under index = local)
        | renderChannel(Format, entries, Frame, Budget)          pure
        v
Rendered{Bytes, Inlined, Pointers, Budget, Size}
        | writeIfChanged(Index); mount(Channel, Rendered, Mode)
        v
ChannelState: synced | stale | unmounted | conflict | shadowed | manual
```

One concept, the **channel**, replaces five mechanisms: index generation, the Claude import block, concat files, ad-hoc regions, and Cursor symlinks. A channel is how one tool receives aggregated passive content at one scope. `passiveEntries` is computed once per scope, so channels differ only in format, mount, and budget. `status`, `clean`, and `.gitignore` read the same bound `[]Channel`, so no path is derived twice.

### Mounts and consent

Three mount kinds cover every vendor read path. `Mount` is a sealed interface.

- `LinkMount`: sync-agents owns a file name the tool reads (a name it chose inside a scanned directory, or a file only this tool reads) and places a relative symlink to the index file. A real file there is a conflict: warned, counted in the exit status, never clobbered.
- `RegionMount`: the tool reads one fixed file that others also edit. Content is spliced between `<!-- sync-agents:<name>:start|end -->` markers; every byte outside is kept.
- `ConfigListMount`: the tool loads files listed in a JSON array. An absent config is created with our entry. An existing strict-JSON config gets the entry by `ensureJSONArrayEntry`, a byte-range insert at `json.Decoder` offsets that leaves formatting and key order alone. JSONC or unparseable config gets a warning and the exact line to paste, and is skipped (`manual`).

Consent follows the mount kind. This generalizes SPEC-012's marker rule from OpenClaw to every shared file. `mayMount(m, facts)` is pure:

| Facts | Verdict |
|---|---|
| our link, markers, or entry already there | mount (refresh) |
| global scope, tool home missing, no `--targets` | skip: tool not installed |
| `RegionMount` with `CreateHost=false`, host missing | skip: the owning program creates it (OpenClaw) |
| Region or ConfigList, native file exists, no `--targets` | skip: "run once with `--targets <tool>`" |
| otherwise | mount |

A `LinkMount` never needs consent, because the file name is ours. Creating a file that did not exist (`~/.codex/AGENTS.md` when `~/.codex` exists, a new `opencode.json`) edits nobody's bytes and needs none either. Listing a tool in `.agents/config` `targets` is not consent to edit a file the user owns: it says "deliver to this tool", not "edit my `opencode.json`". Every write into a shared file goes through `writeIfUnchanged`, a compare-and-swap that re-reads before the atomic rename and backs off on a concurrent edit.

### Budget

- `Cap{Limit, Unit, Knob}`; `Cap.Measure` is the only way to size content. `CapBytes` for Codex, `CapUTF16` for OpenClaw and Windsurf (JavaScript string length; today's rune count undercounts emoji).
- `Budget{Cap, Reserved}`. `Reserved` charges what the tool loads under the same cap: the host's bytes outside our region, and, for the Codex project override, the global file's size.
- `fitBudget` is first fit in name order. A section is inlined when the fixed head, the blocks inlined so far, this block, and the pointers still owed for the rest all fit. Otherwise it becomes a pointer line, `- name (path): description`, under `## Not inlined`, and the walk continues, so a later, smaller rule can still fit.
- The user's lever is `trigger: model_decision` in a rule's frontmatter, the "agent decides" mode Windsurf and Cursor already understand. In a capped bundle such a rule is always a pointer. Uncapped bundles inline everything.
- Sync warns with the demoted names and the knob (`project_doc_max_bytes in ~/.codex/config.toml`, `bootstrapMaxChars`). It warns louder when even the all-pointer form is over the cap. No tool ever truncates mid-rule, because sync stays under the cap.

Worked case (E12): the 31,000 bytes of global rules against OpenClaw's 20,000 chars inline in name order until the next rule no longer fits, and the rest become pointers. Today OpenClaw keeps head and tail and silently drops the middle.

### CLAUDE.md policy

`decideClaudeMD(mode, claudeActive, agentsMDExists, current, probe)` is pure and table-tested. Sync never removes a real `CLAUDE.md`; it removes only its own symlink, and only when a native Claude would read `AGENTS.md` without it (amended 2026-10-06, see Implementation reconciliation).

| Condition | Action |
|---|---|
| `claude` not an active target | keep (do not touch) |
| `claude-md = off` | keep (do not touch) |
| `CLAUDE.md` is a real file | keep; warn that it hides `AGENTS.md` from Claude and suggest `@AGENTS.md` in it |
| `CLAUDE.md` is a symlink that is not ours | keep; warn |
| no `AGENTS.md` | keep (a link would dangle) |
| `claude-md = link` | ensure `CLAUDE.md -> AGENTS.md` |
| auto, version < 2.1.281 | ensure the link: that version reads `CLAUDE.md` only (2.1.277–2.1.280 read AGENTS.md natively except in Bedrock/telemetry-off sessions; 2.1.281 is the first version that reads it in every session) |
| auto, any version, `CLAUDE.local.md` present in the project root | ensure the link: a `CLAUDE.local.md` suppresses native AGENTS.md reading (E4) |
| auto, a `CLAUDE.md`/`.claude/CLAUDE.md`/`CLAUDE.local.md` in a parent directory (not the user-level `~/.claude/CLAUDE.md`) | ensure the link (amended 2026-10-06) |
| auto, version >= 2.1.281 | create nothing; remove an existing link of ours (amended 2026-10-06: it is not harmless) |
| auto, probe failed or version unparseable | change nothing; warn once, naming the error and the `claude-md` key |

The probe runs `claude --version` through the new injected `ToolEnv.Run` (`CommandRunner`) with a 5 s timeout, at most once per process, and only when the answer depends on it. The zero `ToolEnv` has no runner, so no test starts a real `claude`. "Unknown changes nothing" is deliberate: `claude` is often on the terminal's `PATH` but not on a git hook's, and a rule that created the link on unknown would add it from the hook and drop it on the next terminal run, or the reverse.

### Index policy

`index = local` (default) gitignores `.agents/index/` and every `LinkMount` path together. A fresh clone has neither, so nothing dangles; tools read `AGENTS.md` alone until `sync`, and the stub points them at `.agents/rules/`. Output is OS-gated for this machine, and Cursor's index carries this user's global rules (`MergeGlobal`).

`index = commit` commits both, which serves cloud agents (Codex cloud, Copilot coding agent, Cursor background agents) with no setup. It compiles every OS scope with the existing `<!-- OS: <scope> -->` headers, so the bytes do not churn between a macOS and a Linux contributor. It never merges global rules, because a committed file must not carry one developer's personal rules. One `IndexPolicy` value drives both the index directory and the link paths in `gitignoreEntries`, so a committed link never points at an ignored file.

### Refresh versus mount

- `ChannelRefresh` (`index`, `watch`, `add`, `adr`, `import`, source commands) writes inside `.agents/index/` and re-splices regions and entries that already carry ours. It creates nothing in tool directories.
- `ChannelMount` (`sync`, `fix`, `global sync`) also removes legacy placements and creates or repairs mounts where `mayMount` allows.

### $HOME dedupe

When the project's `.agents/` is the global root (`sameTree`), a tool that also has a global channel skips its local channel: global sync owns that tree's index file for it, and Codex and Copilot never see the same rules twice from `$HOME`. Cursor has no global channel, so it keeps its local one and does not merge (the trees are the same).

### Invariants

| Invariant | Encoded in |
|---|---|
| Mounts are one of three kinds, each handled exhaustively | sealed `Mount` interface (`editsSharedFile` is the unexported method) |
| A cap carries its unit | `Cap.Unit`; `Cap.Measure` is the only sizing function |
| Consent before editing bytes someone else wrote | `mayMount`, keyed on `Mount.editsSharedFile` |
| A committed link never points at an ignored file | one `IndexPolicy` drives `.agents/index/` and every link path in `gitignoreEntries` |
| A committed index is the same on every machine | `IndexCommit` forces `allOS` and disables `MergeGlobal` |
| `status` says synced only when sync would write nothing | `channelRows` calls the same `renderChannel` and compares bytes |
| After migration, sync-agents never writes a project `AGENTS.md` | the only writer is `migrateProjectAgentsMD` (CAS); test: sync, index, watch leave a user-owned file byte-identical |
| One writer per region | one region name per (tool, scope), declared next to `channelSpecs` |
| Never delete a user file | `placeLink` conflicts; regions keep outside bytes; config edits insert only; legacy removal needs a proof |
| No rule body anywhere Claude reads | `channelSpecs` has no row whose native path Claude loads (Claude does not read `AGENTS.override.md`, `.cursor/`, `.github/instructions/`, `opencode.json`, `.agents/index/`) |

### Interface depth

Commands see five entry points: `deliverChannels(run)`, `channelRows(scope, explicit)`, `cleanChannels(scope, explicit)`, `migrateProjectAgentsMD()`, and `claudeMDDecision()` with `applyClaudeMD(d)`. Behind them are formats, frontmatter dialects, budgets and units, vendor caps, home resolution (`CODEX_HOME`, the OpenClaw workspace), consent, three mount kinds, JSON byte-range edits, legacy cleanup, region splicing, and ownership proofs. Callers never choose a mount, compute a path, or branch on a tool ID. A request goes `CmdSync` → `deliver.go` → `channel.go` and `region.go`: three files deep.

## Where the directive cannot hold literally

- **OpenClaw.** Its only instruction input is `<workspace>/AGENTS.md`, read as raw text with no override and no list slot. Its content has to be in that file. Globally, that file is OpenClaw's bootstrap file, a tool-owned host like `global_rules.md`, and the region is consent-gated exactly as in SPEC-012. When the workspace is also a sync-agents project (the owner's case), the same file is that project's `AGENTS.md`. The `openclaw-rules` region is then the one place rule bodies sit in an `AGENTS.md`. A Claude session started in that workspace reads them in addition to `~/.claude/rules` (bounded by the 20,000 cap; Tradeoffs).
- **Codex at project scope.** `AGENTS.md` stays clean. But Codex reads one file per directory, so `.agents/index/codex.md` carries a copy of `AGENTS.md`'s text, with every `sync-agents:*` region stripped so an OpenClaw region does not reach Codex twice. The copy is stale between an edit and the next `index`; `status` reports `stale`, and `watch` watches `AGENTS.md`.
- **The one-time migration** rewrites `AGENTS.md` once: header to stub, generated sections out, imports block out, with a backup and a compare-and-swap.
- **opencode.** The content lives in `.agents/index/`, but the pointer to it is one entry in the user's `opencode.json`, added only with consent.
- **`init`** writes a 4-line stub when `AGENTS.md` is absent. After that the file is the user's.

## Fate of each piece

| Piece | Fate |
|---|---|
| `sync-agents index` | Regenerates `.agents/index/` (refresh mode) and runs the one-time migration. Keeps `--no-fix` (skill frontmatter backfill). Old pre-commit hooks that call it keep working. |
| `watch` | Runs `index` on change. Excludes `.agents/index/` (no self-trigger) and also watches `AGENTS.md`, which Codex's override copies. |
| `## Inherits` | Ordinary text in a user-owned file; never touched. |
| `inherit` command | Removed; a hidden stub prints "AGENTS.md is yours now; edit ## Inherits by hand (docs/inheritance.md)". Its links never delivered content. `promote` and linked sources are the mechanisms that do. |
| `import: true` | Inert. Claude no longer preloads opted-in plans, specs, or ADRs; they stay @-mentionable under `.claude/<bucket>/`. `lint` warns and suggests a rule (`add rule <n> --from <doc> --link`), which reaches every tool. |
| Skills listing | Gone. Each tool loads skills from its native skills directory. |
| State pointer | Gone. `rules/state.md` is a rule and reaches every tool as content. `shared: true` keeps its integrity-lock meaning. |
| `trigger: always_on` header | Removed with the generated header. |
| Preserved `## Tools` | User and OpenClaw-doctor text; never touched. `capturePreservedSections` is deleted. |
| `claude-imports` region | Both writers deleted. `scrubClaudeManagedBlock` survives as a legacy proof for `~/.claude/CLAUDE.md`; the migration strips it from `AGENTS.md`. |
| Concat files, `StrategyConcat`, `StrategyRegion` | Replaced by channels. `buildEntriesBody` and `readArtifactBody` are reused by `renderChannel`. |
| `Tool.Region`, `RegionFile`, `RegionCharCap` | Moved into `channelSpecs` and `BudgetSource`. |

## Migration

Runs inside `sync`, `index`, `fix` (local) and `global sync` (global). Idempotent, atomic, `--dry-run` aware, reported once.

1. **Project `AGENTS.md`** (`migrateAgentsMD`, pure; `migrateProjectAgentsMD`, shell).
   - Strip the `claude-imports` region from any `AGENTS.md`, generated or not.
   - Only a file whose first non-blank lines equal `generatedHeader` (bash 0.1 through Go 1.10) is treated as generated. Its header becomes `AgentsMDStub`.
   - Each `## Rules|Skills|Workflows|Agents|Plans|Specs|ADRs|Hooks|State` section is removed only when every body line matches `generatedLine`: index entries with optional OS badge and description, `_No …_` placeholders, the State sentence, the ADR preamble, `### Accepted|Proposed|Shared`, blank lines.
   - Everything else is kept byte for byte and in order: `## Inherits`, `## Tools`, unknown sections, a generated-titled section that holds a hand-written line (reported as kept), and other sync-agents regions (opaque blocks, even when they contain `##`).
   - Before the first rewrite, the old bytes go to `.agents/.sync/AGENTS.md.pre-spec013`. The backup is written once and never overwritten.
   - The write is `writeIfUnchanged(path, old, new)`. An edit that lands between read and write makes it a warned no-op; the next run retries.
   - Property tests: `migrate(migrate(x)) == migrate(x)`, and every line outside the removed spans survives in order. Golden fixtures cover the bash-era header (leading blank line), Go outputs with descriptions, OS badges, ADR groups, hook lines, both State variants, the OpenClaw workspace shape (`## Inherits` with an HTML comment and `@SOUL.md` lines), `## Tools`, a foreign region, a hand-written line inside `## Rules`, and a non-generated file.
2. **`~/.claude/CLAUDE.md`.** Strip the `claude-imports` region; keep user lines such as `@~/.claude/pstack-models.md`. Remove the file only if nothing else remains (today's `scrubClaudeManagedBlock` rule).
3. **Project `CLAUDE.md`.** Decided by `decideClaudeMD`. An existing link of ours is kept on >= 2.1.281, so no installed setup changes on upgrade.
4. **Dead placements** (`legacyPlacements`). Each is removed only when its proof holds; anything else is reported and left.

   | Scope | Path | Proof |
   |---|---|---|
   | local | `.cursor/rules` (fold, or per-rule links inside a real dir) | symlink into `.agents/`; must run before the `.mdc` link |
   | local | `.github/copilot/rules`, `.codex/rules`, `.opencode/rules` | symlink into `.agents/` |
   | global | `~/.cursor/rules/<n>.md` | symlink into `~/.agents/` |
   | global | `~/.github/copilot/instructions.md`, `~/.codex/instructions.md` | regular file starting with `legacyConcatBanner` |
   | global | `~/.claude/CLAUDE.md` | carries the `claude-imports` region |
   | global | `global_rules.md` starting with the banner | wholly ours: rewritten in place as the `codeium-rules` region, whose markers then carry consent |

5. **`.gitignore`.** `init`'s default block holds `.agents/index/` instead of the Copilot and Codex `instructions.md` lines. `sync` appends exact lines from `gitignoreEntries`: `.agents/index/`, each link path, and `CLAUDE.md` when it is our link. Stale lines (`.cursor/*`, `!.cursor/rules`, `.github/copilot/*`) are left; an appended exact file path still wins because the last match decides. `docs/migration-v2.md` lists the lines users may delete.
6. **Git hook.** `hook` replaces the block between its markers with `sync-agents sync` and `git add -- .agents/index 2>/dev/null || true` (a no-op under `index = local`; under `commit` it restages regenerated files; the link paths never change after the first commit). Old installed hooks keep working: their `sync-agents index` now regenerates the index.
7. **Owner's machine.** The first run rewrites the PLEX-baselined workspace `AGENTS.md` (`/mark-drift` afterwards). The HTML comment in its `## Inherits` block, which warns that index deletes everything else, becomes false and can be removed by hand.

## Tradeoffs accepted

**T1. Symlinks at native paths.**
- Pros: ownership is provable ("a link into `.agents/`"); one place to inspect output; the directive holds literally.
- Cons: symlink-following is unverified for Codex, Cursor, Copilot, and opencode; Cursor broke it once (2.2.17).
- We trade certainty about four read paths for a single generated location. Reconsider per tool when its canary fails: flip that row to a real file.

**T2. Codex project delivery through `AGENTS.override.md`.**
- Pros: Codex is the only reader of the override, so no other tool gets the rules twice and `AGENTS.md` stays clean.
- Cons: Codex reads a copy of `AGENTS.md` that is stale until the next `index` (`status`, `watch`, and the hook bound it). A user's own `AGENTS.override.md` is a conflict, and Codex project delivery stops until it is moved aside.
- We trade freshness for single delivery. Reconsider if Codex adds a config list of instruction files.

**T3. `index = local` by default.**
- Pros: no generated diffs, no dangling links, no OS-specific or personal content in git.
- Cons: fresh clones and cloud agents see only `AGENTS.md` until someone runs `sync`.
- We trade zero-setup cloud delivery for a clean repo. Reconsider for teams that rely on cloud agents (`index = commit`, open question 4).

**T4. First-fit pointer demotion.**
- Pros: deterministic; never a mid-rule cut; every demoted rule stays reachable by path; no new priority field.
- Cons: name order, not importance, decides who is demoted; a pointer works only if the agent opens it.
- We trade control for simplicity; `trigger: model_decision` is the lever. Reconsider if users ask for priority ordering.

**T5. Consent follows the mount kind.**
- Pros: sync-agents never edits bytes someone else wrote without being asked, uniformly with SPEC-012.
- Cons: one extra `--targets` run for Codex, Windsurf global, and opencode when the file already exists; until then that channel shows `unmounted`.
- We trade one command for never surprising a user. Reconsider if `unmounted` rows turn out to be ignored.

**T6. Unknown Claude version changes nothing.**
- Pros: no link flapping between terminal and hook runs.
- Cons: on a machine where `claude` is never on `PATH`, auto mode never creates the link; the warning names `claude-md = link`.
- Cost: one `claude --version` per sync when `claude` is a target in auto mode, 5 s worst case (timeout). Typical latency is unmeasured.
- We trade automatic linking on odd machines for stable output. Reconsider if Claude Code exposes its version without an exec (a file or env var).

**T7. Cursor gets global rules through each project.**
- Pros: Cursor has no user-rules file, and this is the only file path to it.
- Cons: a repo without `.agents/` gets nothing; under `index = commit` Cursor gets no global rules at all.
- We trade completeness for never committing personal rules. Reconsider if Cursor ships a file-based user-rules location.

**T8. `opencode.json` byte-range insert.**
- Pros: no reformatting, no key reordering, no JSONC dependency.
- Cons: about 70 LOC of custom JSON offset handling; JSONC configs need a manual line.
- We trade a small parser for never rewriting a user's config. Reconsider if opencode documents a drop-in instructions directory.

**T9. OpenClaw workspace double-load into Claude.** A Claude session started inside an OpenClaw workspace reads up to 20,000 chars of region text that duplicates `~/.claude/rules`. Accepted: the region is OpenClaw's only input. Reconsider if Claude gains a per-directory exclude.

**T10. A 2.0.0 release with breaking changes** (listed below). We trade upgrade friction for deleting about 450 net lines and a file with four writers. `docs/migration-v2.md` and the backup bound the cost.

## Alternatives considered

- **A. `AGENTS.md` as the universal channel** (one region of rule bodies read by every tool). Smallest interface. Rejected: Claude double-loads unless `.claude/rules` is dropped; one cap must serve every tool (the minimum of 20,000 chars and 32 KiB); a committed human file churns on every rule edit; the four-writer file stays; the owner rejected it.
- **B. A region per tool inside `AGENTS.md`.** Every `AGENTS.md` reader receives every region, so rules are delivered N times, and the multi-writer file returns.
- **C. One native file per rule** (`.mdc` and `.instructions.md` per rule). Keeps per-rule `globs` and `applyTo`. Rejected for now: N links, N gitignore lines, and N conflict checks per tool, and a whole-tool budget becomes meaningless. A later `Format` can add it.
- **D. Real files at native paths, no `.agents/index/`.** Immune to symlink regressions, but ownership falls back to banner sniffing in tool directories and contradicts the directive. Kept as the per-tool fallback (T1).
- **E. Route local sync through `TargetDestination`** (SPEC-010's global sync unification). The right long-term move, but it rewrites fold/drill semantics that rule delivery does not need. The channel layer is scope-generic, so this can land later without touching it.
- **F. Plan as data** (candidate 2: sealed `Op`s, `Plan.Undo`, `Plan.Only`). Status-equals-sync is already guaranteed by `channelRows` re-rendering the same bytes. The op layer adds ten op types and an executor for no further guarantee.

## Synthesis decision

**Process.** `/interrogate` ran an adversarial review of the old pipeline (3 models plus live tests: the Claude rules canary, `claude --version`, vendor docs, OpenClaw source). It produced the brief and a traced grounding of the code. `/arena` then had three candidates design from the same brief and grounding, each with a written design and a Go sketch that builds inside `internal/agent`. This spec is candidate 3 as the base, with grafts from candidates 1 and 2.

**Base: candidate 3.** Chosen for:
- the smallest vocabulary that covers every vendor path (three mounts);
- the pure `renderChannel`, against which status compares bytes;
- `Budget` with `Cap{Limit, Unit}` and `Reserved` (it handles Codex's combined cap and OpenClaw's whole-file count);
- first-fit pointer demotion;
- the `sameTree` dedupe;
- the grammar-based migration (verified on 46 files, E13);
- `IndexPolicy`, which ties gitignore of links and index together;
- the refresh/mount split, which keeps `add` out of tool directories;
- Codex global and Windsurf global as regions, so the user's own text stays in the file the tool reads.

**Grafts.**

| Graft | From | Why |
|---|---|---|
| Consent follows the mount kind (`mayMount`) | C1 | Generalizes SPEC-012 to every shared-file edit, and replaces C3's four `Gate` values with one fact (`editsSharedFile`) |
| One-time backup `.agents/.sync/AGENTS.md.pre-spec013` | C1 | The migration is the only time sync-agents rewrites a user's file; the backup makes it reversible |
| Cursor global rules merged into the project `cursor.mdc` under `index = local` | C1 | Cursor has no user-rules file; this is the only file path |
| `ensureJSONArrayEntry` byte-range insert into strict-JSON `opencode.json` | C2 | C3 left existing configs as a permanent manual step |
| `index = commit` compiles every OS scope | C2 | Committed bytes must not churn between contributors' machines |
| `trigger: model_decision` as the budget lever | C2 | Already understood by Windsurf and Cursor; it does not change routing |
| Compare-and-swap on the migration write (extended to all shared-file writes) | C2 | An editor save during sync can no longer be undone silently |
| Explicit verifiable commit order | C2 | Adapted as U1-U7 |

**Rejections.**

| Rejected | From | Reason |
|---|---|---|
| Unknown claude version → create the link | C1, C2 | Flaps between terminal and git-hook `PATH`, where `claude` often is not |
| `invocable: true` as the cap lever | C1 | Moves the rule into Claude's `commands/`, changing Claude routing to fix another tool's budget |
| Write over-cap bundles in full | C1 | OpenClaw's head/tail truncation silently drops the middle; pointers drop whole, named rules instead |
| `global_rules.md` as a symlink to a generated file | C2 | Windsurf UI edits would land in a generated file and be overwritten |
| Title-only drop of generated sections | C2 | Deletes hand-written lines inside a generated-titled section; the line grammar keeps them |
| Global Codex as an override composite | C1, C2 | Hides the user's `~/.codex/AGENTS.md` behind a copy; a region keeps it live |
| `openclaw-project-rules` region | C3 | Double-loads into Claude in the workspace and makes two writers race for one 20,000-char budget. OpenClaw stays global-only (SPEC-012) |
| Plan-as-data op layer | C2 | More vocabulary than needed (Alternative F) |

## Breaking changes

1. `AGENTS.md` is no longer generated. Existing generated files are rewritten once to a stub plus kept sections, with a backup.
2. `sync-agents index` regenerates `.agents/index/`, not `AGENTS.md`.
3. `inherit` is removed (hidden stub for one release).
4. `import: true` no longer preloads plans, specs, or ADRs into Claude.
5. `CLAUDE.md -> AGENTS.md` is version-gated and created only when `claude` is an active target. A real `CLAUDE.md` is never moved aside, `--overwrite` included.
6. Local `sync` no longer folds `rules` into `.cursor/rules`, `.github/copilot/rules`, `.codex/rules`, or `.opencode/rules`; those links are removed.
7. Global paths move. Copilot goes to `~/.copilot/instructions/`. Codex goes to a region in `~/.codex/AGENTS.md`. `~/.cursor/rules/*.md` links are removed. `global_rules.md` becomes region-only.
8. `global sync` no longer creates homes for uninstalled tools, and edits an existing shared file only after one `--targets <tool>` run.
9. The `claude-imports` block is removed from `~/.claude/CLAUDE.md` and `AGENTS.md`.
10. `init`'s `.gitignore` defaults and the pre-commit hook text change.

Release: a `feat!:` commit whose `BREAKING CHANGE:` footer lists the ten items above and links `docs/migration-v2.md`, producing 2.0.0 (current: v1.10.0).

## Deletion list

| File | Removed | LOC |
|---|---|---|
| agent.go | `generateAgentsMD` (1461-1786) | ~326 |
| agent.go | `CmdInheritList/Remove/Add` (1239-1428), replaced by a 12-line stub | ~178 |
| agent.go | `listMDFiles`, `indexEntry`, `indexEntryBadge`, `osScopeOrder`, `scopedEntry`, `osScopedMDFiles`, `osScopedSkills` (`artifactDescription` kept for pointers) | ~95 |
| agent.go | `preservedSectionTitles`, `capturePreservedSections`, `preservedHeading` | ~38 |
| agent.go | CLAUDE.md branches in Sync, Status, Fix, Clean (replaced by policy calls) | ~35 net |
| managedimport.go | whole file | 260 |
| region.go | `ClaudeImportsRegion`, entry-based `renderRegion`/`RegenerateRegion`, `regionStartPattern`, `foreignRegions` | ~50 net |
| concat.go | `ConcatBanner`, `RegenerateConcat` | ~50 |
| destination.go | `StrategyConcat`, `StrategyRegion`, cursor/copilot/codex/opencode/openclaw passive branches | ~75 net |
| globalsync.go | Claude import bookkeeping and block, concat batches, region seeding, `regenerateRegions`, region consent in `bindTool` | ~145 net |
| globalstatus.go | `ConcatState`, `classifyConcatTarget`, `buildConcatContent`, `classifyRegion`, concat/region rows | ~120 net |
| globalclean.go | `stripToolRegion`; CLAUDE.md scrub moves to legacy cleanup | ~40 net |
| tool.go, openclaw.go | `Region`, `RegionFile`, `RegionCharCap`, `openClawRegionCharCap` | ~20 |

About 1,450 production lines go and about 1,000 arrive: channel.go ~300, deliver.go ~250, agentsmd.go ~190, claudemd.go ~110, codex.go ~70, jsonentry.go ~70, edits ~30. Net is about −450; `agent.go` goes from 2,177 to about 1,480. Tests: about 1,690 Go lines deleted or rewritten (managedimport_test 358, index_test ~475, index_regions_test 109, link_index_test 66, the osscope index cases 58, globalsync ClaudeImports 131, integration index/inherit ~160, concat ~180, globalstatus ~65, destination ~45, adr ~25, openclaw index-preserves ~26). About 380 bats lines are rewritten. About 1,000 Go and 150 bats lines are added.

## Implementation plan

Each unit ends green (`GOTOOLCHAIN=local go test -mod=vendor ./...` plus the bats suites it touches) and is one commit. Subtract first.

- **U1. Spec and ledger.** This file; ledger row; next ID SPEC-014. Verify: the ID is unique in the ledger and the file name matches `specs/SPEC-NNN-kebab-title.md`.
- **U2. Migration first, then deletion.**
  - Copy the 46 generated `AGENTS.md` files (E13) into `testdata/agentsmd/` as goldens. Write golden, idempotency, and survival tests. Implement `migrateAgentsMD`, `writeIfUnchanged`, and the backup.
  - In the same unit, delete `generateAgentsMD`, the preserved-section machinery, `managedimport.go`, the Claude import bookkeeping in global sync, and `inherit`. Callers call `migrateProjectAgentsMD` only. `index` keeps the backfill.
  - Verify: every golden reduces to stub or stub + `## Inherits`; a second run writes nothing (same mtime); `~/.claude/CLAUDE.md` fixture keeps its non-import lines.
  - The intermediate state loses nothing real: the deleted list and imports delivered nothing beyond what `.claude/rules` already did.
- **U3. Pure channel layer.** `channelSpecs`, `Format`, `Mount` + `mayMount`, `Cap.Measure`, `fitBudget`, `renderChannel`, `passiveEntries`, `mergeEntries`, `ensureJSONArrayEntry`/`removeJSONArrayEntry`. Table tests:
  - a golden per format;
  - budget edges: exact fit, all-pointers, on-demand, UTF-16 emoji, `Reserved`;
  - the consent truth table;
  - JSON insert/remove preserving bytes, plus JSONC refusal.
  - Nothing is wired yet. Verify: `go test -run 'Channel|Budget|Mount|JSON'`.
- **U4. Local wiring.**
  - `deliverChannels`/`bindChannels` for local scope, `linksBucket` in the fold loop, `gitignoreEntries` and `IndexPolicy`, local legacy cleanup, `channelRows` in `status`, `cleanChannels` in `clean`, `index`/`add`/`adr`/`import`/source commands on refresh, `watch` exclusion, `sameTree`.
  - Verify with a fixture project for every target: the expected link set and index bytes; a second `sync` writes nothing; `clean` restores the pre-sync tree; a user-owned `AGENTS.md` stays byte-identical across sync, index, and watch.
- **U5. CLAUDE.md policy.** `ToolEnv.Run`, `probeClaude`, `decideClaudeMD` (full truth table), `applyClaudeMD` in sync, fix, and status. Bats: a fake `claude` on `PATH` printing `2.1.276`, `2.1.286`, and exiting 1. Verify: link only for 2.1.276; an existing link survives under 2.1.286; nothing changes on exit 1, and the warning prints once.
- **U6. Global wiring.**
  - `StrategyChannel` in `TargetDestination`; `deliverChannels` for global scope. Replace concat, region, and Claude-import code with the `codex-rules` and `codeium-rules` regions, the Copilot link, the opencode config entry, and OpenClaw via its channel (SPEC-012 consent preserved by `mayMount`). Global legacy cleanup; `global status` and `global clean` through `channelRows`/`cleanChannels`.
  - Verify: integration-global.bats routing rewritten; the legacy bannered files are removed and a non-bannered one is left; the OpenClaw consent tests from SPEC-012 pass unchanged.
- **U7. Docs and release.**
  - Write `docs/migration-v2.md` and `docs/architecture/delivery-channels.md` (the delivery table as current truth). Rewrite `docs/commands/index.md` and `docs/inheritance.md`. Update the rest of grounding §7: README, docs/README, the commands docs, semantic-routing, scope-and-targets, topology, os-scoped-routing, adrs, `site/index.html`, `examples/fix/run-demo.sh`, the init and global-init config comments.
  - Release gate: a manual canary per tool on a machine that has it (Cursor >= 2.5, Copilot VS Code and CLI, Codex, opencode). Each confirms the symlinked file loads; for Codex, also that Claude ignores `AGENTS.override.md`. A failing tool flips to the real-file fallback before tagging.
  - Then the `feat!:` commit, 2.0.0. On ship, delete this spec and update the ledger row.

## Implementation reconciliation

Accepted deviations from the plan above, per unit. The docs describe the result as current truth.

**U1** (`7c4df3e`). As planned.

**U2** (`1bba105`, migration and deletion).
1. 11 representative fixtures (3 real, 2 from the pre-change binary, 6 hand-written) instead of 46 copies: the real workspace file carries PII. An uncommitted corpus check ran over 73 generated files on the author's machine; all reduce to stub, stub + Inherits, or stub + Inherits + Tools, and all are idempotent.
2. Blank-line runs between kept blocks are normalized to one; kept blocks are byte for byte; the survival test checks non-blank lines.
3. Removing a mid-file `claude-imports` block drops one adjacent blank line.
4. The migration also runs in `sync` and `fix`. `init` writes the stub only when `AGENTS.md` is absent (Lstat).
5. The `~/.claude/CLAUDE.md` strip runs before global sync's "no artifacts" early return.
6. `artifactDescription` and `regionStartPattern` are kept (pointers, migration); `foreignRegions` is deleted.
7. Limit: an `AGENTS.md` that is itself a symlink has its target rewritten (the backup holds the original).

**U3** (`05737bb`, pure channel layer).
1. Generated files end with `"\n\n"` so exact-fit size accounting holds.
2. `fitBudget` tries a whole-fit fast path first, then greedy first fit with the pointer heading charged up front.
3. `renderChannel` strips regions from `Frame.AgentsMD` itself; a missing or blank `AGENTS.md` means no copy and no override note.
4. One `errJSONNotEditable` covers JSONC and wrong shapes.
5. `removeJSONArrayEntry` drops the member when the array becomes empty, so ensure then remove round-trips exactly. Limit: a user's own empty `instructions: []` disappears on remove.
6. `ensureJSONArrayEntry` creates a document from empty input.
7. `codex.go` landed here; `resolveCodexHome` and the registry change moved to U6.

**U4** (`8f34f57`, local wiring).
1. The `!.cursor/rules` gitignore line is kept, so teams' committed `.mdc` rules stay visible; the exact `.cursor/rules/sync-agents.mdc` line ignores our link. `init`'s block: `.cursor/*`, `!.cursor/rules`, `.codex/*`, `.github/copilot/*`, `.agents/index/`.
2. Per-target directory gitignore lines are unchanged. Under `index = commit`, the `.cursor/` line still ignores the `.mdc` link and sync re-adds it if removed. sync warns only about an `.agents/index/` line, not about `.cursor/`. Documented workaround: `git add -f .cursor/rules/sync-agents.mdc` once.
3. `gitignoreEntries` takes the run's results, so a user's real file at a link path is never ignored.
4. Local legacy cleanup is derived from the registry (`legacyFoldPaths`, the inverse of `linksBucket`); the sketch's `legacyPlacements` table was not built.
5. `channelRows` returns `([]StatusEntry, error)`; extra row states `skipped` (sameTree) and `error` (render failure).
6. A region mount at project scope errors with "delivered by global sync".
7. `App.TargetsFromFlag` drives Explicit (consent).
8. Dry-run passes to-be-removed legacy paths to mount, so it prints would-link instead of a false conflict.
9. Known limits: `fix` does not count channel mounts in "Repaired N"; bucket-only `clean` ignores `--dry-run` (pre-existing).
10. Real run on a temp copy of this repo: Cursor 12 rules (1 global), Copilot, Codex, and opencode 11; Codex 26,630 of 32,768 bytes; a second sync changed no mtime; `clean` restored the tree; the `AGENTS.md` sha was stable.

**U5** (`65d0b4b`, CLAUDE.md policy).
1. `decideClaudeMD(claudeMDFacts, probe func() ClaudeProbe)` takes a struct, because `CLAUDE.local.md` added an input.
2. The decision is computed once per command and passed to `updateGitignore`; no memo on `App`.
3. `ClaudeMDDecision` gains `Current` and `Managed`; `applyClaudeMD` returns `(changed, error)`.
4. "Our link" is any symlink that resolves to `AGENTS.md`.
5. An invalid `claude-md` value warns and changes nothing.
6. Bats setup puts a fake `claude` (2.1.286) first on `PATH`.
7. `fix` no longer relinks a foreign `CLAUDE.md` symlink and `clean` no longer removes one (in the BREAKING footer).
8. The old hook's `git add AGENTS.md CLAUDE.md ...` failed when `CLAUDE.md` was absent; fixed in U4.

**U6** (`f83cda4` split of deliver.go, `fd70c66` global wiring).
1. The home gate also covers per-artifact links for tools with a global channel, so global sync no longer creates `~/.codeium` or `~/.config/opencode` for an uninstalled tool. Skip warnings print only when the channel delivers.
2. Undeliverable global channels are silent unless `--targets` names the tool; `global status` shows `unmounted` with the reason.
3. The global Copilot link and the opencode entry use absolute paths (SPEC-002 convention).
4. `global clean` removes an emptied region host only where sync may create it (Codex, Windsurf); the OpenClaw host is always kept.
5. Passive skills enter channels for every tool; Cursor's passive content routes through the channel (gap row); `Rendered.Demoted` names only cap-forced rules; a channel link conflict makes global sync exit non-zero.
6. Known limits: `global clean` still removes an emptied tool home (pre-existing); the non-bannered legacy "left …" line repeats each run; a rule with broken frontmatter warns twice.

**U7** (`1c6c08d` lint; docs, spec, and ledger in the following commit).
1. `lint` and `lint all` report W201 for `import: true` on plans, specs, and ADRs (recursive). The spec's suggested remedy, `add rule <n> --from <doc> --link`, is refused for a doc already inside `.agents/` (cycle guard), so the docs suggest `git mv` into `rules/`, or `add rule --from` without `--link` to copy.
2. The Windsurf project 12,000-char per-file lint warning was not built; the docs mark the limit as not checked.
3. The embedded state rule template no longer says `index` lists snapshots; `shared: true` is described by what it still does (the integrity lock, for snapshots inside a bucket).
4. Open question 3 is answered by the Claude Code memory docs (checked 2026-10-02): `@path` imports inside an `AGENTS.md` that Claude reads natively are expanded.
5. CHANGELOG.md is generated by `commit-and-tag-version` at release and is not hand-edited; the 2.0.0 entry comes from the `feat!`/`BREAKING CHANGE` footers.

**Size outcome** (measured `7c4df3e..fd70c66`). Production Go +4,240 / −1,932 (net +2,308); tests +3,318 / −1,553. The plan estimated net −450 to −580. Deletions were counted correctly; the new delivery work was under-counted: five tools that received nothing now receive content (Cursor, Copilot, Codex, and opencode at both scopes, OpenClaw through its channel), plus the migration (~330), the CLAUDE.md policy (~400), and JSON byte-range editing (~290). `agent.go` went from 2,177 to 1,605 lines. A simplify pass over `deliver*.go` and `channel.go` is recommended.

### Post-U7 amendment (2026-10-06): parent-directory CLAUDE.md

- **Found:** a live canary showed an `AGENTS.md` under `~` was not loaded by Claude Code 2.1.286, while the same file under `/tmp` was. Cause: `~/CLAUDE.md -> AGENTS.md` (created by sync-agents 1.x with `$HOME` as a project). Claude reads `AGENTS.md` only when no `CLAUDE.md`, `.claude/CLAUDE.md`, or `CLAUDE.local.md` exists in the working directory or above it (E4), so that one link hid `AGENTS.md` in every project under `~`.
- **Two policy gaps:** U5 checked `CLAUDE.local.md` only in the project root, and kept an existing link on >= 2.1.281 as "harmless".
- **Change:** `shadowingClaudeMD` walks the parent directories (skipping the user-level `~/.claude/CLAUDE.md`, which does not count) and `auto` links when it finds one. On >= 2.1.281 our own link is removed (`ClaudeMDUnlink`), re-checked just before removal; a real file or foreign symlink is still never touched. Truth table extended to 960 combinations with an unlink invariant; bats covers removal and the parent case.
- **Supersedes:** the "an existing link of ours stays" row of the policy table and U5 deviation context.

## Remaining before ship

- [ ] Per-tool symlink canary on a machine that has each tool: Cursor >= 2.5 (`.cursor/rules/sync-agents.mdc` symlink loads), Copilot in VS Code and the CLI (`.github/instructions/` and `~/.copilot/instructions/` links load), Codex (`AGENTS.override.md` symlink loads, and Claude ignores the override), opencode (symlinked `instructions` entry loads; a missing entry from a fresh clone is a no-op). A failing tool flips its `channelSpecs` row to the real-file fallback (T1) before tagging.
- [ ] Simplify pass over `deliver*.go` and `channel.go` (size outcome above).
- [ ] Release 2.0.0 from a `feat!:` commit whose `BREAKING CHANGE:` footer lists the breaking changes and links `docs/migration-v2.md`.
- [ ] On ship, per `.agents/rules/specs.md`: confirm the durable content is in `docs/` (delivery-channels.md, migration-v2.md), delete this file, and update the ledger row (status, retire commit, doc destinations).

## Open questions

1. **Canary results** (U7 gate). Does each of Cursor, Copilot (VS Code and CLI), Codex, and opencode follow a symlink at its read path? Does opencode treat a missing `instructions` file (fresh clone with a committed `opencode.json`) as a no-op?
2. **Copilot and `CLAUDE.md`.** The Copilot coding agent may also read a root `CLAUDE.md`. If it does, `CLAUDE.md -> AGENTS.md` (created for Claude < 2.1.281) gives Copilot `AGENTS.md` twice. Unverified.
3. ~~Does Claude's native `AGENTS.md` read resolve `@imports`?~~ Answered: yes, per the Claude Code memory docs (checked 2026-10-02). The workspace's `@SOUL.md` lines load without a `CLAUDE.md` link.
4. Should `index = commit` be suggested by `init` for repos whose teams use cloud agents?
5. Should `codex` join `init`'s default targets, now that it has a working channel? The override then appears in every new project.
6. A follow-up spec for skills: `.agents/skills` is native to Codex, Cursor, and opencode, so `.cursor/skills` and `.codex/skills` links may double-register skills.
7. Should the Windsurf local fold move to `.devin/rules`, the vendor's preferred path?
8. Global sync still ignores `~/.agents/config` `targets=`. The home gate covers most of the risk. Should it read the config too?

## Confidence

**High (checked):**
- Every repo file:line and LOC figure comes from the grounding, re-read for the file sizes on 2026-10-01.
- The type sketch builds and passes `go vet` inside a scratch copy of `internal/agent`.
- E3, E12, and E13 were re-run on 2026-10-01: 7 of 15 imports; 31,000 and 45,930 bytes; 46 files, every generated line matched, outcomes 7 stub-only and 39 stub + `## Inherits`.
- `claude --version` prints `2.1.286 (Claude Code)`.
- E4 (AGENTS.md native read from 2.1.277, all sessions from 2.1.281, `CLAUDE.local.md` suppression, override and `.agents/` never read by Claude), from the Claude Code memory docs fetched 2026-10-01.
- The workspace `AGENTS.md` warning at lines 14-16.
- Codex override precedence, one file per directory, and the 32 KiB combined cap; Copilot paths and `applyTo`; Cursor `.mdc`-only with settings-only user rules; Windsurf 6,000/12,000; opencode `instructions`. All from vendor docs, read during the review.

**Low:**
- Symlink following at the Codex, Cursor, Copilot, and opencode read paths. *Missing context*: no docs, and no CLI installed here.
- OpenClaw and Windsurf counting UTF-16 units. *Extrapolating*: OpenClaw is TypeScript; Windsurf does not state its unit.
- Copilot coding agent reading `CLAUDE.md`. *Missing context*.
- Added-LOC estimates. *Extrapolating* from the sketch, ±25%.
- The latency of `claude --version`. *Missing context*: not measured.
