import { writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { readSingleArgument } from "./cli-args.mjs";

const miniappRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const rawUrl = readSingleArgument(
  process.argv.slice(2),
  "usage: pnpm configure:integration -- https://your-api.example.com",
);

const url = new URL(rawUrl);
const loopback = url.hostname === "localhost" || url.hostname === "127.0.0.1" || url.hostname === "::1";
if (url.username || url.password || url.search || url.hash) {
  throw new Error("API base URL must not contain credentials, query parameters, or a fragment");
}
if (url.protocol !== "https:" && !(url.protocol === "http:" && loopback)) {
  throw new Error("real-device integration requires HTTPS; HTTP is allowed only for loopback development");
}

const apiBaseUrl = url.toString().replace(/\/$/u, "");
await writeFile(
  resolve(miniappRoot, ".env.local"),
  `TARO_APP_VALIDATION_MODE=integration\nTARO_APP_API_BASE_URL=${apiBaseUrl}\n`,
  "utf8",
);
process.stdout.write(`integration target configured: ${url.origin}\n`);
