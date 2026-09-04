import { cp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { createRequire } from "node:module";
import { dirname, parse, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const babel = require("@babel/core");
const presetRequire = createRequire(require.resolve("babel-preset-taro"));
const presetEnv = presetRequire("@babel/preset-env");
const EXPECTED_VERSION = "2.5.2";
const miniappRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

const miniappMarkdownStyle = `
._h1,._h2,._h3,._h4,._h5,._h6{line-height:1.35;margin:.9em 0 .45em}
.md-p{line-height:1.65;margin:.65em 0;overflow-wrap:anywhere;word-break:break-word}
.md-p:first-child{margin-top:0}
.md-p:last-child{margin-bottom:0}
.md-blockquote{background:rgba(120,120,128,.08);border-left-color:#9a8df5;border-radius:0 8px 8px 0;margin:.8em 0;padding:.45em .8em}
.md-table{font-size:.92em;margin:.8em 0}
.md-pre{background:#202127;border-radius:12px;box-sizing:border-box;color:#f4f4f5;line-height:1.55;margin:.8em 0;max-width:100%;overflow:auto;padding:14px;white-space:pre}
.md-pre .md-code{background:transparent;color:inherit;font-size:.88em;padding:0;white-space:pre}
.md-code{overflow-wrap:anywhere;word-break:break-word}
._codeWrap{padding-top:42px;position:relative}
._codeCopy{background:#2f3038;border-radius:7px;color:#d9d2ff;font-size:12px;line-height:28px;padding:0 10px;position:absolute;right:8px;top:7px;z-index:2}
._codeRichText{display:block;min-width:max-content}
`;

function replaceRequired(source, pattern, replacement, label) {
  if (!pattern.test(source)) {
    throw new Error(`mp-html ${EXPECTED_VERSION} no longer matches the ${label} integration point`);
  }
  return source.replace(pattern, replacement);
}

async function transpileNativeScript(path) {
  const source = await readFile(path, "utf8");
  const result = await babel.transformAsync(source, {
    babelrc: false,
    comments: false,
    compact: true,
    configFile: false,
    filename: path,
    presets: [
      [
        presetEnv,
        {
          bugfixes: true,
          modules: "commonjs",
          targets: { android: "5", ios: "9" },
        },
      ],
    ],
    sourceMaps: false,
    sourceType: "unambiguous",
  });
  if (!result?.code) {
    throw new Error(`failed to transpile native script ${path}`);
  }
  await writeFile(path, result.code, "utf8");
}

export async function prepareMarkdownRenderer({ packageRoot, outputRoot }) {
  const resolvedPackageRoot = resolve(packageRoot);
  const resolvedOutputRoot = resolve(outputRoot);
  if (resolvedOutputRoot === parse(resolvedOutputRoot).root) {
    throw new Error("refusing to replace a filesystem root");
  }

  const packageManifest = JSON.parse(await readFile(resolve(resolvedPackageRoot, "package.json"), "utf8"));
  if (packageManifest.version !== EXPECTED_VERSION) {
    throw new Error(`expected mp-html ${EXPECTED_VERSION}, received ${packageManifest.version ?? "unknown"}`);
  }

  const componentSourceRoot = resolve(resolvedPackageRoot, "dist/mp-weixin");
  const pluginSourceRoot = resolve(resolvedPackageRoot, "plugins/markdown");
  const markdownBuild = require(resolve(pluginSourceRoot, "miniprogram/build.js"));

  await rm(resolvedOutputRoot, { force: true, recursive: true });
  await mkdir(dirname(resolvedOutputRoot), { recursive: true });
  await cp(componentSourceRoot, resolvedOutputRoot, { recursive: true });
  await mkdir(resolve(resolvedOutputRoot, "markdown"), { recursive: true });
  const markdownPluginPath = resolve(resolvedOutputRoot, "markdown/index.js");
  const markedPath = resolve(resolvedOutputRoot, "markdown/marked.min.js");
  await Promise.all([
    cp(resolve(pluginSourceRoot, "index.js"), markdownPluginPath),
    cp(resolve(pluginSourceRoot, "marked.min.js"), markedPath),
  ]);
  await Promise.all([transpileNativeScript(markdownPluginPath), transpileNativeScript(markedPath)]);

  const indexPath = resolve(resolvedOutputRoot, "index.js");
  let componentSource = await readFile(indexPath, "utf8");
  componentSource = replaceRequired(
    componentSource,
    /r=\[\];Component/u,
    'r=[require("./markdown/index.js")];Component',
    "plugin registry",
  );
  componentSource = replaceRequired(
    componentSource,
    /properties:\{/u,
    "properties:{markdown:Boolean,",
    "component properties",
  );
  componentSource = replaceRequired(
    componentSource,
    /navigateTo:function\(e,n\)\{var o=this;/u,
    "navigateTo:function(e,n){e=this._ids[decodeURI(e)]||e;var o=this;",
    "heading anchor",
  );
  await writeFile(indexPath, componentSource, "utf8");

  const nodeTemplatePath = resolve(resolvedOutputRoot, "node/node.wxml");
  let nodeTemplate = await readFile(nodeTemplatePath, "utf8");
  nodeTemplate = replaceRequired(
    nodeTemplate,
    /<rich-text wx:else id="\{\{n\.attrs\.id\}\}" style="\{\{n\.f\}\}" user-select="\{\{opts\[4\]\}\}" nodes="\{\{\[n\]\}\}"\/>/u,
    '<view wx:elif="{{n.name===\'pre\'}}" id="{{n.attrs.id}}" class="_pre {{n.attrs.class}} _codeWrap" style="{{n.attrs.style}}"><view class="_codeCopy" data-i="{{i}}" catchtap="copyCode">复制</view><rich-text class="_codeRichText" user-select="{{opts[4]}}" nodes="{{n.children}}"/></view><rich-text wx:else id="{{n.attrs.id}}" style="{{n.f}}" user-select="{{opts[4]}}" nodes="{{[n]}}"/>',
    "code block template",
  );
  await writeFile(nodeTemplatePath, nodeTemplate, "utf8");

  const nodeSourcePath = resolve(resolvedOutputRoot, "node/node.js");
  let nodeSource = await readFile(nodeSourcePath, "utf8");
  nodeSource = replaceRequired(
    nodeSource,
    /methods:\{noop:function\(\)\{\},getNode/u,
    'methods:{copyCode:function(t){var e=this.getNode(t.currentTarget.dataset.i),r=this.root.getText([e]).replace(/\\n$/u,"");r&&wx.setClipboardData({data:r})},noop:function(){},getNode',
    "code copy method",
  );
  await writeFile(nodeSourcePath, nodeSource, "utf8");

  const nodeStylePath = resolve(resolvedOutputRoot, "node/node.wxss");
  const nodeStyle = await readFile(nodeStylePath, "utf8");
  await writeFile(nodeStylePath, `${markdownBuild.style}\n${miniappMarkdownStyle}\n${nodeStyle}`, "utf8");
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await prepareMarkdownRenderer({
    packageRoot: resolve(miniappRoot, "node_modules/mp-html"),
    outputRoot: resolve(miniappRoot, "src/native-components/mp-html"),
  });
  process.stdout.write(`prepared mp-html ${EXPECTED_VERSION} with Markdown support\n`);
}
