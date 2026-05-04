# Platform packages

Each subdirectory here is its own npm package shipping a single
prebuilt Go binary for one (`os`, `cpu`) triple. The root package
(`@brickhouse-tech/sync-agents`) lists them under
`optionalDependencies` so npm only installs the one that matches the
host's `process.platform` / `process.arch`.

The `bin/sync-agents.js` launcher in the root package resolves the
matching platform package via `require.resolve` and execs its binary.
If no platform package matches (unsupported triple, install failure,
etc.), it falls back to `src/sh/sync-agents.sh` so the package never
fully bricks.

| Directory       | Go GOOS  | Go GOARCH | Binary             |
| --------------- | -------- | --------- | ------------------ |
| `darwin-arm64`  | darwin   | arm64     | `sync-agents`      |
| `darwin-x64`    | darwin   | amd64     | `sync-agents`      |
| `linux-arm64`   | linux    | arm64     | `sync-agents`      |
| `linux-x64`     | linux    | amd64     | `sync-agents`      |
| `win32-x64`     | windows  | amd64     | `sync-agents.exe`  |

The binaries are populated by CI (`make build-all`) before publish;
they are not checked in.
