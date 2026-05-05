#!/usr/bin/env node
// Run from a platform package directory (npm/<triple>/) — typically via that
// package's `prepack` script. Syncs the package's version with the root
// package.json and cross-compiles the matching Go binary into ./bin/.

const path = require("node:path");
const fs = require("node:fs");
const { execFileSync } = require("node:child_process");

const triple = path.basename(process.cwd());
const rootPath = path.resolve("..", "..");
const rootPkg = JSON.parse(fs.readFileSync(path.join(rootPath, "package.json"), "utf8"));

const here = JSON.parse(fs.readFileSync("package.json", "utf8"));
if (here.version !== rootPkg.version) {
  here.version = rootPkg.version;
  fs.writeFileSync("package.json", JSON.stringify(here, null, 2) + "\n");
}

execFileSync("make", ["-C", rootPath, "build-platform", `PLATFORM=${triple}`], {
  stdio: "inherit",
});
