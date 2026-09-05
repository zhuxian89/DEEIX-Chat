# DEEIX Chat 微信小程序

这是 DEEIX Chat 的正式微信小程序客户端。工程使用 Taro 4.2.1、React 18.3.1、TypeScript strict 和 Webpack 5，只编译微信小程序端，并通过目录内独立的 pnpm workspace 与主仓库隔离。

小程序提供微信一键登录、最近会话、AI 对话、AI 生图和更多模型。快捷入口的模型由后端配置，客户端不会静默切换到其他价格的模型。用户、订阅、余额、计费、会话和文件继续使用 DEEIX 现有后端体系。

## 目录边界

`pnpm-workspace.yaml` 和 `pnpm-lock.yaml` 归本目录所有。不要把 `miniapp/` 加入仓库根 workspace 或 Turbo pipeline。微信开发者工具应导入本目录，`miniprogramRoot` 已指向 `dist/`。

## 后端生产配置

在 Web 管理后台“参数配置”中设置：

```text
wechat_miniapp:enabled=true
wechat_miniapp:app_id=wx59fcdf6143e32cef
wechat_miniapp:app_secret=从微信公众平台获取的 AppSecret
wechat_miniapp:default_chat_model=管理员模型页中的 platformModelName
wechat_miniapp:default_image_model=管理员模型页中的 platformModelName
```

要求：

- `app_id` 必须与构建/发布小程序使用的 AppID 一致。
- 两个默认模型填写 DEEIX 模型目录中的 `platformModelName`，不是展示名、供应商原始 ID 或数据库数字 ID。
- 默认模型必须已启用，并对快捷注册用户所属权限组开放相应的 `chat` 或 `image_gen` 能力。
- `app_secret` 由后端使用 `DATA_ENCRYPTION_KEY` 加密保存，禁止写入本目录、前端环境变量或微信项目配置。
- 关闭 `wechat_miniapp:enabled` 会使一键登录返回稳定的服务不可用错误，不影响 Web 登录和公众号注册码流程。

后端首次启动会通过现有 schema 迁移创建独立的 `wechat_miniapp_bindings` 表。它不会修改既有用户、计费、会话或公众号表结构。

## 配置和构建

首次安装只在本目录启动一次安装器：

```bash
pnpm install --reporter=append-only
```

配置本机小程序项目和 HTTPS 后端：

```bash
pnpm configure:weapp
pnpm configure:integration -- https://chat.example.com
pnpm check
```

小程序 AppID 已固定为正式账号 `wx59fcdf6143e32cef`，`project.config.json` 纳入版本管理，配置脚本拒绝写入其他 AppID。后端地址脚本只写入已忽略的 `.env.local`；生产源码只读取 `TARO_APP_API_BASE_URL`，不保存 AppSecret、OpenID、UnionID 或任何令牌。

也可以持续编译：

```bash
pnpm dev:weapp
```

## 微信开发者工具

1. 导入 `miniapp/`，不要只导入 `dist/`。
2. 确认详情中的小程序 AppID 与后台参数 `wechat_miniapp:app_id` 一致。
3. “项目配置”中的小程序目录应为 `dist/`。
4. 建议使用已发布的稳定基础库，不把灰度基础库作为上线唯一验证结果。
5. 修改微信后台域名后，在“详情 → 域名信息”刷新项目配置并重新编译。

## 开发版本上传操作规范

本节是小程序上传流程的维护入口；高频约束由 `.codestable/attention.md` 引用。上传开发版本不等于提审或正式发布。

### 范围与前置检查

- 仅修改小程序且无需后端部署时，按 owner 的既有要求，相关验证通过后直接上传开发版本，不重复询问。上传读取本地构建产物，不要求先 commit 或 push；Git 提交/推送仍需当次明确授权。微信后台的版本管理、提审和发布由 owner 操作。
- 已配置正确的 AppID、后端地址和域名不要重复配置，依赖已可用时不要重跑安装器。文档或记忆维护本身不需要重新打包上传。
- 唯一正式 AppID：`wx59fcdf6143e32cef`。工程根必须是 `miniapp/`，由 `project.config.json` 的 `miniprogramRoot: "dist/"` 指向产物；禁止以 `miniapp/dist/` 作为 CLI 工程根。
- 本机 CLI 路径为 `C:\Program Files (x86)\Tencent\微信web开发者工具\cli.bat`，已使用的服务端口为 `37750`。端口是本机配置，不是平台常量；连接失败先检查开发者工具“设置 → 安全设置 → 服务端口”和实际监听状态。
- 每个新代码包先更新 `src/product/build-version.ts`，然后核对源码版本、产物内版本与上传标签三者一致。只修改上传命令的版本标签不会更新代码包；不要把旧版本写死在原生页面标题中。

