# Specs — ledger and lifecycle

`specs/` contains **active proposals only**: work that is drafted, in
flight, or partially shipped. Fully-shipped specs do not live here —
their durable content is promoted into [`docs/`](../docs/README.md) and
the spec body is deleted in the same PR (git history is the archive).
This ledger is the permanent index: every spec that ever existed has a
row, so SPEC IDs stay resolvable after the file is gone.

**Precedence:** `docs/` and code are current truth. A spec — active,
retired, or found in git history — never outranks them. Agents: do not
treat retired spec content as current intent.

## Lifecycle

1. **Draft → Active** — spec lives here, frontmatter `status:` tracks it.
2. **Shipped (fully)** — in one PR: promote durable decisions/contracts
   into `docs/`, delete the spec file, update this ledger row (status,
   retire commit, doc destinations).
3. **Shipped (partially)** — trim the spec to only the open work
   (collapse shipped parts to a short summary), update `status:`.
4. Recover any retired body verbatim:
   `git log --all --oneline -- specs/` then `git show <sha>^:specs/<file>`.

## Ledger

| ID | Title | Status | Retired in | Durable docs |
| --- | --- | --- | --- | --- |
| SPEC-001 | go-install first-class (goreleaser) | ✅ shipped v1.4.0 | `6c53595` | [install](../docs/install.md) |
| SPEC-002 | promote + global sync (semantic routing) | ✅ shipped v1.4.0 | `6c53595` (siblings merged in `9de9326`) | [promote](../docs/commands/promote.md), [global-sync](../docs/commands/global-sync.md), [semantic-routing](../docs/architecture/semantic-routing.md) |
| SPEC-003 | source manifest pull (`sources.lock`) | ✅ shipped v1.4.0 | `6c53595` | [sources](../docs/sources.md) |
| SPEC-004 | new asset buckets (agents/plans/specs/hooks) + lint | ✅ shipped v1.4.0 | `6c53595` | [topology](../docs/topology.md), [lint](../docs/commands/lint.md) |
| SPEC-005 | sandboxing + quarantine | 🟡 Parts A+B shipped v1.1.0–v1.2.0; **Part C open** | — (active) | [quarantine](../docs/quarantine.md) |
| SPEC-006 | OS-scoped routing | 🟡 core shipped v1.3.0; **badge + concat headers open** | — (active) | [os-scoped-routing](../docs/os-scoped-routing.md) |
| SPEC-007 | linked sources | ✅ shipped v1.4.0 | `6c53595` | [linked-sources](../docs/linked-sources.md) |
| SPEC-008 | full-context integrity lock (`agents.lock` + `agents.sum`) | 📝 draft | — (active) | — |
| SPEC-009 | spec lifecycle tooling (`lint` spec checks + `spec retire`) | 📝 draft | — (active) | — |

Next spec ID: **SPEC-010**.
