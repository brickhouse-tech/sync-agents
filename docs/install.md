# Installation

Every supported install channel for the `sync-agents` binary, with the trade-offs of each.

## go install

```bash
go install github.com/brickhouse-tech/sync-agents@latest
```

Requires Go 1.21+. The binary is placed in `$GOPATH/bin` (or
`$HOME/go/bin`). Version is read from the module proxy at install time
via `debug.ReadBuildInfo`.

## Homebrew

Prebuilt binary; no Go toolchain needed.

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

## npm (discontinued)

`@brickhouse-tech/sync-agents` on npm stopped at 2.0.0 and is no longer
published. Uninstall it (`npm uninstall -g @brickhouse-tech/sync-agents`)
and use one of the channels above.

## See also

- [Command reference](./commands/README.md)
- [Topology & configuration](./topology.md)
