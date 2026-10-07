# Delivery channels

How each tool receives the content of your passive rules, at project
scope and at user scope, as implemented since SPEC-013.

## The short version

- `.agents/` is the source. Rules, workflows, and skills whose
  [semantic](semantic-routing.md) is passive are the content every tool
  should load.
- Claude and Windsurf read a rules directory natively. They get the
  folded `.claude/rules` and `.windsurf/rules` links, as before.
- Every other tool gets one generated file per tool, rendered into
  `.agents/index/` (project) or `~/.agents/index/` (user), and one
  mount at a path the vendor documents.
- `AGENTS.md` is your file. sync-agents rewrote it once, to remove the
  old link index, and does not write it again. See
  [migration-v2.md](../migration-v2.md).

A **channel** is how one tool receives aggregated passive content at one
scope. It has a format (the dialect of the generated file), a mount (how
the tool's read path reaches it), and an optional budget (the tool's
size limit).

## Per-tool delivery

| Tool | Scope | Tool reads | sync-agents writes | Mount | Size limit |
|---|---|---|---|---|---|
| Claude | project | `.claude/rules/*.md`; `CLAUDE.md`, else `AGENTS.md` | `.claude/rules -> ../.agents/rules`; `CLAUDE.md -> AGENTS.md` per the [CLAUDE.md policy](#claudemd-policy) | fold | none |
| Claude | user | `~/.claude/rules/*.md` | one link per rule into `~/.agents/rules/` | per artifact | none |
| Windsurf | project | `.windsurf/rules/*.md`, root `AGENTS.md` | `.windsurf/rules -> ../.agents/rules` | fold | 12,000 chars per file (vendor; not checked) |
| Windsurf | user | `~/.codeium/windsurf/memories/global_rules.md` | `~/.agents/index/codeium.md`; region `codeium-rules` | region | 6,000 UTF-16 units, whole file |
| Cursor | project | `.cursor/rules/*.mdc`, `AGENTS.md` | `.agents/index/cursor.mdc`; link `.cursor/rules/sync-agents.mdc` | link | none |
| Cursor | user | app settings only, no file | nothing (status shows a `gap` row) | none | n/a |
| Copilot | project | `.github/instructions/**/*.instructions.md`, `.github/copilot-instructions.md`, `AGENTS.md` | `.agents/index/copilot.md`; link `.github/instructions/sync-agents.instructions.md` | link | none |
| Copilot | user | `~/.copilot/instructions/**/*.instructions.md` | `~/.agents/index/copilot.md`; absolute link `~/.copilot/instructions/sync-agents.instructions.md` | link | none |
| Codex | project | per directory: `AGENTS.override.md`, else `AGENTS.md` | `.agents/index/codex.md`; link `AGENTS.override.md` | link | 32 KiB combined with the user file |
| Codex | user | `$CODEX_HOME/AGENTS.override.md`, else `AGENTS.md` | `~/.agents/index/codex.md`; region `codex-rules` in `$CODEX_HOME/AGENTS.md` | region | 32 KiB |
| opencode | project | `AGENTS.md`; files listed in `opencode.json` `instructions` | `.agents/index/opencode.md`; entry in `opencode.json` | config list | none |
| opencode | user | `~/.config/opencode/opencode.json` `instructions` | `~/.agents/index/opencode.md`; absolute-path entry | config list | none |
| OpenClaw | project | none (global-only target, SPEC-012) | nothing | none | n/a |
| OpenClaw | user | `<workspace>/AGENTS.md`, raw text | `~/.agents/index/openclaw.md`; region `openclaw-rules` | region | 20,000 UTF-16 units, whole file |

`$CODEX_HOME` defaults to `~/.codex`. The OpenClaw workspace comes from
`openclaw.json`, as in [global-sync.md](../commands/global-sync.md).

The generated files:

| File | Content |
|---|---|
| `cursor.mdc` | MDC frontmatter (`description`, `alwaysApply: true`), the banner, then one `## <name>` section per rule. Under `index = local` it also carries your user-level rules (Cursor has no user-rules file). A project rule wins over a user rule of the same name. |
| `copilot.md` | `applyTo: "**"` frontmatter, the banner, then the rules. |
| `codex.md` | The project's `AGENTS.md` with every `sync-agents:*` region removed, a note, the banner, then the rules. Codex reads the override instead of `AGENTS.md`, so the override has to carry `AGENTS.md`'s text. |
| `opencode.md`, `codeium.md`, `openclaw.md` | The banner, then the rules. Region bodies use the same bytes. |

Each rule section starts with `<!-- OS: <scope> -->` when the rule
lives in an OS-scoped subdirectory. Rules that do not fit the budget are
listed under `## Not inlined` as `- name (path): description`.

Skills, subagents, plans, specs, ADRs, and hooks do not go through
channels. Their per-tool routing is unchanged; see
[semantic-routing.md](semantic-routing.md).

## Mounts and consent

There are three mount kinds.

- **Link.** sync-agents owns a file name the tool reads and places a
  symlink to the index file there. Project links are relative; user
  links are absolute. A real file at that path is a conflict: sync warns,
  leaves it, and exits non-zero.
- **Region.** The tool reads one fixed file that others also edit. sync
  splices the content between `<!-- sync-agents:<name>:start -->` and
  `<!-- sync-agents:<name>:end -->`. Every byte outside the markers is
  kept.
- **Config list.** The tool loads files listed in a JSON array. A missing
  `opencode.json` is created with one entry. An existing strict-JSON
  file gets the entry by a byte-range insert that keeps formatting and
  key order. A JSONC or unparseable file is not edited: the channel shows
  `manual` and sync prints the line to add.

Consent rules, checked in this order:

| Situation | Result |
|---|---|
| Our link, region markers, or config entry is already there | refresh |
| User scope, the tool's home directory is missing, tool not named in `--targets` | skip: tool not installed |
| Region whose host the tool creates (OpenClaw), host missing | skip: run the tool once first |
| Region or config list, the file exists, tool not named in `--targets` | skip: run once with `--targets <tool>` |
| Otherwise | mount |

A link never needs consent, because the file name is ours. Creating a
file that did not exist edits nobody's bytes and needs no consent.
Listing a tool in `.agents/config` `targets` is not consent to edit a
file you own: it means "deliver to this tool". After one `--targets`
run the markers or the entry are in place, and later runs refresh them.

At user scope, an undeliverable channel is silent unless `--targets`
names the tool. `global status` shows it as `unmounted` with the reason.

## Budgets

| Tool | Limit | Unit | What counts | Knob |
|---|---|---|---|---|
| Codex, user | 32,768 | bytes | the whole `AGENTS.md`, including your text outside the region | `project_doc_max_bytes` in `$CODEX_HOME/config.toml` |
| Codex, project | 32,768 | bytes | the user-level `AGENTS.md` plus the override | same |
| OpenClaw | 20,000 | UTF-16 units | the whole workspace `AGENTS.md` | `agents.defaults.bootstrapMaxChars` in `openclaw.json` |
| Windsurf, user | 6,000 | UTF-16 units | the whole `global_rules.md` | none (fixed by the vendor) |

Cursor, Copilot, and opencode document no limit, so their files inline
every rule.

Fitting is first fit in name order. A rule is inlined when the fixed
part, the rules already inlined, this rule, and the pointers still owed
for the rest all fit. Otherwise it becomes a pointer and the walk goes
on, so a later, smaller rule can still fit. No tool ever cuts a rule in
the middle, because the file stays under the limit.

- `trigger: model_decision` in a rule's frontmatter makes it a pointer in
  every capped file. Uncapped files inline it. Claude and Windsurf still
  read the source file natively.
- When the limit forces a rule out, sync warns with the rule names and
  the knob.
- When even the all-pointer form is over the limit, sync warns that the
  tool will truncate.

## Index policy

`.agents/config` key `index`:

| Value | `.agents/index/` and link paths | OS scopes compiled | User rules in `cursor.mdc` |
|---|---|---|---|
| `local` (default) | gitignored | this machine's only | yes |
| `commit` | committed | every scope, with `<!-- OS: -->` headers | no |

Under `local`, a fresh clone has neither the index nor the links, so no
link dangles. Tools read `AGENTS.md` alone until someone runs `sync`.
Under `commit`, cloud agents (Codex cloud, Copilot coding agent, Cursor
background agents) get delivery with no setup, and the committed bytes
are the same on every contributor's machine.

## .gitignore

`init` writes this block:

```gitignore
# sync-agents — ignore tool artifacts, keep symlinks
.cursor/*
!.cursor/rules
.codex/*
.github/copilot/*
.agents/index/
```

`!.cursor/rules` keeps your own committed `.mdc` rules visible to git.
`sync` then appends exact lines when they are missing:

- one `<tool dir>/` line per active target (`.claude/`, `.windsurf/`,
  `.cursor/`, `.github/copilot/`, `.codex/`, `.opencode/`);
- under `index = local`: `.agents/index/` and each link path it placed
  (`.cursor/rules/sync-agents.mdc`,
  `.github/instructions/sync-agents.instructions.md`,
  `AGENTS.override.md`). A link path held by a real file is yours and is
  not ignored;
- `CLAUDE.md` when it is, or is about to be, our symlink.

`opencode.json` is never ignored: it can hold your own settings.

### `index = commit` caveats

1. `init` does not read `index`, so its block still ignores
   `.agents/index/`. sync warns about this line. Delete it once; sync
   does not add it back under `commit`.
2. The `.cursor/` target line ignores `.cursor/rules/sync-agents.mdc`,
   and sync re-adds the line if you delete it. sync does not warn about
   this. Track the link once with
   `git add -f .cursor/rules/sync-agents.mdc`. A tracked file is not
   affected by `.gitignore`, and the link target never changes.

## CLAUDE.md policy

`.agents/config` key `claude-md`: `auto` (default), `link`, or `off`.
Rows are checked in order.

| Condition | Action |
|---|---|
| `claude` is not an active target | nothing |
| `claude-md = off` | nothing |
| `CLAUDE.md` is a real file | keep it; warn that Claude reads it instead of `AGENTS.md`, and suggest adding `@AGENTS.md` to it |
| `CLAUDE.md` is a symlink that does not resolve to `AGENTS.md` | keep it; warn |
| no `AGENTS.md` | nothing (a link would dangle) |
| `claude-md = link` | ensure `CLAUDE.md -> AGENTS.md` |
| `auto`, `CLAUDE.local.md` exists in the project root | ensure the link |
| `auto`, a `CLAUDE.md`, `.claude/CLAUDE.md`, or `CLAUDE.local.md` in a parent directory (not `~/.claude/CLAUDE.md`) | ensure the link; the reason names the parent file |
| `auto`, `claude --version` < 2.1.281 | ensure the link |
| `auto`, version >= 2.1.281, our link exists | remove our link (status `[stale]` until sync runs) |
| `auto`, version >= 2.1.281, no link | create nothing |
| `auto`, version unknown | change nothing; warn once, naming the error and the `claude-md` key |

- The probe runs `claude --version` with a 5 s timeout, once per
  command, and only when the answer depends on it.
- "Unknown changes nothing" is deliberate. `claude` is often on a
  terminal's `PATH` but not on a git hook's. Acting on unknown would make
  the two runs undo each other.
- sync never moves or deletes a real `CLAUDE.md` or a foreign symlink,
  `--overwrite` included. The only `CLAUDE.md` sync removes is its own
  link to `AGENTS.md`, re-checked just before removal.
- Why remove our link on a native Claude: any `CLAUDE.md` stops Claude
  from reading `AGENTS.md` in that directory and every directory below
  it. A `CLAUDE.md -> AGENTS.md` link at the project root hides nested
  `AGENTS.md` files in subdirectories; one in a home directory that is
  itself a project hides `AGENTS.md` in every project under it.

### What Claude reads

From the Claude Code memory docs
(<https://code.claude.com/docs/en/memory>, checked 2026-10-02):

- Claude Code v2.1.277 and later read `AGENTS.md` when there is no
  `CLAUDE.md`, `.claude/CLAUDE.md`, or `CLAUDE.local.md` in the working
  directory or above it. "Before v2.1.281, some sessions, such as those
  on Amazon Bedrock or with telemetry disabled, read `CLAUDE.md` files
  only." That is why the threshold is 2.1.281.
- `CLAUDE.local.md` counts as a `CLAUDE.md` for that check, so it stops
  Claude from reading `AGENTS.md`. That is why `auto` links when one
  exists. The Claude setting **Project instructions** =
  `claude-md-and-agents-md` is the alternative.
- The check walks up the tree, so a `CLAUDE.md` in any parent directory
  also stops Claude reading a project's `AGENTS.md`. Verified live on
  2026-10-06: an `AGENTS.md` under a home directory holding
  `CLAUDE.md -> AGENTS.md` was not loaded; the same file under `/tmp`
  was. That is why `auto` links when a parent directory has one.
- `~/.claude/CLAUDE.md`, a managed `CLAUDE.md`, and `.claude/rules/`
  files do not count, and load alongside `AGENTS.md`.
- `@path` imports inside an `AGENTS.md` are expanded.
- Claude does not read `AGENTS.local.md`, `AGENTS.override.md`, or
  anything under a `.agents/` directory. No channel writes rule bodies
  to a path Claude reads, so Claude does not get the rules twice.
- `.claude/rules/` supports symlinks. A symlink whose target is outside
  the working directory is treated like an external import and needs
  approval. The project fold points inside the project. User-level
  `~/.claude/rules/` is trusted without approval.
- A `CLAUDE.md -> AGENTS.md` symlink is read once.

## $HOME as a project

When a project's `.agents/` is the global root (the project is `$HOME`),
Copilot, Codex, and opencode skip their project channel. `global sync`
owns that tree's delivery for them, so they do not get the same rules
twice. `status` shows them as `skipped`. Cursor has no user channel, so
it keeps its project channel, without merging user rules (the trees are
the same).

## Refresh and mount

- `index`, `watch`, `add`, `adr`, `import`, and the source commands
  refresh: they rewrite files inside `.agents/index/` and re-splice
  regions and entries that already exist. They create nothing in a tool
  directory.
- `sync`, `fix`, and `global sync` mount: they also remove legacy
  placements and create or repair mounts where consent allows.
- `watch` ignores `.agents/index/` and watches `AGENTS.md`, because the
  Codex override copies it.

## Status states

`status` and `global status` print one row per channel, computed by
re-rendering and comparing bytes, so `synced` means sync would write
nothing.

| State | Meaning |
|---|---|
| `synced` | the index matches the render and the mount reaches it |
| `stale` | sources or `AGENTS.md` changed since the last run |
| `unmounted` | the mount is absent; the detail says why (consent, not installed, never synced) |
| `conflict` | a real file holds the link path |
| `shadowed` | the mount is in place, but the tool reads another file first (a non-empty `$CODEX_HOME/AGENTS.override.md`) |
| `manual` | the config is JSONC or unparseable; the detail holds the entry to add |
| `skipped` | project channel left to `global sync` ($HOME as a project) |
| `gap` | the tool has no file to deliver to (Cursor, user scope) |
| `error` | rendering failed; the detail holds the error |

## Verified and unverified

| Claim | Status |
|---|---|
| Claude loads `.claude/rules/*.md` and `~/.claude/rules/*.md`, including symlinked rule files | verified live, Claude Code 2.1.286 |
| Claude reads `AGENTS.md` natively from 2.1.277, in every session from 2.1.281 | Claude docs |
| Each vendor read path in the delivery table | vendor docs |
| Cursor follows a symlinked `.mdc` | **unverified**. Cursor 2.2.17 broke symlinked `.mdc` and 2.5 fixed it (Cursor forum) |
| Copilot (VS Code and CLI) follows a symlinked `.instructions.md` | **unverified** |
| Codex follows a symlinked `AGENTS.override.md` | **unverified** |
| opencode loads an `instructions` entry that is a symlink, and ignores a missing one | **unverified** |
| OpenClaw and Windsurf count UTF-16 units | inferred: OpenClaw is TypeScript; Windsurf does not state its unit |

None of the Cursor, Copilot, Codex, or opencode CLIs was installed where
this was built, so the four rows above stay unverified until someone
checks each tool. If a tool does not follow the link, its
`channelSpecs` row changes to write the rendered bytes at the native
path instead of a symlink.

## Open follow-ups

Carried over from SPEC-013 when it shipped in 2.0.0:

- **Per-tool symlink check.** Confirm the four unverified rows above,
  and that opencode treats a missing `instructions` file (a fresh clone
  with a committed `opencode.json`) as a no-op.
- **Simplify pass.** Production Go grew by about 2.3k lines net in
  2.0.0, mostly new delivery code. `deliver*.go` and `channel.go` are
  the candidates.
- **Copilot and `CLAUDE.md`.** The Copilot coding agent may also read a
  root `CLAUDE.md`. Where sync links `CLAUDE.md -> AGENTS.md`, Copilot
  could read `AGENTS.md` twice. Unverified.
- **`init` defaults.** Should `init` suggest `index = commit` for teams
  using cloud agents, and add `codex` to the default targets?
- **Skills double-registration.** `.agents/skills` is native to Codex,
  Cursor, and opencode, so the `.cursor/skills` and `.codex/skills`
  links may register skills twice.
- **Windsurf path.** Should the local fold move to `.devin/rules`, the
  vendor's preferred path?
- **Global `targets=`.** Global sync ignores `~/.agents/config`
  `targets=`; the home gate covers most of the risk.

## See also

- [migration-v2.md](../migration-v2.md): what changed in 2.0.0 and what
  to do
- [semantic-routing.md](semantic-routing.md): passive versus invocable
  routing for every bucket
- [scope-and-targets.md](scope-and-targets.md): tool IDs and per-scope
  directories
- [`sync`](../commands/sync.md), [`index`](../commands/index.md),
  [`global sync`](../commands/global-sync.md)
- SPEC-013 (AGENTS.md is not an index), SPEC-012 (OpenClaw global
  target), SPEC-002 (global scope)
