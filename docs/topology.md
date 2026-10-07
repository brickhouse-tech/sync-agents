# Topology & configuration

How the `.agents/` source-of-truth tree is laid out, which buckets are optional, and how `config` and `STATE.md` fit in.

## The `.agents/` tree

`.agents/` is the source of truth. It contains all rules, skills,
workflows, and state for your agents:

```
.agents/
  ├── config              # sync targets and options (targets, index, claude-md, ...)
  ├── index/              # generated per-tool delivery files (cursor.mdc, copilot.md, ...)
  ├── rules/
  │   ├── rule1.md
  │   ├── rule2.md
  │   └── ...
  ├── skills/
  │   ├── skill1/
  │   │   └── SKILL.md
  │   ├── skill2/
  │   │   └── SKILL.md
  │   └── ...
  ├── workflows/
  │   ├── workflow1.md
  │   ├── workflow2.md
  │   └── ...
  ├── agents/              # optional: subagent definitions (Claude, Cursor, opencode)
  │   └── reviewer.md
  ├── plans/               # optional: per-effort implementation plans (how/when)
  │   └── auth-effort/
  │       └── rollout.md
  ├── specs/               # optional: durable design/requirements docs (what/why)
  │   └── SPEC-001.md
  ├── adrs/                # optional: Architecture Decision Records, status = subdirectory
  │   ├── proposed/
  │   │   └── adopt-grpc.md
  │   ├── accepted/
  │   │   └── use-postgres.md
  │   └── denied/          # kept so rejected decisions are not re-proposed
  │       └── use-mongo.md
  └── STATE.md
```

Running `sync-agents sync` creates symlinks from `.agents/`
subdirectories into `.claude/`, `.windsurf/`, `.cursor/`,
`.github/copilot/`, and, when enabled, `.codex/` and `.opencode/`.
Changes to `.agents/` show up in those directories at once, because they
are symlinks, not copies.

Rules reach Cursor, Copilot, Codex, and opencode through generated files
in `.agents/index/`, which `sync`, `index`, and `watch` rebuild. See
[delivery channels](./architecture/delivery-channels.md).

`AGENTS.md` at the project root is your file. `init` writes a short
stub when there is none; sync-agents never writes it after that.
`CLAUDE.md -> AGENTS.md` is created only when the installed Claude Code
needs it (see the
[CLAUDE.md policy](./architecture/delivery-channels.md#claudemd-policy)).

## Skills use a directory layout

Skills use a directory layout (`skills/name/SKILL.md`) rather than flat
files. This allows skills to include supporting files alongside their
definition. The [`fix` command](./commands/fix.md) can convert legacy
flat skill files to the directory layout automatically.

## Optional buckets

`agents/`, `plans/`, `specs/`, `hooks/`, and `adrs/` activate only when
their directory exists — `init` does not create them,
`add agent|plan|spec|hook|adr <name>` does.

`plans/`, `specs/`, `hooks/`, and `adrs/` are Claude-only
(`.claude/plans`, `.claude/specs`, …). Other tools open them from
`.agents/` when asked.

`agents/` reaches every tool that has a **native subagent surface**
reading markdown with YAML frontmatter — Claude (`.claude/agents/`),
Cursor (`.cursor/agents/`), and opencode (`.opencode/agents/`,
`~/.config/opencode/agents/` at user scope). Windsurf, Copilot, and
Codex have no subagent concept, so they are skipped rather than sent a
mislabeled artifact.

Frontmatter is **not translated** between harnesses. Claude's `tools:`
and `model:` sit alongside Cursor's `readonly:` and `is_background:` in
the same file, and each tool ignores the keys it does not recognize.
That is also why sync-agents' own routing keys are namespaced — a bare
`tools:` would collide with Claude's tool-permission allowlist and
silently strip the subagent's access.

`plans/` and `specs/` share plumbing but differ in lifecycle: specs are
durable what/why documents, plans are per-effort how/when documents
that retire when the effort lands.

Any bucket may also carry OS-scoped subdirectories (`macos/`, `linux/`,
`unix/`, `windows/`) — see [OS-scoped routing](./os-scoped-routing.md).

## STATE.md

`.agents/STATE.md` tracks the current state of your project from the
agent's perspective. It serves as a resumption point after failures or
interruptions — the agent can read `STATE.md` to determine where it
left off and what tasks remain. Update it regularly to keep agents in
sync with progress.

## Configuration (`.agents/config`)

`sync-agents init` creates `.agents/config` with default sync targets:

```
# sync-agents configuration
# Comma-separated list of sync targets (available: claude, windsurf, cursor, copilot, codex, opencode)
# Override per-command with: sync-agents sync --targets claude,cursor
targets = claude,windsurf,cursor,copilot
# index = local      # local (default: .agents/index/ and its links are gitignored) | commit
# claude-md = auto   # auto (default: link CLAUDE.md -> AGENTS.md only for Claude Code < 2.1.281) | link | off
```

Edit this file to limit which targets `sync` writes to by default. The
`--targets` flag on any command overrides the config.

Other recognized keys:

- `quarantine = on|off` — disable the remote-install quarantine gate
  (default `on`); see [Quarantine](./quarantine.md).
- `os = <goos>` — override the detected OS for testing/cross-compile
  CI; see [OS-scoped routing](./os-scoped-routing.md).
- `index = local|commit` — whether `.agents/index/` and its links are
  gitignored (default) or committed; see
  [delivery channels](./architecture/delivery-channels.md#index-policy).
- `claude-md = auto|link|off` — when sync creates `CLAUDE.md ->
  AGENTS.md`; see the
  [CLAUDE.md policy](./architecture/delivery-channels.md#claudemd-policy).

## See also

- [Command reference](./commands/README.md)
- [Scope and target directories](./architecture/scope-and-targets.md)
- [Semantic routing](./architecture/semantic-routing.md)
- [Delivery channels](./architecture/delivery-channels.md)
- [ADRs](./adrs.md)
