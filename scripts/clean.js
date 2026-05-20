#!/usr/bin/env node
const fs = require("node:fs");
const path = require("node:path");
const { PLATFORMS } = require("./lib/platforms");

fs.rmSync(path.join("bin", "sync-agents"), { force: true });
for (const triple of PLATFORMS) {
  fs.rmSync(path.join("npm", triple, "bin"), { recursive: true, force: true });
}
