import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const miniappRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const officialAppID = "wx59fcdf6143e32cef";
const applicationBundles = ["dist/app.js", "dist/pages/index/index.js"];
const forbiddenRuntimePatterns = [
  { pattern: /\bprocess(?:\.|\[)/u, label: "a Node.js process reference" },
  { pattern: /\b(?:localStorage|setStorage|setStorageSync)\b/u, label: "persistent token storage capability" },
  { pattern: /(?:password|access|refresh)-secret/u, label: "a test secret" },
];

const projectConfig = JSON.parse(await readFile(resolve(miniappRoot, "project.config.json"), "utf8"));
if (projectConfig.appid !== officialAppID) {
  throw new Error(`project.config.json must use the official AppID ${officialAppID}`);
}

for (const relativePath of applicationBundles) {
  const source = await readFile(resolve(miniappRoot, relativePath), "utf8");
  for (const { pattern, label } of forbiddenRuntimePatterns) {
    if (pattern.test(source)) {
      throw new Error(`${relativePath} contains ${label}`);
    }
  }
}

const buildVersionSource = await readFile(resolve(miniappRoot, "src/product/build-version.ts"), "utf8");
const buildVersion = buildVersionSource.match(/MINIAPP_BUILD_VERSION\s*=\s*"([^"]+)"/u)?.[1];
if (!buildVersion) {
  throw new Error("src/product/build-version.ts does not declare MINIAPP_BUILD_VERSION");
}
const pageBundle = await readFile(resolve(miniappRoot, "dist/pages/index/index.js"), "utf8");
if (!pageBundle.includes(buildVersion)) {
  throw new Error(`dist/pages/index/index.js does not contain current build version ${buildVersion}`);
}

const markdownArtifacts = [
  {
    path: "dist/pages/index/index.json",
    patterns: [/"mp-html":"\/native-components\/mp-html\/index"/u],
  },
  {
    path: "dist/base.wxml",
    patterns: [/<mp-html\s/u, /markdown="\{\{i\.markdown\}\}"/u],
  },
  {
    path: "dist/native-components/mp-html/index.js",
    patterns: [/require\("\.\/markdown\/index\.js"\)/u, /markdown:Boolean/u],
  },
  {
    path: "dist/native-components/mp-html/node/node.js",
    patterns: [/copyCode:function/u, /setClipboardData/u],
  },
  {
    path: "dist/native-components/mp-html/node/node.wxml",
    patterns: [/catchtap="copyCode"/u, />复制<\/view>/u],
  },
  {
    path: "dist/native-components/mp-html/node/node.wxss",
    patterns: [/\.md-blockquote/u, /\.md-table/u, /\.md-pre/u, /\._codeCopy/u],
  },
  {
    path: "dist/native-components/mp-html/markdown/marked.min.js",
    patterns: [/marked/u],
  },
];

for (const artifact of markdownArtifacts) {
  const source = await readFile(resolve(miniappRoot, artifact.path), "utf8");
  for (const pattern of artifact.patterns) {
    if (!pattern.test(source)) {
      throw new Error(`${artifact.path} is missing the production Markdown renderer`);
    }
  }
}

const nativeMarkdownParser = await readFile(
  resolve(miniappRoot, "dist/native-components/mp-html/markdown/marked.min.js"),
  "utf8",
);
const forbiddenNativeSyntax = [
  /(?:\)|\}|\]|\b[A-Za-z_$][\w$]*)=>/u,
  /\{\.\.\./u,
  /\?\.|\?\?/u,
  /\\p\{/u,
];
for (const pattern of forbiddenNativeSyntax) {
  if (pattern.test(nativeMarkdownParser)) {
    throw new Error("native Markdown parser contains syntax rejected by the WeChat uploader");
  }
}

process.stdout.write("miniapp runtime bundle checks passed\n");
