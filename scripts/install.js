#!/usr/bin/env node
// Pre-build hook: ensure Go deps are tidy and vendored.
const { execFileSync } = require("node:child_process");

execFileSync("go", ["mod", "tidy", "-e"], { stdio: "inherit" });
execFileSync("go", ["mod", "vendor"], { stdio: "inherit" });
