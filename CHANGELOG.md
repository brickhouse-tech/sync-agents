# Changelog

All notable changes to this project will be documented in this file. See [commit-and-tag-version](https://github.com/absolute-version/commit-and-tag-version) for commit guidelines.

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
