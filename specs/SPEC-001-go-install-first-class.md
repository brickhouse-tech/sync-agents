---
id: SPEC-001
title: Make `go install` a first-class install path for sync-agents
status: Draft
owner: nmccready
created: 2026-05-13
updated: 2026-05-13 (rev 3: aligned with existing CI/release infra at v0.2.4)
---

# [SPEC-001] Feature: First-class `go install` + goreleaser for sync-agents

## Overview

Restructure the repository so that

```
go install github.com/brickhouse-tech/sync-agents@latest
```

resolves and installs the `sync-agents` binary directly, with no subpath. Adopt
[goreleaser](https://goreleaser.com) as the canonical release tool for tagged
binary releases (Linux, macOS, Windows × amd64/arm64). The existing
`npm install -g @brickhouse-tech/sync-agents` flow MUST continue to work,
fed from the same goreleaser-produced artifacts.

## Pre-existing infrastructure (as of v0.2.4)

The release pipeline is already substantially built out. The pieces below
exist and SHALL NOT be reinvented:

1. **Versioning via `commit-and-tag-version`** — runs as `npx commit-and-tag-version`
   inside the brickhouse-tech org reusable workflow
   `brickhouse-tech/.github/.github/workflows/release.yml@main`, invoked from
   this repo's `.github/workflows/release.yml` on every push to `main`.
   Conventional Commits drive the bump; the workflow auto-detects breaking
   changes (`feat!:`, `fix!:`, `BREAKING CHANGE:`) and forces a major bump.
   Skipped if HEAD commit is already `chore(release): …`.
2. **Tag push → GitHub Release** — `.github/workflows/auto-release.yml`
   triggers on `v*` tags and delegates to org reusable
   `auto-release.yml@main`, which runs `gh release create "$TAG" --title "$TAG"
   --generate-notes --verify-tag`. Empty release (no assets) today.
3. **Tag push → npm OIDC trusted publish with provenance** —
   `.github/workflows/publish.yml` builds all 5 platform packages
   (darwin-{arm64,x64}, linux-{arm64,x64}, win32-x64) and publishes each
   via OIDC + `--provenance`, then publishes the root package. Already on
   Node 24.
4. **optionalDependencies sync** — `scripts/sync-optional-deps.js` runs in
   the root package's `prepack` and rewrites each `optionalDependencies`
   entry to match the root version. The lag visible in source
   (`0.1.18` while parent is `0.2.4`) is cosmetic — overwritten in CI
   immediately before publish.

What's still missing:

- **`go install` from the network is broken** — `go.mod` sits at `go/go.mod`,
  module path mismatches on-disk location. This spec fixes that.
- **GitHub Releases ship no Go binary assets** — `auto-release.yml` creates a
  text-only release. Go users can't grab a prebuilt `sync-agents` from a
  Release page. This spec adds goreleaser as a complementary step that
  appends archives + `checksums.txt` to the existing Release.

## Motivation

Current layout (as of v0.2.4):

```
sync-agents/
├── go/
│   ├── go.mod               ← module: github.com/brickhouse-tech/sync-agents
│   ├── cmd/sync-agents/main.go
│   └── internal/
├── package.json
├── bin/sync-agents.js
├── npm/<triple>/
├── src/{sh,md}/
└── Makefile
```

Problems this causes:

1. **`go install` is broken from the network.** The module path
   `github.com/brickhouse-tech/sync-agents` does not match the on-disk
   location of `go.mod` (which sits under `go/`). Go's module proxy joins the
   repo URL with the directory containing `go.mod`, so the only working install
   command today is the awkward
   `go install github.com/brickhouse-tech/sync-agents/go/cmd/sync-agents@latest`.
2. **Discoverability suffers.** Go-native users expect `go install <module>@latest`
   to "just work" — the de facto first-class install path for a Go CLI.
3. **Releases are bespoke.** Cross-compilation is hand-rolled in `Makefile` and
   `scripts/build-platform.js`. There is no signed checksums file, no
   GitHub Releases artifacts, no SBOM, and no Homebrew tap. Each npm publish
   has to rebuild from source on the publishing host.
4. **Duplication of intent.** The npm platform packages exist solely to ship a
   prebuilt Go binary. A standard Go release pipeline produces those exact
   binaries for free.

## Goals

- `go install github.com/brickhouse-tech/sync-agents@latest` installs the CLI.
- `goreleaser` is the single source of truth for cross-compiled binaries.
- `npm install -g @brickhouse-tech/sync-agents` still works, pulling the same
  goreleaser binaries into the platform packages.
- A tagged release on GitHub produces:
  - GitHub Release with archives, checksums (`SHA256SUMS`), and changelog.
  - A Homebrew tap formula (optional, behind a flag).
  - Synced npm platform package binaries (manually published or via CI).

## Non-Goals

- Rewriting the CLI's commands or behavior. Surface stays identical.
- Migrating off cobra or changing the dependency graph beyond what's required
  to move files.
- Signing releases (cosign / sigstore). Tracked separately.
- Publishing a Homebrew formula in this spec — leave the tap stub but do not
  cut a brew release yet.

## Requirements

### Requirement: Module root layout

The system SHALL place `go.mod`, `go.sum`, and the `main` package at the
repository root, so that the Go module path matches the repository URL.

#### Scenario: Fresh `go install` from network

- **GIVEN** a developer has Go ≥1.22 on PATH and no local clone of the repo
- **WHEN** they run `go install github.com/brickhouse-tech/sync-agents@latest`
- **THEN** the command exits 0
- **AND** `$(go env GOBIN)/sync-agents` (or `$GOPATH/bin/sync-agents`) is
  executable
- **AND** `sync-agents --version` prints a version string matching the latest
  tagged release

#### Scenario: Pinned-version install

- **GIVEN** the repo has a published tag `vX.Y.Z`
- **WHEN** a developer runs `go install github.com/brickhouse-tech/sync-agents@vX.Y.Z`
- **THEN** the installed binary's `--version` output matches `X.Y.Z`

### Requirement: goreleaser pipeline

The system SHALL include a checked-in `.goreleaser.yaml` that builds the CLI
for darwin/linux/windows on amd64/arm64 and uploads archives + checksums as
**assets to the GitHub Release already created by the org-level
`auto-release.yml` reusable workflow**. goreleaser SHALL NOT create a parallel
Release or duplicate the changelog — it appends artifacts.

#### Scenario: Tag triggers asset upload

- **GIVEN** a maintainer's push to `main` triggered the reusable release
  workflow which created and pushed tag `vX.Y.Z`
- **AND** the org `auto-release.yml@main` has created the empty GitHub
  Release for that tag
- **WHEN** this repo's `.github/workflows/goreleaser.yml` job runs on the
  same tag push
- **THEN** `goreleaser release --clean` exits 0
- **AND** archives for {darwin,linux,windows} × {amd64,arm64} are uploaded
  to the existing GitHub Release as assets
- **AND** a `checksums.txt` (SHA256) is attached
- **AND** the Release notes (from `auto-release.yml`'s `--generate-notes`)
  remain unchanged

#### Scenario: Local dry-run snapshot

- **GIVEN** a contributor wants to validate the release config without tagging
- **WHEN** they run `SYNC_AGENTS_VERSION=$(jq -r .version package.json) \
  goreleaser release --snapshot --clean`
- **THEN** the command exits 0
- **AND** `dist/` contains archives for every configured platform

### Requirement: npm install continuity

The system SHALL preserve the existing `npm install -g @brickhouse-tech/sync-agents`
install path, using goreleaser artifacts as the source for the per-platform
binaries shipped in optional dependencies.

#### Scenario: Existing npm consumers upgrade

- **GIVEN** an existing user has `@brickhouse-tech/sync-agents@0.1.20` installed
- **WHEN** they run `npm i -g @brickhouse-tech/sync-agents@latest` after this
  spec ships (vX.Y.Z)
- **THEN** install succeeds without manual intervention
- **AND** the bin entry `sync-agents` is on PATH
- **AND** `sync-agents --version` reports `X.Y.Z`

#### Scenario: Platform package binary parity

- **GIVEN** a release `vX.Y.Z` has been produced by goreleaser
- **WHEN** the npm publish flow runs (manual or CI)
- **THEN** each `npm/<triple>/bin/sync-agents` binary is byte-identical to its
  counterpart inside the goreleaser `dist/` tree
- **AND** the platform package versions in `package.json` `optionalDependencies`
  match the parent package version

### Requirement: Build/test ergonomics for contributors

The system SHALL keep a working `make build && make test` flow from a fresh
clone, with no requirement to `cd go/`.

#### Scenario: Single-command local build

- **GIVEN** a contributor has just cloned the repo
- **WHEN** they run `make build`
- **THEN** `bin/sync-agents` is produced
- **AND** running `bin/sync-agents --version` matches the version derived from
  `package.json`

#### Scenario: bats + go tests run together

- **GIVEN** a contributor wants to run all tests
- **WHEN** they run `npm test`
- **THEN** both the bats suite (`test/sync-agents.bats`) and Go tests under
  `internal/...` execute
- **AND** the command exits 0 on a clean tree

### Requirement: Version single-sourcing via `package.json`

The system SHALL treat `package.json`'s `version` field as the single source
of truth. The existing brickhouse-tech reusable `release.yml` workflow already
runs `npx commit-and-tag-version` on every push to `main` and pushes the
resulting `vX.Y.Z` tag. goreleaser SHALL consume that version when building
the Go binary by reading `package.json` at workflow time (not the raw tag
string), so the `go install`, npm, and Go-release-archive paths cannot drift.

#### Scenario: goreleaser inherits version from package.json

- **GIVEN** a `vX.Y.Z` tag was created by `commit-and-tag-version` on the
  prior push to `main` and has just been pushed
- **WHEN** the goreleaser workflow runs in CI
- **THEN** it reads `package.json` `.version` and exports it as
  `$SYNC_AGENTS_VERSION` before invoking goreleaser
- **AND** `internal/version.Version` is set to that value via
  `-ldflags -X`
- **AND** `sync-agents --version` on the resulting binary reports `X.Y.Z`
- **AND** the value matches the pushed tag, the parent `package.json` and
  every platform `package.json` under `npm/<triple>/`

#### Scenario: Drift fails the release

- **GIVEN** the pushed tag is `vA.B.C` but `package.json` says `X.Y.Z`
  (i.e. someone tagged manually, bypassing `commit-and-tag-version`)
- **WHEN** the goreleaser workflow runs
- **THEN** the workflow fails fast in a version-check step with a clear
  error
- **AND** no Go-binary assets are uploaded to the GitHub Release

#### Scenario: Local snapshot build works without a tag

- **GIVEN** a contributor on `main` with no clean tag yet
- **WHEN** they run `SYNC_AGENTS_VERSION=$(jq -r .version package.json) \
  goreleaser release --snapshot --clean`
- **THEN** `dist/` contains archives for every configured platform
- **AND** the embedded `--version` string is the `package.json` value

## Acceptance Criteria

- **AC-1**: Given Go ≥1.22 and no local clone, when running
  `go install github.com/brickhouse-tech/sync-agents@latest`, then the binary
  installs and `sync-agents --version` reports the latest released version.
- **AC-2**: Given a tag `vX.Y.Z` is pushed, when the goreleaser workflow runs,
  then archives for darwin/linux/windows × amd64/arm64 plus `checksums.txt`
  are attached as assets to the existing GitHub Release created by
  `auto-release.yml`.
- **AC-3**: Given an existing npm install at `0.2.4`, when upgrading to the
  spec's first release, then `npm i -g @brickhouse-tech/sync-agents@latest`
  succeeds and the bin works.
- **AC-4**: Given a fresh clone, when running `make build && make test`, then
  both succeed without `cd go/`.
- **AC-5**: Given any release after this spec ships, when comparing the
  binary inside the npm platform package to the matching goreleaser archive
  asset on the GitHub Release, then they were both produced by the same CI
  run and report identical `--version` output (byte-identity is a
  non-goal — the two pipelines use different stripping flags and
  Node/Go-stage compression).

## Technical Design

### Target layout

```
sync-agents/
├── .goreleaser.yaml        ← NEW
├── .github/workflows/
│   ├── goreleaser.yml      ← NEW (tag-triggered goreleaser; uploads assets to existing Release)
│   ├── release.yml         ← existing (on push to main, delegates to org reusable for commit-and-tag-version)
│   ├── auto-release.yml    ← existing (on tag push, delegates to org reusable for `gh release create`)
│   ├── publish.yml         ← existing, only path edit needed (go-version-file)
│   └── tests.yml           ← existing, only path edit needed (go-version-file)
├── go.mod                  ← MOVED from go/go.mod
├── go.sum                  ← MOVED from go/go.sum
├── main.go                 ← MOVED from go/cmd/sync-agents/main.go
├── internal/
│   ├── agent/              ← MOVED from go/internal/agent/
│   └── version/            ← MOVED from go/internal/version/
├── vendor/                 ← regenerated at new root
├── bin/
│   ├── sync-agents         ← local build output (gitignored)
│   └── sync-agents.js      ← npm launcher (unchanged)
├── npm/<triple>/           ← unchanged shape; binaries sourced from dist/
├── src/{sh,md}/            ← unchanged
├── scripts/
│   ├── build-platform.js   ← updated to call goreleaser snapshot or copy from dist/
│   └── sync-optional-deps.js ← unchanged
├── examples/, test/        ← unchanged
├── package.json            ← unchanged shape; version single-sourced
├── Makefile                ← updated: drop `cd go/`
└── README.md               ← updated install instructions (go install first)
```

### Module path

- `module github.com/brickhouse-tech/sync-agents` stays.
- `main` package moves to repo root → `go install <module>@latest` resolves
  directly. No subpath required.

### Internal import rewrites

```
github.com/brickhouse-tech/sync-agents/internal/agent
github.com/brickhouse-tech/sync-agents/internal/version
```

Identical strings to today — the rewrite is purely on-disk file moves; import
paths are stable.

### Release flow (high level)

```
npm run release             # commit-and-tag-version
  ├── bump package.json
  ├── update CHANGELOG.md
  ├── git commit -m "chore(release): vX.Y.Z"
  └── git tag vX.Y.Z

git push --follow-tags      # maintainer

GitHub Actions: release.yml on tag push
  ├── checkout @ tag
  ├── verify  package.json version == tag (drop the leading "v")
  ├── export SYNC_AGENTS_VERSION=$(jq -r .version package.json)
  ├── goreleaser release --clean
  │     └── ldflags: -X .../internal/version.Version=$SYNC_AGENTS_VERSION
  └── (manual) npm sync + publish
```

### `.goreleaser.yaml` sketch

```yaml
version: 2
project_name: sync-agents

before:
  hooks:
    # Source of truth: package.json. Fail loudly if the tag drifted from it.
    - sh -c 'v=$(jq -r .version package.json); if [ "v$v" != "{{ .Tag }}" ]; then echo "version drift: package.json=$v tag={{ .Tag }}" >&2; exit 1; fi'
    - go mod tidy

builds:
  - id: sync-agents
    main: ./
    binary: sync-agents
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      # {{ .Env.SYNC_AGENTS_VERSION }} is exported by the workflow from
      # package.json; this keeps package.json the single source of truth.
      - -s -w
      - -X github.com/brickhouse-tech/sync-agents/internal/version.Version={{ .Env.SYNC_AGENTS_VERSION }}

archives:
  - id: default
    formats: [tar.gz]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        formats: [zip]

checksum:
  name_template: "checksums.txt"

# auto-release.yml already creates the Release with --generate-notes;
# goreleaser must not duplicate or replace it.
release:
  github:
    owner: brickhouse-tech
    name: sync-agents
  draft: false
  prerelease: auto
  mode: append           # upload assets to the existing Release
  use_existing: true     # do not error if the Release already exists
```

### GitHub Actions: `goreleaser.yml` sketch (NEW)

```yaml
name: goreleaser
on:
  push:
    tags: ["v*.*.*"]

jobs:
  # Run after auto-release.yml has had a chance to create the GH Release.
  # We rely on goreleaser's `mode: append` + `use_existing: true` to be
  # idempotent if this races ahead.
  goreleaser:
    runs-on: ubuntu-latest
    permissions:
      contents: write   # upload assets to releases
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v6
        with: { go-version-file: go.mod }
      - name: Export version from package.json
        run: |
          v=$(jq -r .version package.json)
          tag="${GITHUB_REF_NAME}"
          if [ "v$v" != "$tag" ]; then
            echo "::error::version drift: package.json=$v tag=$tag"
            exit 1
          fi
          echo "SYNC_AGENTS_VERSION=$v" >> "$GITHUB_ENV"
      - uses: goreleaser/goreleaser-action@v6
        with: { version: latest, args: release --clean }
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          SYNC_AGENTS_VERSION: ${{ env.SYNC_AGENTS_VERSION }}
```

### `commit-and-tag-version` — already in place

The org reusable workflow at
`brickhouse-tech/.github/.github/workflows/release.yml@main` already runs
`npx commit-and-tag-version` on every push to `main`. This spec does NOT
add a `commit-and-tag-version` devDependency or `npm run release` script
to this repo — they would only create local drift from how CI actually
versions the project.

`scripts/sync-optional-deps.js` already runs in `prepack` and rewrites
`optionalDependencies` to match root version, so the cosmetic drift visible
in source (`0.1.18` vs root `0.2.4`) is overwritten in CI before publish
and does not affect published artifacts.

### Makefile updates

Remove `cd go &&` prefixes:

```make
install:
	go mod tidy -e && go mod vendor

build:
	go build $(LDFLAGS) -o bin/sync-agents ./

build-platform: install
	# unchanged logic, drop the cd go &&
```

### Migration steps (one PR)

1. `git mv go/go.mod go.mod && git mv go/go.sum go.sum`
2. `git mv go/cmd/sync-agents/main.go main.go`
3. `git mv go/internal internal`
4. `rm -rf go/vendor && go mod vendor` (at the new root)
5. Update `Makefile` to drop `cd go/` prefixes.
6. Update `scripts/build-platform.js` to build from repo root.
7. Update `.github/workflows/publish.yml` and `tests.yml`:
   `go-version-file: go/go.mod` → `go-version-file: go.mod`.
8. Add `.goreleaser.yaml` (with `mode: append` so it never duplicates the
   Release auto-release.yml creates).
9. Add `.github/workflows/goreleaser.yml`.
10. Add `dist/` to `.gitignore`.
11. Update `README.md`: lead with `go install`, fall back to npm.
12. Run `go build ./...`, `go vet ./...`, `make build`, `npm test`,
    `SYNC_AGENTS_VERSION=$(jq -r .version package.json) goreleaser release --snapshot --clean`
    locally before pushing.
13. Merge PR. The reusable workflow will cut a `feat:` minor bump tag, which
    triggers all three downstream workflows (`auto-release.yml`,
    `publish.yml`, new `goreleaser.yml`). Verify on the resulting Release
    page.

## Test Plan

- [ ] `go build ./...` succeeds from repo root.
- [ ] `go vet ./...` succeeds from repo root.
- [ ] `go test ./internal/...` succeeds (add at least one smoke test for
      `version.Version` if none exists).
- [ ] `make build` produces `bin/sync-agents`.
- [ ] `make build-all` produces all five platform binaries.
- [ ] `npm test` runs both bats and Go suites.
- [ ] `goreleaser release --snapshot --clean` exits 0 and produces archives
      for all targets (with `SYNC_AGENTS_VERSION` exported from `package.json`).
- [ ] Drift guard: pushing a tag that doesn't match `package.json` fails the
      `goreleaser.yml` workflow in the version-export step before goreleaser
      runs.
- [ ] `auto-release.yml` still creates the Release with notes; `goreleaser.yml`
      appends assets to it (verified by manually tagging in a fork or by
      running `goreleaser release --snapshot` and inspecting `dist/`).
- [ ] Cut `vX.Y.Z-rc.1` tag in a fork; verify `go install github.com/<fork>/sync-agents@vX.Y.Z-rc.1`
      yields a working binary on at least darwin-arm64 and linux-amd64.
- [ ] `npm pack` on the parent package + each platform package produces tarballs
      identical in shape to pre-migration baseline (manual diff).
- [ ] `sync-agents --version`, `sync-agents init`, and `sync-agents sync` smoke
      tests pass after install via both paths.

## Rollout

- One PR cuts the layout move + adds goreleaser + new workflow.
- Merge to `main`. The reusable `release.yml` will auto-bump the version
  via `commit-and-tag-version` and push the tag.
- The new `goreleaser.yml` workflow fires on that tag push concurrently with
  `auto-release.yml` and `publish.yml`. Verify Go binary archives + checksums
  attach to the GitHub Release.
- README announces `go install` as the recommended install path.
- Keep npm install supported indefinitely; no deprecation in this spec.

## Open Questions

- **Q1:** Do we want a Homebrew tap (`brickhouse-tech/tap/sync-agents`) in this
  spec, or defer? Recommend defer — adds maintainer burden.
- **Q2:** Should `version.Version` default to `dev` when not built via
  goreleaser (currently it falls back to `package.json`'s version via
  `Makefile`)? Recommend **keep the Makefile fallback** — it already reads
  `package.json`, which matches the new source-of-truth rule, so a local
  `make build` reports the correct version without needing a tag.
- **Q3:** Do we want SLSA provenance / cosign signing now or defer? Defer.

## References

- Go module layout: <https://go.dev/ref/mod#vcs>
- goreleaser docs: <https://goreleaser.com/customization/>
- npm bin-binary pattern (precedent: esbuild, biome, turbo).
