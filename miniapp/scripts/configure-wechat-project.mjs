import { readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { readSingleArgument } from "./cli-args.mjs";

const miniappRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const officialAppID = "wx59fcdf6143e32cef";
const appId = readSingleArgument(
  process.argv.slice(2),
  `usage: pnpm configure:weapp [${officialAppID}]`,
  officialAppID,
);
if (appId !== officialAppID) {
  throw new Error(`This project only supports the official AppID ${officialAppID}`);
}

const template = JSON.parse(await readFile(resolve(miniappRoot, "project.config.example.json"), "utf8"));
template.appid = officialAppID;
await writeFile(resolve(miniappRoot, "project.config.json"), `${JSON.stringify(template, null, 2)}\n`, "utf8");
process.stdout.write(`WeChat project configured for ${officialAppID}\n`);
