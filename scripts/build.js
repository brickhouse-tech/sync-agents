#!/usr/bin/env node
// Host build: produces bin/sync-agents for the current OS/arch.
const { execFileSync } = require("node:child_process");
const { ldflags } = require("./lib/ldflags");

execFileSync(
  "go",
  ["build", "-ldflags", ldflags(), "-o", "bin/sync-agents", "./"],
  { stdio: "inherit" },
);
