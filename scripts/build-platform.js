#!/usr/bin/env node
// Cross-compile one platform binary into npm/<triple>/bin/.
//
// Two usage modes:
//   1. From a platform package dir (npm/<triple>/), via that package's `prepack`.
//      The triple is taken from the cwd basename, and the package.json version
//      is synced to root before building.
//   2. From the repo root, with `--triple <triple>` (or env TRIPLE), e.g.
//      `npm run build:platform -- --triple darwin-arm64`. Used for local
//      verification and by `build:all`.

const path = require("node:path");
const fs = require("node:fs");
const { execFileSync } = require("node:child_process");
const { PLATFORMS, tripleToGo } = require("./lib/platforms");
const { ldflags } = require("./lib/ldflags");

function parseTripleArg() {
  const args = process.argv.slice(2);
  const idx = args.indexOf("--triple");
  if (idx >= 0 && args[idx + 1]) return args[idx + 1];
  if (process.env.TRIPLE) return process.env.TRIPLE;
  return null;
}

function resolveContext() {
  const explicit = parseTripleArg();
  if (explicit) {
    return { triple: explicit, rootDir: process.cwd(), outDir: path.join(process.cwd(), "npm", explicit, "bin") };
  }
  // Platform-package prepack mode: cwd is npm/<triple>/.
  const triple = path.basename(process.cwd());
  if (!PLATFORMS.includes(triple)) {
    throw new Error(
      `unable to infer triple from cwd (${process.cwd()}); pass --triple <one of: ${PLATFORMS.join(", ")}>`,
    );
  }
  const rootDir = path.resolve("..", "..");
  // Sync the platform package version to root.
  const here = JSON.parse(fs.readFileSync("package.json", "utf8"));
  const rootPkg = JSON.parse(fs.readFileSync(path.join(rootDir, "package.json"), "utf8"));
  if (here.version !== rootPkg.version) {
    here.version = rootPkg.version;
    fs.writeFileSync("package.json", JSON.stringify(here, null, 2) + "\n");
  }
  return { triple, rootDir, outDir: path.join(process.cwd(), "bin") };
}

const { triple, rootDir, outDir } = resolveContext();
if (!PLATFORMS.includes(triple)) {
  console.error(`unknown triple: ${triple} (expected one of: ${PLATFORMS.join(", ")})`);
  process.exit(1);
}

const { goos, goarch, ext } = tripleToGo(triple);
const outFile = path.join(outDir, `sync-agents${ext}`);
fs.mkdirSync(outDir, { recursive: true });

console.log(`==> ${triple} (${goos}/${goarch}) -> ${path.relative(rootDir, outFile)}`);

execFileSync(
  "go",
  ["build", "-ldflags", ldflags(rootDir), "-o", outFile, "./"],
  {
    cwd: rootDir,
    stdio: "inherit",
    env: {
      ...process.env,
      CGO_ENABLED: "0",
      GOOS: goos,
      GOARCH: goarch,
    },
  },
);
