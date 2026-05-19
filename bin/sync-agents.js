#!/usr/bin/env node
// Launcher: resolve the Go binary shipped via the matching platform package
// (@brickhouse-tech/sync-agents-<os>-<arch>). Exits with a clear error on
// unsupported triples rather than silently falling back to a bash script.

const { spawnSync } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");

const platformPkg = `@brickhouse-tech/sync-agents-${process.platform}-${process.arch}`;
const exe = process.platform === "win32" ? "sync-agents.exe" : "sync-agents";

function resolveGoBinary() {
  try {
    const pkgJson = require.resolve(`${platformPkg}/package.json`);
    const candidate = path.join(path.dirname(pkgJson), "bin", exe);
    if (fs.existsSync(candidate)) return candidate;
  } catch {
    // platform package not installed or not available for this triple.
  }
  return null;
}

const target = resolveGoBinary();

if (!target) {
  console.error(
    `sync-agents: no pre-built binary for ${process.platform}/${process.arch}.\n` +
    `Install via one of the supported channels:\n` +
    `  go install github.com/brickhouse-tech/sync-agents@latest\n` +
    `  brew install brickhouse-tech/tap/sync-agents\n` +
    `  https://github.com/brickhouse-tech/sync-agents/releases`
  );
  process.exit(1);
}

const result = spawnSync(target, process.argv.slice(2), { stdio: "inherit" });

if (result.error) {
  console.error(`sync-agents: failed to exec ${target}: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status ?? 0);
