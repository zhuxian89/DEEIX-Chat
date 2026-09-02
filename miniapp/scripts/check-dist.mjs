import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const miniappRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const applicationBundles = ["dist/app.js", "dist/pages/index/index.js"];
const forbiddenRuntimePatterns = [
  { pattern: /\bprocess(?:\.|\[)/u, label: "a Node.js process reference" },
  { pattern: /\b(?:localStorage|setStorage|setStorageSync)\b/u, label: "persistent token storage capability" },
  { pattern: /(?:password|access|refresh)-secret/u, label: "a test secret" },
];

for (const relativePath of applicationBundles) {
  const source = await readFile(resolve(miniappRoot, relativePath), "utf8");
  for (const { pattern, label } of forbiddenRuntimePatterns) {
    if (pattern.test(source)) {
      throw new Error(`${relativePath} contains ${label}`);
    }
  }
}

process.stdout.write("miniapp runtime bundle checks passed\n");
