# Topology & configuration

How the `.agents/` source-of-truth tree is laid out, which buckets are optional, and how `config` and `STATE.md` fit in.

## The `.agents/` tree

`.agents/` is the source of truth. It contains all rules, skills,
workflows, and state for your agents:

```
.agents/
  ├── config              # sync targets (claude, windsurf, cursor, copilot)
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
  ├── agents/              # optional: Claude subagent definitions
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
  │   └── denied/          # kept but never indexed — prevents re-proposing rejected decisions
  │       └── use-mongo.md
  └── STATE.md
```

Running `sync-agents sync` creates symlinks from `.agents/`
subdirectories into `.claude/`, `.windsurf/`, `.cursor/`, and
`.github/copilot/`. Any changes to `.agents/` are automatically
reflected in the target directories because they are symlinks, not
copies.

`AGENTS.md` is an auto-generated index of everything in `.agents/` and
is symlinked to `CLAUDE.md` so that Claude reads the index natively.

## Skills use a directory layout

Skills use a directory layout (`skills/name/SKILL.md`) rather than flat
files. This allows skills to include supporting files alongside their
definition. The [`fix` command](./commands/fix.md) can convert legacy
flat skill files to the directory layout automatically.

## Optional buckets

`agents/`, `plans/`, `specs/`, `hooks/`, and `adrs/` activate only when
their directory exists — `init` does not create them,
`add agent|plan|spec|hook|adr <name>` does. They sync to Claude only
(`.claude/agents`, `.claude/plans`, `.claude/specs`); other tools
consume plans/specs through the `AGENTS.md` index and have no subagent
surface.

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
# Comma-separated list of sync targets (available: claude, windsurf, cursor, copilot)
targets = claude,windsurf,cursor,copilot
```

Edit this file to limit which targets `sync` writes to by default. The
`--targets` flag on any command overrides the config.

Other recognized keys:

- `quarantine = on|off` — disable the remote-install quarantine gate
  (default `on`); see [Quarantine](./quarantine.md).
- `os = <goos>` — override the detected OS for testing/cross-compile
  CI; see [OS-scoped routing](./os-scoped-routing.md).

## See also

- [Command reference](./commands/README.md)
- [Scope and target directories](./architecture/scope-and-targets.md)
- [Semantic routing](./architecture/semantic-routing.md)
- [ADRs](./adrs.md)
