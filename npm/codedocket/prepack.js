#!/usr/bin/env node
"use strict";

// npm runs this script before `npm pack`/`npm publish`. The vendor binary
// is gitignored — it only ever exists on the publishing machine — so without
// this rebuild the published tarball would ship whatever a previous dev
// build left in vendor/ (a 0.3.2 binary sat here while the package said
// 0.4.0). Building from the repo root with the package version stamped
// guarantees the fallback binary matches what is being published.

const { spawnSync } = require("node:child_process");
const path = require("node:path");

const pkg = require("./package.json");
const repoRoot = path.resolve(__dirname, "..", "..");
const exe = process.platform === "win32" ? "codedocket.exe" : "codedocket";

const result = spawnSync(
  "go",
  [
    "build",
    "-trimpath",
    "-ldflags",
    `-X main.version=${pkg.version}`,
    "-o",
    path.join(__dirname, "vendor", exe),
    "./cmd/codedocket",
  ],
  { cwd: repoRoot, stdio: "inherit" }
);

if (result.error) {
  if (result.error.code === "ENOENT") {
    console.error(
      "prepack: the Go toolchain is required to build the vendor binary (install go, or pack with --ignore-scripts to skip the rebuild)."
    );
    process.exit(1);
  }
  throw result.error;
}
process.exit(result.status === null ? 1 : result.status);
