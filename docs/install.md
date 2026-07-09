# Installation

Every supported install channel for the `sync-agents` binary, with the trade-offs of each.

## npm (recommended for Node.js projects)

Ships native Go binaries via per-platform optional packages — no build
step, no Go toolchain required.

```bash
npm install -g @brickhouse-tech/sync-agents
```

Or as a project devDependency:

```bash
npm install --save-dev @brickhouse-tech/sync-agents
```

## go install (no Node.js required)

```bash
go install github.com/brickhouse-tech/sync-agents@latest
```

Requires Go 1.21+. The binary is placed in `$GOPATH/bin` (or
`$HOME/go/bin`). Version is read from the module proxy at install time
via `debug.ReadBuildInfo`.

## Homebrew

```bash
brew install brickhouse-tech/tap/sync-agents
```

The tap is updated automatically on every release via GoReleaser.

## GitHub Releases (pre-built binaries)

Download the archive for your platform from the
[Releases page](https://github.com/brickhouse-tech/sync-agents/releases),
extract, and place the binary on your `PATH`:

```bash
# Example: macOS arm64
curl -fsSL https://github.com/brickhouse-tech/sync-agents/releases/latest/download/sync-agents_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/').tar.gz | tar -xz
sudo mv sync-agents /usr/local/bin/
```

SHA-256 checksums are published alongside each release as
`checksums.txt`.

## See also

- [Command reference](./commands/README.md)
- [Topology & configuration](./topology.md)
