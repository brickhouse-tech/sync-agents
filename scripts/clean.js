#!/usr/bin/env node
const fs = require("node:fs");
const path = require("node:path");

fs.rmSync(path.join("bin", "sync-agents"), { force: true });
