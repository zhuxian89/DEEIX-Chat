import { readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { readSingleArgument } from "./cli-args.mjs";

const miniappRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const appId = readSingleArgument(
  process.argv.slice(2),
  "usage: pnpm configure:weapp -- wx0123456789abcdef",
  "touristappid",
);
if (appId !== "touristappid" && !/^wx[0-9a-fA-F]{16}$/u.test(appId)) {
  throw new Error("AppID must be touristappid or wx followed by 16 hexadecimal characters");
}

const template = JSON.parse(await readFile(resolve(miniappRoot, "project.config.example.json"), "utf8"));
template.appid = appId;
await writeFile(resolve(miniappRoot, "project.config.json"), `${JSON.stringify(template, null, 2)}\n`, "utf8");
process.stdout.write(`WeChat project configured for ${appId === "touristappid" ? "offline tools" : appId}\n`);
