#!/usr/bin/env node

const fs = require("fs/promises");
const path = require("path");

const packageRoot = path.resolve(__dirname, "..");
const repoRoot = path.resolve(packageRoot, "..", "..");
const packageJSON = require(path.join(packageRoot, "package.json"));

const outputRoot = path.resolve(process.argv[2] || path.join(packageRoot, "dist", "platform-packages"));
const version = process.argv[3] || packageJSON.version;

const platforms = [
  { platform: "darwin", arch: "amd64", os: "darwin", cpu: "x64" },
  { platform: "darwin", arch: "arm64", os: "darwin", cpu: "arm64" },
  { platform: "linux", arch: "amd64", os: "linux", cpu: "x64" },
  { platform: "linux", arch: "arm64", os: "linux", cpu: "arm64" },
  { platform: "windows", arch: "amd64", os: "win32", cpu: "x64", ext: ".exe" }
];

async function copyBinary(entry, targetDir) {
  const assetName = `arctl-${entry.platform}-${entry.arch}${entry.ext || ""}`;
  const source = path.join(repoRoot, "bin", assetName);
  const target = path.join(targetDir, "bin", assetName);
  await fs.mkdir(path.dirname(target), { recursive: true });
  await fs.copyFile(source, target);
  if (entry.platform !== "windows") {
    await fs.chmod(target, 0o755);
  }
  return assetName;
}

function platformPackageJSON(entry, assetName) {
  const packageName = `@yaogdu-skill-hub/shub-${entry.platform}-${entry.arch}`;
  return {
    name: packageName,
    version,
    description: `${entry.platform}/${entry.arch} arctl binary for @yaogdu-skill-hub/shub`,
    homepage: packageJSON.homepage,
    repository: packageJSON.repository,
    bugs: packageJSON.bugs,
    license: packageJSON.license,
    os: [entry.os],
    cpu: [entry.cpu],
    files: [`bin/${assetName}`],
    publishConfig: packageJSON.publishConfig
  };
}

async function main() {
  await fs.mkdir(outputRoot, { recursive: true });
  for (const entry of platforms) {
    const packageDir = path.join(outputRoot, `shub-${entry.platform}-${entry.arch}`);
    const assetName = await copyBinary(entry, packageDir);
    const json = platformPackageJSON(entry, assetName);
    await fs.writeFile(path.join(packageDir, "package.json"), `${JSON.stringify(json, null, 2)}\n`);
    console.log(packageDir);
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
