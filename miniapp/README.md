# DEEIX Chat 微信小程序

这是 DEEIX Chat 的正式微信小程序客户端。工程使用 Taro 4.2.1、React 18.3.1、TypeScript strict 和 Webpack 5，只编译微信小程序端，并通过目录内独立的 pnpm workspace 与主仓库隔离。

小程序提供微信一键登录、最近会话、AI 对话、AI 生图和更多模型。快捷入口的模型由后端配置，客户端不会静默切换到其他价格的模型。用户、订阅、余额、计费、会话和文件继续使用 DEEIX 现有后端体系。

## 目录边界

`pnpm-workspace.yaml` 和 `pnpm-lock.yaml` 归本目录所有。不要把 `miniapp/` 加入仓库根 workspace 或 Turbo pipeline。微信开发者工具应导入本目录，`miniprogramRoot` 已指向 `dist/`。

## 后端生产配置

在 Web 管理后台“参数配置”中设置：

```text
wechat_miniapp:enabled=true
wechat_miniapp:app_id=wx0123456789abcdef
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
pnpm configure:weapp -- wx0123456789abcdef
pnpm configure:integration -- https://chat.example.com
pnpm check
```

两个配置脚本同时兼容 pnpm 转发的 `--`，不会再把它误解析为 URL。它们只写入已忽略的 `project.config.json` 和 `.env.local`。生产源码只读取 `TARO_APP_API_BASE_URL`，不保存 AppSecret、OpenID、UnionID 或任何令牌。

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
