# Quarantine (remote content review)

Why remotely-fetched artifacts land in a scanned holding area by default, and how to review, approve, or reject them.

## The gate

Remote installs are treated like a hostile supply chain. By default,
everything `pull`/`update` fetches lands in `.agents/.quarantine/` —
invisible to `sync` and the index — after a static scan for:

- network-then-execute patterns (`curl | bash`),
- credential access combined with network calls,
- obfuscation (long base64, zero-width Unicode),
- prompt-injection phrasing aimed at your agent.

## Workflow

```bash
sync-agents pull                  # → 1 quarantined (run `sync-agents quarantine`)
sync-agents quarantine            # review findings per artifact
sync-agents approve code-review   # promote into .agents/ (blocked on CRITICAL unless --force)
sync-agents reject sketchy-rule   # delete without installing
```

## Escape hatches (loud by design)

- Critical findings block `approve`; overriding with `--force` is
  recorded in `sources.lock` as `approved_with_findings` so the
  decision is auditable.
- `--trust` on `pull`/`update` skips the gate for one invocation —
  findings still print.
- `quarantine = off` in `.agents/config` disables the gate for teams
  that review via pinned SHAs in PRs instead.

Local authoring (`add`, `promote`) never quarantines — the gate applies
to remote content only.

## Scope: placement, not execution

sync-agents never runs a skill. Its job ends at putting each artifact in
the right place for each harness. The safety work therefore sits at the
one point sync-agents controls, **install time**: fetch hardening plus
the quarantine gate above.

Running a skill's scripts safely belongs to the harness that executes
them. Claude Code, for example, has its own sandbox and tool
permissions. For untrusted third-party skills, isolate the harness
itself (run it in a container or VM) rather than expecting sync-agents
to wrap execution. A sync-agents exec sandbox (SPEC-005 Part C) was
considered and deliberately dropped for this reason.

## See also

- [Source manifest, lockfile & provenance](./sources.md)
- SPEC-005 (retired) — see the [spec ledger](../specs/README.md)