### 每次上传的固定顺序

先运行与改动有关的测试和类型检查，再依次清理 `compile` 缓存、清理 `file` 缓存、生产构建、检查产物、上传。任一步失败都停止后续步骤，不能拿旧 `dist` 继续上传。以下 PowerShell 命令从 `miniapp/` 目录执行；每步检查退出码，未成功不运行下一步。

```powershell
$miniappProject = (Resolve-Path .).Path
$wechatCli = 'C:\Program Files (x86)\Tencent\微信web开发者工具\cli.bat'
$wechatPort = 37750

& $wechatCli cache --clean compile --project $miniappProject --port $wechatPort --lang zh
if ($LASTEXITCODE -ne 0) { throw '清理 compile 缓存失败' }
& $wechatCli cache --clean file --project $miniappProject --port $wechatPort --lang zh
if ($LASTEXITCODE -ne 0) { throw '清理 file 缓存失败' }
pnpm build:weapp
if ($LASTEXITCODE -ne 0) { throw '生产构建失败，禁止上传旧产物' }
pnpm check:dist
if ($LASTEXITCODE -ne 0) { throw '构建产物检查失败' }
```

`check:dist` 已校验正式 AppID、当前源码版本存在于页面包、运行时安全及 Markdown 产物；它**不等于排除了所有旧版本字符串**。上传前还要核对页面包中只出现本次预期构建版本，并确认本次修改已进入产物。可以列出页面包内版本：

```powershell
node -e "const fs=require('node:fs');const s=fs.readFileSync('dist/pages/index/index.js','utf8');console.log([...new Set(s.match(/0\.1\.\d+\.\d{8}/g))]);"
```

核对无误后，读取源码版本作为上传标签，描述填写本次实际变更。CLI 参数是 `--desc`，不是 `--description`：

```powershell
$buildVersion = node -p "require('node:fs').readFileSync('src/product/build-version.ts','utf8').match(/0\.1\.\d+\.\d{8}/)[0]"
if ($LASTEXITCODE -ne 0) { throw '读取构建版本失败' }
& $wechatCli upload --project $miniappProject --port $wechatPort --version $buildVersion --desc '填写本次实际变更说明' --lang zh
if ($LASTEXITCODE -ne 0) { throw '微信 CLI 上传失败' }
```

### 回执、版本错乱与真机验收

- **本地验证**：只报告实际运行的检查、结果和构建版本，不把定向检查写成全量回归。
- **CLI 回执**：记录正式 AppID、版本、退出码和 `√ upload`。回执成功但没有后台证据时，只能说“微信工具返回上传成功”。
- **后台确认**：正式账号后台出现对应开发版本后，才算完成后台版本确认。不能根据 CLI 标签推断自己已看到后台。
- **真机验收**：确认首页运行版本后，按原始触发动作检查症状。键盘遮挡、快速甩动到底、图片加载后高度变化等必须验证交互结果，源码里存在 `bounces={false}` 等属性不等于症状已消失。没有真机证据时明确待验收。

若扫码页版本新、首页版本旧，先检查上传工程根、正式 AppID、双缓存清理结果和构建产物，不反复换标签上传，也不反复要求 owner 清手机缓存。若首页已是新版本但问题仍在，则转回功能诊断；静态源码检查只能证明代码结构符合预期，不能当作实际滚动或裁切问题的复现与修复证据。

上述规则的依据：2026-09-03 同一 `0.1.16` 上传标签曾运行 `0.1.13` / `0.1.9`，按工程根、双缓存、构建及产物检查流程处理后才正确运行；2026-09-04 的 `0.1.36` 虽通过定向检查和上传，owner 确认运行版本后仍复现图片底部回弹/裁切，因此上传成功与症状验收必须分开。不要把后续候选修复仅凭上传回执记为真机已修复。

上传前遇到内存不足、工具线程启动失败或命令暂时无输出，先检查对应进程是否结束及资源状态；未结束就继续等待，不启动重复构建或安装。确实失败且资源恢复后才做一次有界重试，不擅自结束用户其他进程，不跳过构建和产物检查。

## 图片滚动闪动的排查与回归

已确认案例：iPhone 真机在 `0.1.37.20260904` 的 AI 生图对话中，手指仍按着滑动时图片闪一下、列表突然移位，最后一张图片无法稳定看全。关闭回弹、关闭滚动锚定和调整容器高度后仍复现；不能把“松手后的惯性”当作未经核实的前提。

