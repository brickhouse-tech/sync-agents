const path = require("node:path");
const fs = require("node:fs");

function ldflags(rootDir = process.cwd()) {
  const pkg = JSON.parse(
    fs.readFileSync(path.join(rootDir, "package.json"), "utf8"),
  );
  return `-X github.com/brickhouse-tech/sync-agents/internal/version.Version=${pkg.version}`;
}

module.exports = { ldflags };
