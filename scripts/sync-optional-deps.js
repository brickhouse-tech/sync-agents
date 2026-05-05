#!/usr/bin/env node
// Sync the root package's optionalDependencies versions to match the root
// version field. Runs as part of the root package's `prepack` so the
// published tarball pins each platform package at the same version that's
// being released.

const fs = require("node:fs");

const pkg = JSON.parse(fs.readFileSync("package.json", "utf8"));
const v = pkg.version;

let changed = false;
for (const name of Object.keys(pkg.optionalDependencies || {})) {
  if (pkg.optionalDependencies[name] !== v) {
    pkg.optionalDependencies[name] = v;
    changed = true;
  }
}

if (changed) {
  fs.writeFileSync("package.json", JSON.stringify(pkg, null, 2) + "\n");
}
