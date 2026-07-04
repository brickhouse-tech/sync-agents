# Changelog

All notable changes to this project will be documented in this file. See [commit-and-tag-version](https://github.com/absolute-version/commit-and-tag-version) for commit guidelines.

## [1.2.1](https://github.com/brickhouse-tech/sync-agents/compare/v1.2.0...v1.2.1) (2026-07-04)

## [1.2.0](https://github.com/brickhouse-tech/sync-agents/compare/v1.1.0...v1.2.0) (2026-07-03)


### Features

* **quarantine:** static scan + review gate for remote installs (SPEC-005 Part B, fixes [#52](https://github.com/brickhouse-tech/sync-agents/issues/52)) ([44994b3](https://github.com/brickhouse-tech/sync-agents/commit/44994b38042c550adf631c1a2a177f6662a0818f)), closes [#62](https://github.com/brickhouse-tech/sync-agents/issues/62)

## [1.1.0](https://github.com/brickhouse-tech/sync-agents/compare/v1.0.1...v1.1.0) (2026-07-03)


### Features

* **source:** declarative source manifest, pull/update, lockfile + sha256 integrity (fixes [#51](https://github.com/brickhouse-tech/sync-agents/issues/51)) ([05c422a](https://github.com/brickhouse-tech/sync-agents/commit/05c422a23a2e9cc764e4504c6d9ca0cf51e920fa))


### Bug Fixes

* **deps:** track go.sum — required for gopkg.in/yaml.v3 integrity ([fee62fa](https://github.com/brickhouse-tech/sync-agents/commit/fee62fa7dfd967aea8b732b43a65047c9f49d67f))
* **hooks:** preserve script-style hooks in the index; warn when nothing merges ([5ee6960](https://github.com/brickhouse-tech/sync-agents/commit/5ee6960285f4cbecf4aa60d8e521b7a02969722e))

## [1.0.1](https://github.com/brickhouse-tech/sync-agents/compare/v1.0.0...v1.0.1) (2026-07-03)

## [1.0.0](https://github.com/brickhouse-tech/sync-agents/compare/v0.3.6...v1.0.0) (2026-07-03)


### ⚠ BREAKING CHANGES

* **cli:** `sync-agents hook` is deprecated in favor of
`sync-agents git-hook`. The alias remains functional until v1.0.

Co-authored-by: nmccready <creadbot@github.com>
Co-authored-by: Claude Fable 5 <noreply@anthropic.com>

### Features

* **adrs:** ADR bucket with status subdirectories + adr lifecycle command (SPEC-004 Part F, refs [#50](https://github.com/brickhouse-tech/sync-agents/issues/50)) ([#60](https://github.com/brickhouse-tech/sync-agents/issues/60)) ([26570c9](https://github.com/brickhouse-tech/sync-agents/commit/26570c9c52794a714d18ec1df63bd61def42b85d))
* **cli:** rename `hook` to `git-hook`, keep deprecated alias (SPEC-004 Part C, refs [#50](https://github.com/brickhouse-tech/sync-agents/issues/50)) ([#59](https://github.com/brickhouse-tech/sync-agents/issues/59)) ([2c1167d](https://github.com/brickhouse-tech/sync-agents/commit/2c1167dea64ce07566fdb1d68b489e5d8ec142cd))

## [0.3.6](https://github.com/brickhouse-tech/sync-agents/compare/v0.3.5...v0.3.6) (2026-07-03)


### Features

* **index:** plans+specs buckets, description-aware index, skill header backfill (SPEC-004 Part D, refs [#50](https://github.com/brickhouse-tech/sync-agents/issues/50)) ([#58](https://github.com/brickhouse-tech/sync-agents/issues/58)) ([80f7dbc](https://github.com/brickhouse-tech/sync-agents/commit/80f7dbcc8176988a2674ad787153f5a4fab40976))

## [0.3.5](https://github.com/brickhouse-tech/sync-agents/compare/v0.3.4...v0.3.5) (2026-07-03)


### Features

* **index:** plans+specs buckets, description-aware index, skill header backfill (SPEC-004 Part D, refs [#50](https://github.com/brickhouse-tech/sync-agents/issues/50)) ([#57](https://github.com/brickhouse-tech/sync-agents/issues/57)) ([1f7cae8](https://github.com/brickhouse-tech/sync-agents/commit/1f7cae8cdaedb5b892c1b84ffd38df46e741cca9))

## [0.3.4](https://github.com/brickhouse-tech/sync-agents/compare/v0.3.3...v0.3.4) (2026-07-02)


### Features

* **agents:** subagents bucket — .agents/agents/ routed to .claude/agents/ (SPEC-004 Part B, refs [#50](https://github.com/brickhouse-tech/sync-agents/issues/50)) ([#56](https://github.com/brickhouse-tech/sync-agents/issues/56)) ([839b2f2](https://github.com/brickhouse-tech/sync-agents/commit/839b2f2e5ab8490985599b375a8aa443600fa2bf))

## [0.3.3](https://github.com/brickhouse-tech/sync-agents/compare/v0.3.2...v0.3.3) (2026-07-02)


### Features

* **lint:** skill frontmatter compliance — lint + --fix per Claude authoring rules (fixes [#54](https://github.com/brickhouse-tech/sync-agents/issues/54)) ([#55](https://github.com/brickhouse-tech/sync-agents/issues/55)) ([f27db87](https://github.com/brickhouse-tech/sync-agents/commit/f27db87bab5ae81b9defddb1e670f1feed797447))

## [0.3.2](https://github.com/brickhouse-tech/sync-agents/compare/v0.3.1...v0.3.2) (2026-07-02)


### Features

* **claude:** write managed @-import block so Claude loads passive rules (fixes [#46](https://github.com/brickhouse-tech/sync-agents/issues/46)) ([#47](https://github.com/brickhouse-tech/sync-agents/issues/47)) ([a82e66c](https://github.com/brickhouse-tech/sync-agents/commit/a82e66c0107d388c09375977edbece624e90a94e))

## [0.3.1](https://github.com/brickhouse-tech/sync-agents/compare/v0.3.0...v0.3.1) (2026-06-29)


### Bug Fixes

* remove duplicate 0.2.8/0.2.7 changelog sections, fix flaky idempotent test ([#45](https://github.com/brickhouse-tech/sync-agents/issues/45)) ([65478a8](https://github.com/brickhouse-tech/sync-agents/commit/65478a88454a50802c9e629c6bf24c1d5684cde2))

## [0.3.0](https://github.com/brickhouse-tech/sync-agents/compare/v0.2.6...v0.3.0) (2026-06-16)


### Features

* **agent:** CmdGlobalStatus + CmdGlobalClean (SPEC-002 PR D) ([c7c07d7](https://github.com/brickhouse-tech/sync-agents/commit/c7c07d7b9897ca97e4f3e695c2e519eb44cc335c))
* **agent:** CmdGlobalSync with semantic-aware routing (SPEC-002 PR C.2) ([93981de](https://github.com/brickhouse-tech/sync-agents/commit/93981de39015333ae3ae913fe8c919cbbbb01ae6))
* **agent:** CmdPromote and CmdGlobalInit (SPEC-002 PR B) ([f63d143](https://github.com/brickhouse-tech/sync-agents/commit/f63d1432da17bac8c92b870a984f8f0533519d4e))
* **agent:** Scope + Tool types and GlobalRoot resolver (SPEC-002 PR A) ([a84c13f](https://github.com/brickhouse-tech/sync-agents/commit/a84c13f40f290f49bda35db30af7f28d3954cc27))
* **agent:** semantic resolver + frontmatter parser (SPEC-002 PR C.1) ([cf55ae9](https://github.com/brickhouse-tech/sync-agents/commit/cf55ae915f0be30f3e12e65d05d84f3d9fbbe4b8))
* **install:** add go install, Homebrew, and GitHub Releases channels ([#20](https://github.com/brickhouse-tech/sync-agents/issues/20)) ([ff1a7cc](https://github.com/brickhouse-tech/sync-agents/commit/ff1a7ccfd47b372d447f5ad6ccffc0d8ea084074))
* **remove-bash:** drop bash fallback, fail loudly on unsupported triples ([#19](https://github.com/brickhouse-tech/sync-agents/issues/19)) ([089ae83](https://github.com/brickhouse-tech/sync-agents/commit/089ae83942f280bd2247339dae558285ff3434e7))


### Bug Fixes

* basic int testing fixes in help menu ([54e2158](https://github.com/brickhouse-tech/sync-agents/commit/54e215805580b126c263054b37c14b147f0fb2fa))
* **help:** loop-render top-level command and options list ([#38](https://github.com/brickhouse-tech/sync-agents/issues/38)) ([df89794](https://github.com/brickhouse-tech/sync-agents/commit/df897946811bf28cfb751c1f06fd86ce493d26e8))
* migrate goreleaser config for v2.16 compatibility ([1a8549d](https://github.com/brickhouse-tech/sync-agents/commit/1a8549d280d18cf4a386e2a711e1de293c5d7938))
* rule tempate to remove dead empty line ([b4a77b0](https://github.com/brickhouse-tech/sync-agents/commit/b4a77b0a365ab607dc1e30dd5841491d81dd9888))


## [0.2.6](https://github.com/brickhouse-tech/sync-agents/compare/v0.2.5...v0.2.6) (2026-06-12)


### Bug Fixes

* **version:** strip leading v from BuildInfo-resolved version ([db51c8d](https://github.com/brickhouse-tech/sync-agents/commit/db51c8df47a809f7571740581a2871adcb81c906)), closes [#37](https://github.com/brickhouse-tech/sync-agents/issues/37)

## [0.2.5](https://github.com/brickhouse-tech/sync-agents/compare/v0.2.4...v0.2.5) (2026-05-14)


### Features

* hoist Go module to repo root for first-class `go install` + goreleaser ([ba335bc](https://github.com/brickhouse-tech/sync-agents/commit/ba335bc02dcb9726bdd6dc0ab199d105ea750b88))


### Bug Fixes

* **release:** sync optionalDependencies into release commit via postbump ([2eae388](https://github.com/brickhouse-tech/sync-agents/commit/2eae3889e9d85ae83614f6d5439a6bd052dfeef9))

## [0.2.4](https://github.com/brickhouse-tech/sync-agents/compare/v0.2.3...v0.2.4) (2026-05-13)


### Bug Fixes

* **scripts:** bash 3.2 compat for bootstrap-platform-publish ([b433883](https://github.com/brickhouse-tech/sync-agents/commit/b433883a30bc6ed8a0300017c0033d9f0332b49a))

## [0.2.3](https://github.com/brickhouse-tech/sync-agents/compare/v0.2.2...v0.2.3) (2026-05-13)

## [0.2.2](https://github.com/brickhouse-tech/sync-agents/compare/v0.2.1...v0.2.2) (2026-05-13)


### Bug Fixes

* **ci:** disable SC2120/SC2119 style warnings ([a3176c2](https://github.com/brickhouse-tech/sync-agents/commit/a3176c2b9f14015c5459c8683d0f4d4e7e768680))
* **ci:** drop flaky shellcheck npm wrapper, use system shellcheck ([0212eb4](https://github.com/brickhouse-tech/sync-agents/commit/0212eb4f434c049fc7001db1157d13d0ce9831b2)), closes [#23](https://github.com/brickhouse-tech/sync-agents/issues/23)

## [0.2.1](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.21...v0.2.1) (2026-05-13)

## [0.1.21](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.20...v0.1.21) (2026-05-05)

## [0.1.20](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.19...v0.1.20) (2026-05-05)


### Bug Fixes

* **ci:** use corepack to pin npm 11.13.0 on Node 22 for OIDC publish ([3e18d4b](https://github.com/brickhouse-tech/sync-agents/commit/3e18d4b038ea5a00be0dbafcdd4faf98e8f474f5))

## [0.1.19](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.18...v0.1.19) (2026-05-05)


### Features

* **npm:** ship Go binaries via per-platform optional packages ([bcc0ace](https://github.com/brickhouse-tech/sync-agents/commit/bcc0ace5594e7a0c752be702a062f26bb8ae810b))


### Bug Fixes

* **npm:** drive platform builds via npm prepack lifecycle + node 20/22/24 matrix ([97b52f0](https://github.com/brickhouse-tech/sync-agents/commit/97b52f09d161c94278580577cde183fde5a89989)), closes [#18](https://github.com/brickhouse-tech/sync-agents/issues/18)

## [0.1.18](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.17...v0.1.18) (2026-05-04)

## [0.1.17](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.16...v0.1.17) (2026-04-30)


### Features

* rewrite sync-agents CLI in Go ([3b2f2f9](https://github.com/brickhouse-tech/sync-agents/commit/3b2f2f9659bea6ff4234251c74bc49fb5800b0f9))


### Bug Fixes

* **go:** inject version from package.json + untrack built binary ([66e6b83](https://github.com/brickhouse-tech/sync-agents/commit/66e6b83a98699232eee815460a10b2279e710002))

## [0.1.16](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.15...v0.1.16) (2026-04-26)


### Features

* per-file state snapshots replacing monolithic STATE.md ([e62f3f0](https://github.com/brickhouse-tech/sync-agents/commit/e62f3f06ce02b18dd2fce39269899e2cd2c1f57e))

## [0.1.15](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.14...v0.1.15) (2026-04-26)

## [0.1.14](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.13...v0.1.14) (2026-04-21)


### Features

* **rules:** add git workflow rule ([2ec3935](https://github.com/brickhouse-tech/sync-agents/commit/2ec39359f1e66b3771cc4d75f5e037992018fac8))


### Bug Fixes

* **sh:** resolve sync-agents version command for global npm installs ([1e4ab65](https://github.com/brickhouse-tech/sync-agents/commit/1e4ab6578e52c4bdef7dacfa8084ca8add3cdb58))

## [0.1.13](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.12...v0.1.13) (2026-04-21)


### Features

* **rules:** add documentation sync rule for keeping docs and examples in sync ([cd1b2c5](https://github.com/brickhouse-tech/sync-agents/commit/cd1b2c5386ec9308a41a04fb30c5f01a427a482c))

## [0.1.12](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.11...v0.1.12) (2026-04-21)


### Features

* add version subcommand as alias for --version ([e1e43f2](https://github.com/brickhouse-tech/sync-agents/commit/e1e43f22d8f220b293559f11f3c9462817e807d2))

## [0.1.11](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.10...v0.1.11) (2026-04-21)


### Bug Fixes

* convert flat skill files to directory layout in fix command ([53c9130](https://github.com/brickhouse-tech/sync-agents/commit/53c913032b92a9bb81bf5577561fde1b747364ba))
* make fix command merge by default, detect same-inode dirs ([af07927](https://github.com/brickhouse-tech/sync-agents/commit/af07927ea70385813ff0ceb302c5cb6d95786e52))
* make fix command repair broken/missing symlinks ([1d8d603](https://github.com/brickhouse-tech/sync-agents/commit/1d8d603b01245eb99e55f3adcfad2072a61887b5))

## [0.1.10](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.9...v0.1.10) (2026-04-21)


### Features

* add fix command to migrate legacy dirs into .agents/ ([38bff08](https://github.com/brickhouse-tech/sync-agents/commit/38bff08fedc717321eb6ff2bf92579a8de31ac2f))
* skills use directory layout (skills/name/SKILL.md) ([95960d9](https://github.com/brickhouse-tech/sync-agents/commit/95960d922ae0e0188f13a1210777cfce662f96ec))


### Bug Fixes

* replace ls with find to satisfy shellcheck SC2012 ([095f2b8](https://github.com/brickhouse-tech/sync-agents/commit/095f2b8be33138ebdf94850ccf3f0f9dd8bdb407))
* use arithmetic assignment instead of ((fixed++)) ([7fdd3e0](https://github.com/brickhouse-tech/sync-agents/commit/7fdd3e04e17970bdf5ad5bd47b38b55cc3923c19))

## [0.1.9](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.8...v0.1.9) (2026-04-06)


### Features

* add default .gitignore entries during init ([#7](https://github.com/brickhouse-tech/sync-agents/issues/7)) ([bda26e1](https://github.com/brickhouse-tech/sync-agents/commit/bda26e176728f66b8ecf651c300000a99d2ac280))
* auto-update .gitignore with synced symlink entries ([eb61d2a](https://github.com/brickhouse-tech/sync-agents/commit/eb61d2a3b7c8f6f9b3103a74bb5f1938607a367c))


### Bug Fixes

* **sync-agents.sh:** combine file appends to fix SC2129 style warning ([e55eb02](https://github.com/brickhouse-tech/sync-agents/commit/e55eb02d52dee77b4cb7adc27008fe6659440d9c))

## [0.1.8](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.7...v0.1.8) (2026-04-04)

## [0.1.7](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.6...v0.1.7) (2026-04-01)

## [0.1.6](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.5...v0.1.6) (2026-03-27)


### Features

* add inheritance convention support ([7017e39](https://github.com/brickhouse-tech/sync-agents/commit/7017e39127e51d7168c05fc70451df862d71df6c))


### Bug Fixes

* resolve shellcheck lint warnings in inherit command ([673131e](https://github.com/brickhouse-tech/sync-agents/commit/673131ec60b522409cd685db9c037ec3ddb14699))

## [0.1.5](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.4...v0.1.5) (2026-03-26)

## [0.1.4](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.3...v0.1.4) (2026-03-14)


### Features

* add cursor/codex/copilot targets, watch, import, hook, templates, LICENSE ([5cf1831](https://github.com/brickhouse-tech/sync-agents/commit/5cf1831eeaec7771ff4d850659bf95ec8fac6f93)), closes [#4](https://github.com/brickhouse-tech/sync-agents/issues/4) [#5](https://github.com/brickhouse-tech/sync-agents/issues/5) [#6](https://github.com/brickhouse-tech/sync-agents/issues/6) [#7](https://github.com/brickhouse-tech/sync-agents/issues/7) [#8](https://github.com/brickhouse-tech/sync-agents/issues/8) [#9](https://github.com/brickhouse-tech/sync-agents/issues/9) [#10](https://github.com/brickhouse-tech/sync-agents/issues/10) [#12](https://github.com/brickhouse-tech/sync-agents/issues/12)

## [0.1.3](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.2...v0.1.3) (2026-03-14)


### Bug Fixes

* remove python3 dependency, fix version on global install ([bd3f3a4](https://github.com/brickhouse-tech/sync-agents/commit/bd3f3a4cdeffd819ee69c4702a6959f059b1b3f1))

## [0.1.2](https://github.com/brickhouse-tech/sync-agents/compare/v0.1.1...v0.1.2) (2026-03-14)


### Bug Fixes

* dynamic version test + upgrade setup-node to v4 ([868c705](https://github.com/brickhouse-tech/sync-agents/commit/868c70511ab0e055f84c2b0798ee5705778ec68e))

## 0.1.1 (2026-03-14)


### Features

* initialize repo ([8fd6c26](https://github.com/brickhouse-tech/sync-agents/commit/8fd6c26328dbdaae0ef9e84636b9b31b82241100))
* ready to publish and test ([1ecf855](https://github.com/brickhouse-tech/sync-agents/commit/1ecf855200b4edbbc8b615424e3b61670c2d69bf))
* save off possibly working script ([385e3de](https://github.com/brickhouse-tech/sync-agents/commit/385e3ded759d0d3f56a38903b533787b123da322))
* save off possibly working script ([ff4e84c](https://github.com/brickhouse-tech/sync-agents/commit/ff4e84cb2d0fa9803b680e6f3a5c665e92ded833))
* tested sync-agent locally g2g ([0792813](https://github.com/brickhouse-tech/sync-agents/commit/07928138f936472f474fd9f7bc43b9e9d3eebdd7))
