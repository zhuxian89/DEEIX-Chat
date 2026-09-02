import { readFile, stat } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const miniappRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

async function requireFile(relativePath) {
  const path = resolve(miniappRoot, relativePath);
  const info = await stat(path);
  if (!info.isFile()) {
    throw new Error(`${relativePath} must be a file`);
  }
  return readFile(path, "utf8");
}

const workspace = await requireFile("pnpm-workspace.yaml");
if (!/^\s*-\s+["']?\.["']?\s*$/m.test(workspace)) {
  throw new Error("miniapp/pnpm-workspace.yaml must own the current package");
}

await requireFile("pnpm-lock.yaml");
await requireFile("project.config.example.json");
await requireFile("project.config.json");

const manifest = JSON.parse(await requireFile("package.json"));
if (manifest.dependencies?.["@deeix/api-contract"] !== "file:../packages/api-contract") {
  throw new Error("API contract must remain a local file dependency");
}

const gitignore = await requireFile(".gitignore");
for (const protectedEntry of ["*.local", "project.config.json", "project.private.config.json"]) {
  if (!gitignore.split(/\r?\n/u).includes(protectedEntry)) {
    throw new Error(`.gitignore must protect ${protectedEntry}`);
  }
}

process.stdout.write("miniapp isolation checks passed\n");
