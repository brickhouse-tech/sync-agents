# Source manifest, lockfile & provenance

Declarative, reproducible installation of `.agents/` artifacts from upstream GitHub repositories — the package-manager layer of sync-agents.

## The manifest

Declare upstream rules, skills, workflows — or whole `.agents/` trees —
in `.agents/sources.yaml` and install them reproducibly:

```yaml
version: 1
sources:
  - skill:anthropic/skill-pack@v1.2.0/skills/code-review
  - rule:my-org/agent-norms@main/rules/security.md
  - tree:my-org/team-agents@v2.0.0   # fans out to every bucket in the upstream .agents/
```

## How `pull` works

`sync-agents pull` resolves each ref to a commit SHA via the GitHub
API, fetches the repo tarball (cached by SHA under
`$XDG_CACHE_HOME/sync-agents/`, no `git` binary needed), verifies a
deterministic sha256 content hash against `.agents/sources.lock`, and
only then installs — a tampered tarball or corrupted cache aborts
before anything is written.

Every installed artifact carries provenance (`_origin.json` inside
skill dirs, `<name>.origin.json` beside flat files) recording
owner/repo/ref/SHA/hash; commit these so clones keep their provenance.

## Safety rules

- Locally-edited artifacts are never overwritten without `--force`.
- A destination without origin metadata is treated as a manual
  conflict.
- Private repos use your token (`SYNC_AGENTS_GITHUB_TOKEN` >
  `GITHUB_TOKEN` > `GH_TOKEN` > `gh auth token`).
- All commands accept `--global` to operate on `~/.agents/` instead of
  the project.
- Everything fetched remotely passes through the
  [quarantine gate](./quarantine.md) before it can enter the live tree.

## Day-to-day commands

```bash
sync-agents source add skill:anthropic/skill-pack@v1.2.0/skills/code-review
sync-agents source list          # ok / outdated / modified / missing per entry
sync-agents update               # bump tag-tracked entries when upstream moves
sync-agents pull --offline       # cache-only, for CI or airplanes
sync-agents source bundle        # rebuild sources.yaml from installed origin metadata
sync-agents source detach NAME   # un-manage: flip to manual, drop the entry
```

## See also

- [Quarantine (remote content review)](./quarantine.md)
- [Linked (editable) sources](./linked-sources.md)
- [Command reference](./commands/README.md)