根因证据来自当前安装的 Taro 4.2.1 运行时：图片滚动区域旁边的“回到底部”按钮采用条件渲染，自动跟随状态变化时会增删这个兄弟节点。实际页面 JSX 经 Taro 渲染后，按钮删除产生了 `root.cn.[0].cn` 整段更新，其中重新包含图片和之前的 `scrollTop=999999`。只看 React 的组件标识、源码中的 `bounces={false}` 或布局属性，无法发现这次原生数据重发。

修复保留按钮节点，只切换 `display`。运行时回归检查覆盖按钮反复显示和隐藏，要求 `setData` 仅更新按钮属性，不重发图片子树和旧滚动目标；维护入口是 `src/product/chat-auto-scroll.test.ts` 的 `manual image scrolling only updates the bottom button, not the native image subtree`。

以后遇到同类闪动或跳位，按以下顺序排查：

1. 以用户确认的触发动作建立前提，区分手指仍按着、松手后滚动和图片加载完成等场景；静态截图不能证明事件顺序。
2. 追踪滚动事件引起的状态变化，以及这些状态控制的节点增删；检查当前版本 Taro 实际发送的 `setData` 路径和内容，不止检查 JSX 或 CSS。可直接用安装的运行时渲染实际页面片段，捕获数据更新，无需先让 owner 反复录屏或增加真机日志。
3. 对仅改变可见性的邻近控件，验证保留节点能否把更新限制在控件属性；真实消息增删仍需单独验证，不能将此结论泛化为所有节点都必须常驻。

定向验证命令：

```powershell
pnpm exec tsx --test src/product/chat-auto-scroll.test.ts
```

该检查在修复前因重发图片与旧滚动目标失败，修复后 6/6 通过；类型检查、生产构建和产物检查通过。`0.1.38.20260905` 上传后，owner 于 2026-09-05 在本次 iPhone 真机场景确认问题解决。自动检查负责阻止此数据重发路径复发，真机确认负责验证实际交互效果。

## 合法域名

在微信公众平台的“开发管理 → 开发设置 → 服务器域名”中，把后端 HTTPS Origin（例如 `https://chat.example.com`）同时加入：

- `request 合法域名`：登录、模型、会话、消息流和文件上传。
- `downloadFile 合法域名`：生图结果和受保护文件下载。
- `uploadFile 合法域名`：对话图片附件上传。

只填写 Origin，不带 `/api/v1` 路径，不要填写 `http://`、IP、localhost 或带端口的临时地址。证书链必须受信任且 TLS 配置符合微信要求。

## 运行时安全边界

- 冷启动调用 `wx.login`，一次性 code 只发送给 DEEIX 后端。
- access token 和 refresh Cookie 只保存在小程序进程内存；不写入 storage、日志或 UI 状态。
- refresh Cookie 仅向 `/api/v1/auth/refresh` 发送，每次轮换覆盖旧值。
- 进程被杀后重新执行微信登录；后端只撤销旧小程序会话，不影响 Web 会话。
- 当前身份只认同一 AppID 下的 OpenID。UnionID 可空留档，本期不用于关联或合并 Web/公众号账号。

## 发布前检查表

代码构建成功不等于已经具备微信发布资格。上传审核前逐项完成并留证：

- 小程序主体、名称、服务类目与实际 AI 对话/图片生成能力一致。
- 按所在地和微信规则完成小程序备案、域名备案及 HTTPS 证书配置。
- 在微信后台配置《小程序用户隐私保护指引》，声明登录标识、相册/相机、上传图片和生成内容的用途。
- 提供可访问的用户协议、隐私政策、客服与账号注销说明；当前客户端已提供 AI 内容提示，但法律文本和主体信息必须由运营方确认。
- 核对生成式 AI 服务备案/登记、内容安全、生成内容标识和投诉处置要求；未满足时不得提交生产审核。
- 真机验证首次登录、杀进程后同一用户、余额/订阅一致、多轮对话、停止、图片附件、生图、保存相册、网络切换和 refresh 轮换。
- 确认生产环境未开启开发者工具的“不校验合法域名”选项。

## 验证命令

```bash
pnpm typecheck
pnpm test
pnpm build:weapp
pnpm check:dist
pnpm check:isolation
```

`pnpm check` 会按顺序运行全部检查。`check:dist` 会扫描运行时产物中的 Node 全局变量、开发配置和敏感标记；`check:isolation` 会确认独立 workspace、lockfile、私密配置忽略规则及本地 API contract 依赖。
