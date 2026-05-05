#!/usr/bin/env node
// Launcher: prefer the Go binary shipped via the matching platform package
// (@brickhouse-tech/sync-agents-<os>-<arch>); fall back to the bash script
// at src/sh/sync-agents.sh so the package never bricks on an unsupported
// triple.

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
    // platform package not installed (npm skipped it via os/cpu mismatch
    // or it failed as an optionalDependency); fall through to bash.
  }
  return null;
}

function fallbackShellScript() {
  return path.join(__dirname, "..", "src", "sh", "sync-agents.sh");
}

const target = resolveGoBinary() ?? fallbackShellScript();
const isShell = target.endsWith(".sh");

const result = spawnSync(isShell ? "bash" : target, isShell ? [target, ...process.argv.slice(2)] : process.argv.slice(2), {
  stdio: "inherit",
});

if (result.error) {
  console.error(`sync-agents: failed to exec ${target}: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status ?? 0);
