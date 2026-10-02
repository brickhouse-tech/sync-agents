# Migrating to sync-agents 2.0

What changes when you upgrade from 1.x, what the first run does to your
files, and how to adjust the result.

## Do this after upgrading

1. In each project, run `sync-agents sync`. Read the `AGENTS.md:` line
   it prints.
2. Commit the migrated `AGENTS.md` and the new `.gitignore` lines.
3. If you use opencode or Codex and their config file already exists,
   run `sync-agents sync --targets opencode` (or `codex`) once. See
   [Consent](#consent-for-files-you-own).
4. Run `sync-agents global sync`. Then run `sync-agents global status`
   and check for `unmounted` rows.
5. If a tool watches your files for integrity (a PLEX baseline, a
   checksum, a review bot), acknowledge the one-time `AGENTS.md`
   rewrite there.

## Why

The 1.x `AGENTS.md` was a list of links into `.agents/`. No tool
follows those links, so the list delivered no rule content. Claude
already loaded rules from `.claude/rules/`, which made the `@`-import
block redundant. The per-tool paths for Cursor, Copilot, and Codex were
paths those tools do not read. And the generator deleted every line you
wrote outside `## Inherits` and `## Tools`.

In 2.0, `AGENTS.md` is yours. Every tool gets rule bodies through a path
its vendor documents. See
[delivery-channels.md](architecture/delivery-channels.md).

## Breaking changes

### Project scope

1. **`AGENTS.md` is no longer generated.** A generated file is rewritten
   once to a short stub plus the sections you kept (see
   [The AGENTS.md rewrite](#the-agentsmd-rewrite)). `init` writes the
   stub only when `AGENTS.md` does not exist.
2. **`sync-agents index` regenerates `.agents/index/`**, the per-tool
   delivery files, not `AGENTS.md`. Old pre-commit hooks that call
   `sync-agents index` keep working.
3. **The `inherit` command is removed.** A hidden stub prints where to
   go. `## Inherits` is plain text you edit by hand. See
   [inheritance.md](inheritance.md).
4. **`import: true` does nothing.** Plans, specs, and ADRs with this
   frontmatter are no longer preloaded into Claude. `sync-agents lint`
   reports each one (W201). To make the content always-on, move it to
   `rules/`.
5. **`CLAUDE.md -> AGENTS.md` is version-gated.** sync creates it only
   when `claude` is an active target and the installed Claude Code is
   older than 2.1.281, or when `CLAUDE.local.md` exists. An existing link
   stays. A real `CLAUDE.md` is never moved aside, `--overwrite`
   included. `fix` no longer relinks a `CLAUDE.md` symlink that points
   elsewhere, and `clean` no longer removes one.
6. **Rules are no longer folded into Cursor, Copilot, Codex, or
   opencode directories.** The `.cursor/rules`, `.github/copilot/rules`,
   `.codex/rules`, and `.opencode/rules` links are removed. In their
   place sync creates `.agents/index/`, the links
   `.cursor/rules/sync-agents.mdc`,
   `.github/instructions/sync-agents.instructions.md`, and
   `AGENTS.override.md`, and an `opencode.json` entry.
7. **`.gitignore` and the git hook change.** `init`'s block lists
   `.agents/index/` instead of the `!.codex/instructions.md` and
   `!.github/copilot/instructions.md` exceptions. sync appends the link
   paths. The `git-hook` block is now `sync-agents sync` plus
   `git add -- .agents/index`, and re-running `git-hook` replaces an old
   block in place.

### User scope

8. **Global delivery paths moved.**
   - Copilot: `~/.copilot/instructions/sync-agents.instructions.md`,
     not `~/.github/copilot/instructions.md`.
   - Codex: the `codex-rules` region of `~/.codex/AGENTS.md`, not
     `~/.codex/instructions.md`.
   - Cursor: the `~/.cursor/rules/*.md` links are removed. Cursor has
     no user-rules file; your user rules reach it through each project's
     `cursor.mdc`.
   - Windsurf: `global_rules.md` holds a `codeium-rules` region. Text
     outside it is kept.
   - opencode: an entry in `~/.config/opencode/opencode.json`.
   - OpenClaw: unchanged (`openclaw-rules` region, SPEC-012 consent).
9. **`global sync` no longer creates a home for an uninstalled tool**
   (Windsurf, Copilot, Codex, opencode, OpenClaw). It edits an existing
   file of yours only after one `--targets <tool>` run.
10. **The `claude-imports` block is removed** from `~/.claude/CLAUDE.md`
    and from `AGENTS.md`. Your other lines in `~/.claude/CLAUDE.md` are
    kept; the file is removed only if nothing else is left.
11. **`global status` prints one channel row per tool** instead of
    concat and region rows. A link conflict makes `global sync` exit
    non-zero.

## What you see on the first run

A project with a 1.x `AGENTS.md`, `.cursor/rules` and `.codex/rules`
folds, and Claude Code 2.1.286:

```text
[info] AGENTS.md: replaced the generated header with a short pointer to .agents/;
       removed Rules, Skills, Workflows, State; removed the claude-imports block.
       Backup: .agents/.sync/AGENTS.md.pre-spec013. sync-agents will not rewrite this file again.
[info] Removed: .codex/rules
[info] Removed: .cursor/rules
[info] Removed legacy rule links: rules now reach Cursor, Copilot, Codex, and opencode through .agents/index/
[info] cursor   .cursor/rules/sync-agents.mdc -> .agents/index/cursor.mdc (1 rules)
[info] copilot  .github/instructions/sync-agents.instructions.md -> .agents/index/copilot.md (1 rules)
[info] codex    AGENTS.override.md -> .agents/index/codex.md (1 rules, 491 of 32768 bytes)
[info] CLAUDE.md not created: Claude Code 2.1.286 reads AGENTS.md natively (>= 2.1.281)
[info] Added 8 entries to .gitignore
```

The second run prints no `AGENTS.md:` line and removes nothing.

## The AGENTS.md rewrite

The migration runs in `sync`, `fix`, `index`, and every command that
used to regenerate the index. It runs once per file.

- **Which files.** Only a file whose first non-blank lines are the old
  generated header (bash 0.1 through Go 1.10) is treated as generated.
  The `claude-imports` block is stripped from any `AGENTS.md`.
- **What is removed.** The header becomes a 4-line stub that points at
  `.agents/`. A `## Rules`, `Skills`, `Workflows`, `Agents`, `Plans`,
  `Specs`, `ADRs`, `Hooks`, or `State` section is removed only when every
  line in it is one the old generator could have written.
- **What is kept**, byte for byte and in order: `## Inherits`,
  `## Tools`, unknown sections, a generated-titled section that holds a
  line you wrote, and other `sync-agents:*` regions such as
  `openclaw-rules`.
- **Backup.** Before the first rewrite, the old file is copied to
  `.agents/.sync/AGENTS.md.pre-spec013`. The backup is never
  overwritten. To undo, copy it back.
- **Concurrency.** The write is a compare-and-swap. If the file changes
  between read and write, sync leaves it and warns; the next run
  retries.
- `--dry-run` prints `would migrate AGENTS.md` and writes nothing.

`AGENTS.md` is rewritten once, and the backup is at
`.agents/.sync/AGENTS.md.pre-spec013`. If an integrity monitor tracks
`AGENTS.md`, the rewrite shows up as drift; acknowledge it once. If your
`## Inherits` section carries a comment saying the index deletes
everything else, that comment is now false and you can remove it.

Limit: when `AGENTS.md` is itself a symlink, the migration rewrites the
file it points to.

## Legacy files and the proofs used

sync removes an old placement only when it can prove sync-agents made
it. Anything else is reported and left.

| Scope | Path | Removed when |
|---|---|---|
| project | `.cursor/rules`, `.github/copilot/rules`, `.codex/rules`, `.opencode/rules` | it is a symlink into `.agents/rules`. In a real directory, only the per-rule links into `.agents/rules` are removed, and the directory goes only if that empties it |
| user | `~/.cursor/rules/<name>.md` | it is a symlink into `~/.agents/` |
| user | `~/.github/copilot/instructions.md`, `~/.codex/instructions.md` | it is a regular file that starts with the old generated banner |
| user | `~/.claude/CLAUDE.md` | it carries the `claude-imports` block; only the block is removed |
| user | Windsurf `global_rules.md` | it starts with the old banner: rewritten in place as the `codeium-rules` region, whose markers then carry consent |

A non-bannered `instructions.md` is reported on every run and kept.

## Opting out and adjusting

### CLAUDE.md

`.agents/config`:

```ini
claude-md = auto   # default: link only when Claude Code needs it
claude-md = link   # always keep CLAUDE.md -> AGENTS.md (the 1.x behavior)
claude-md = off    # never touch CLAUDE.md
```

If `claude` is not on `PATH` (a git hook, CI), `auto` cannot read the
version, changes nothing, and warns. Set `link` or `off` to decide
without the probe. If you keep a real `CLAUDE.md`, add `@AGENTS.md` to it
so Claude loads both.

### Committing the delivery files

```ini
index = commit
```

Fresh clones and cloud agents then get delivery with no setup. Two
one-time steps (see
[delivery-channels.md](architecture/delivery-channels.md#index--commit-caveats)):

1. Delete the `.agents/index/` line from `.gitignore`.
2. Run `git add -f .cursor/rules/sync-agents.mdc`.

### Consent for files you own

sync creates `opencode.json`, `~/.codex/AGENTS.md`, and Windsurf's
`global_rules.md` when they do not exist (at user scope, only when the
tool's home directory exists). When one exists with your
content, sync needs one run that names the tool:

```bash
sync-agents sync --targets opencode
sync-agents global sync --targets codex
sync-agents global sync --targets codeium
```

After that, the region markers or the config entry carry consent. Until
then, `status` shows the channel as `unmounted`. Listing a tool in
`.agents/config` `targets` is not consent.

### .gitignore lines you can delete

The 1.x lines `!.codex/instructions.md` and
`!.github/copilot/instructions.md` re-included files no tool reads.
Delete them.

### Rules over a tool's size limit

Rules that do not fit Codex's 32 KiB, OpenClaw's 20,000 chars, or
Windsurf's 6,000 chars become one-line pointers, and sync names them.
Raise the limit with the knob sync prints, or mark rules that apply only
to some tasks with `trigger: model_decision`.

### Codex and your own AGENTS.override.md

If a project already has a real `AGENTS.override.md`, Codex project
delivery is a `conflict` until you move the file aside. A non-empty
`~/.codex/AGENTS.override.md` makes the user channel `shadowed`.

## See also

- [delivery-channels.md](architecture/delivery-channels.md): the
  per-tool delivery table, budgets, and the CLAUDE.md policy
- [`sync`](commands/sync.md), [`index`](commands/index.md),
  [`global sync`](commands/global-sync.md), [`lint`](commands/lint.md)
- [inheritance.md](inheritance.md): `## Inherits` after 2.0
- SPEC-013 (AGENTS.md is not an index)
