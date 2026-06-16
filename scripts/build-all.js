#!/usr/bin/env node
// Build every platform binary in one shot, for local verification.
const { execFileSync } = require("node:child_process");
const path = require("node:path");
const { PLATFORMS } = require("./lib/platforms");

const buildPlatform = path.join(__dirname, "build-platform.js");
for (const triple of PLATFORMS) {
  execFileSync(process.execPath, [buildPlatform, "--triple", triple], {
    stdio: "inherit",
  });
}
