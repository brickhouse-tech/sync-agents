# sync-agents

**One `.agents/` directory. Every AI coding assistant. Always in sync.**

`sync-agents` is a package manager and sync engine for AI-agent context — the rules, skills, workflows, subagents, plans, specs, and ADRs you feed to Claude, Cursor, Windsurf, and Copilot. Write everything once in `.agents/`, and `sync-agents` fans it out to every tool via symlinks, keeps an `AGENTS.md` index current, and lets you pull shared context from other repos as safely and reproducibly as you'd install an npm package.

## Why you want this

If you use more than one AI coding tool — or more than one repo — you already know the pain: the same rules copy-pasted into `.claude/`, `.cursor/`, `.windsurf/`, and `.github/copilot/`, drifting apart the moment you edit one of them. Team conventions living in a wiki nobody's agent reads. A "cool skill" pasted from the internet with zero review of what it actually tells your agent to do.

`sync-agents` fixes all three:

- **Zero drift.** `.agents/` is the single source of truth; tool directories are symlinks into it. Edit once, every tool sees it instantly.
- **Real distribution.** Declare upstream rules/skills/whole trees in `sources.yaml`, get SHA-pinned, lockfile-verified installs — a package manager, not a copy-paste culture.
- **Supply-chain safety by default.** Everything fetched remotely is statically scanned and quarantined until you approve it. Your agent's instructions are an attack surface; treat them like one.

## What is this, in simple terms?

Think of the instructions you give AI coding assistants as software. This tool is **npm for your AI's brain**.

- Your project gets one folder (`.agents/`) that holds everything you've taught your AI: house rules, reusable how-to guides, step-by-step playbooks.
- Every AI tool you use reads from that one folder — like several TVs all tuned to the same channel. Change the channel once, every screen updates.
- Want a rule or skill someone else published? "Install" it like an app from an app store, pinned to an exact version so it can't silently change under you.
- And just like you wouldn't run a random downloaded program without a virus scan, anything you install from the internet is scanned and held in a waiting room until you say it's okay.

No more copy-pasting the same instructions into four different tools, and no more wondering where that one config file came from.

## Install

```bash
# npm — ships prebuilt native binaries, no Go toolchain needed
npm install -g @brickhouse-tech/sync-agents

# Homebrew
brew install brickhouse-tech/tap/sync-agents

# go install (Go 1.21+, no Node needed)
go install github.com/brickhouse-tech/sync-agents@latest
```

Pre-built archives (with SHA-256 checksums) are also on the [Releases page](https://github.com/brickhouse-tech/sync-agents/releases). More detail in [docs/install.md](docs/install.md).

## 60-second quickstart

```bash
cd your-project

# 1. Create .agents/ (rules/, skills/, workflows/, STATE.md) + AGENTS.md index
sync-agents init

# 2. Add your first rule and skill
sync-agents add rule no-secrets
sync-agents add skill code-review

# 3. Fan everything out to .claude/, .windsurf/, .cursor/, .github/copilot/
sync-agents sync

# 4. Install a shared skill from another repo (SHA-pinned + scanned + quarantined)
sync-agents source add skill:anthropic/skill-pack@v1.2.0/skills/code-review
sync-agents quarantine          # review the scan findings
sync-agents approve code-review # promote it into .agents/

# 5. Check that everything is wired up
sync-agents status
```

That's it — every supported tool now reads the same rules, and `AGENTS.md` (symlinked to `CLAUDE.md`) indexes it all automatically.

## Killer features

### 📦 Seven buckets, one tree
Rules, skills, workflows, subagents, plans, specs, and ADRs each get a first-class home under `.agents/`, routed to the right place per tool (ADRs even track proposed/accepted/denied status by directory). [Topology →](docs/topology.md)

### 🔒 Declarative sources with a lockfile
`sources.yaml` + `sources.lock`: every upstream artifact resolves to a commit SHA, downloads are content-hashed, and a tampered tarball aborts before anything touches disk. `pull`, `update`, `source list` — reproducible like a real package manager. [Sources →](docs/sources.md)

### 🛡️ Quarantine + static scanning, on by default
Remote installs are treated like a hostile supply chain. Fetched artifacts are scanned for `curl | bash` patterns, credential-access-plus-network combos, obfuscation, and prompt injection — then held in `.agents/.quarantine/` until you `approve` them. [Quarantine →](docs/quarantine.md)

### 🔗 Linked (editable) sources — *new in 1.4.0*
`npm link` for agent context: point a source at a live local checkout instead of a frozen snapshot. Edits flow both ways, `git pull` reaps updates, and the link is recorded declaratively so it survives clones. [Linked sources →](docs/linked-sources.md)

### 🖥️ OS-scoped routing
Drop rules into `rules/macos/`, `rules/linux/`, or `rules/unix/` and they only sync on matching machines. One committed tree, no brew-rules noise on your Linux box. [OS routing →](docs/os-scoped-routing.md)

### 🗂️ AGENTS.md auto-index
Every artifact is indexed into a generated `AGENTS.md` (symlinked to `CLAUDE.md`), with descriptions pulled from frontmatter, passive rules `@`-imported for Claude, and an `## Inherits` section for org → team → project rule hierarchies. [Index →](docs/commands/index.md) · [Inheritance →](docs/inheritance.md)

### ✅ Lint for skill compliance
`sync-agents lint` validates every `SKILL.md` against Claude's published authoring rules (name format, description quality, reserved words) and `--fix` mechanically repairs what it can. CI-friendly exit codes. [Lint →](docs/commands/lint.md)

### 🌍 Global scope too
`promote` an artifact from a project to your user-level `~/.agents/`, then `global sync` fans it out to every tool's *user* config dir with semantic-aware routing. [Global commands →](docs/commands/global-sync.md)

## Documentation

The full manual lives in [`docs/`](docs/README.md):

- [Command reference](docs/commands/README.md) — every command and flag
- [Topology & configuration](docs/topology.md) — the `.agents/` tree, `config`, `STATE.md`
- [Sources, lockfile & provenance](docs/sources.md) · [Quarantine](docs/quarantine.md) · [Linked sources](docs/linked-sources.md)
- [Inheritance](docs/inheritance.md) · [ADRs](docs/adrs.md) · [OS-scoped routing](docs/os-scoped-routing.md)
- [Architecture](docs/README.md#architecture) — scope resolution, semantic routing, global roots
- [Examples](examples/README.md) — ready-to-import rules, skills, and workflows

## License

[MIT](LICENSE) © Brickhouse Tech
