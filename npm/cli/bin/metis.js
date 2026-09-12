#!/usr/bin/env node

const { spawnSync } = require("node:child_process");

const platformPackages = {
  "linux-x64": "@vector-metis/cli-linux-x64",
  "linux-arm64": "@vector-metis/cli-linux-arm64",
  "darwin-x64": "@vector-metis/cli-darwin-x64",
  "darwin-arm64": "@vector-metis/cli-darwin-arm64",
  "win32-x64": "@vector-metis/cli-win32-x64",
};

const key = `${process.platform}-${process.arch}`;
const packageName = platformPackages[key];
if (!packageName) {
  console.error(`@vector-metis/cli does not support ${key}`);
  process.exit(1);
}

let binary;
try {
  binary = require(packageName).bin;
} catch (error) {
  console.error(`@vector-metis/cli is missing its platform package ${packageName}`);
  console.error("Reinstall the package with npm so optional dependencies are installed.");
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
if (result.error) {
  console.error(`failed to start metis: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status ?? 1);
