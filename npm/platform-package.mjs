import { mkdir, copyFile, chmod, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";

const [, , packageDir, binaryPath, version, binaryName = "metis"] = process.argv;
if (!packageDir || !binaryPath || !version || !process.env.NPM_PACKAGE_NAME) {
  throw new Error("usage: NPM_PACKAGE_NAME=name node platform-package.mjs <package-dir> <binary> <version> [binary-name]");
}
const destination = resolve(packageDir, "bin", binaryName);
await mkdir(dirname(destination), { recursive: true });
await copyFile(resolve(binaryPath), destination);
if (!destination.endsWith(".exe")) await chmod(destination, 0o755);
await writeFile(resolve(packageDir, "package.json"), JSON.stringify({
  name: process.env.NPM_PACKAGE_NAME,
  version,
  description: "Metis CLI platform binary",
  repository: {
    type: "git",
    url: "https://github.com/vector-metis/metis-cli.git",
  },
  license: "Apache-2.0",
  files: ["bin"],
  main: "index.js",
}, null, 2) + "\n");
await writeFile(resolve(packageDir, "index.js"), `exports.bin = require("node:path").join(__dirname, "bin", ${JSON.stringify(binaryName)});\n`);
