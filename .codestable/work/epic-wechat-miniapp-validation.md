---
epic: ../epics/wechat-miniapp-validation.md
phase: executing
approved_revision: 6d42828d25d5ae1a713f8b5de776f42d11168729fe7e411b8cac94779875e75c
current_item: ITEM-3
next_action: owner 在真机分别进行连续对话和按需生图，回传实际结果
blocked_by: null
item_progression: continuous
milestone_commit: manual
remote_publish: manual
---

## 子项进度

- [x] ITEM-1
- [x] ITEM-2
- [ ] ITEM-3
- [ ] ITEM-4

## 临时决策与证据

- 2026-09-01：owner 确认 Taro 路线，并要求使用独立子文件夹以降低上游同步冲突。
- 2026-09-01：npm registry 核验 Taro 与 uni-app 持续发布，Remax 最新发布时间停留在 2022；Taro 官方类型包含微信端 `enableChunked`、`onChunkReceived`、`abort` 和响应 `cookies`。
- 2026-09-01：owner 明确豁免本 Epic 的独立 reviewer，由主 agent 完成设计审查；全局子 agent 禁令保持有效。
- 2026-09-01：主 agent design review 首轮发现两个 important：嵌套目录需要自己的 pnpm workspace/lock 边界；离线协议验证与真实后端/账号/模型调用需要分道并保护秘密。来源流程已修订完整 Epic，等待同一审查阶段复审。
- 2026-09-01：主 agent design review 复审目标 `2ba266be230bffa12e820adcd79e3a2acd5a1d04d70ba7c41ea3a0cff0db6c4e`；两个 important 均 resolved，无 blocking / important / new findings，结论为通过。等待 owner gate。
- 2026-09-01：owner 确认最终契约，选择 `per-item / manual / manual`，明确不 commit、不 push；Epic 激活，批准 revision 为 `6d42828d25d5ae1a713f8b5de776f42d11168729fe7e411b8cac94779875e75c`，开始 ITEM-1。
- 2026-09-01：ITEM-1 完成。`miniapp/` 使用独立 workspace 与 lockfile，Taro 4.2.1 / React 18.3.1 / Webpack 5 骨架、API contract `file:` 类型 smoke、离线/集成配置安全门槛、隔离 checker 和最小微信页面已生成。验证：`pnpm check:isolation` 通过；`pnpm typecheck` 通过；Node test 10/10 通过；`pnpm build:weapp` 25.32 秒成功；`dist/app.js`、`app.json`、页面 JS 存在；根 package/workspace/lock/turbo 四文件 SHA-256 安装前后完全一致。安装过程暴露的 Taro 未使用 Less peer 仅为 warning；测试工具改用 Node test + tsx，避免 Vitest/Vite 与 Taro Vite 4 peer 冲突。未 commit、未 push。
- 2026-09-01：owner 撤销逐项暂停，要求 ITEM-2 一次性交付到可真机调试状态；执行改为 continuous，但继续保持 `milestone_commit: manual` / `remote_publish: manual`，不 commit、不 push，不再设置中间确认点。
- 2026-09-01：ITEM-2 完成。已实现 `POST /api/v1/auth/login`、`deeix_chat_refresh_token` Cookie 脱敏属性观察、`GET /api/v1/me` Bearer 校验，以及连续两次 `POST /api/v1/auth/refresh` 轮换与每轮校验；不手动保存或重发 refresh token，密码和 access token 仅驻留内存，2FA 账号明确拒绝用于本探针。已提供微信 AppID 与集成后端地址配置脚本及真机调试页面。统一验证 `pnpm check` 通过：TypeScript 通过、Node test 15/15 通过、微信构建 6.83 秒成功、隔离 checker 通过；配置脚本已验证；根 package/workspace/lock/turbo 四文件 SHA-256 保持不变；应用源码及 app/page bundle 未发现 token 持久化或 storage 调用。未 commit、未 push。
- 2026-09-02：真机验证确认微信原生请求层能观察登录 `Set-Cookie`，但不会为 refresh 自动续传可用 Cookie；登录与 Bearer 校验通过，首次 refresh 返回 `401 auth.invalid_refresh_token`。owner 确认改用小程序专用内存 Cookie 桥接：仅在传输层闭包持有 `deeix_chat_refresh_token`，只向 `/api/v1/auth/refresh` 发送，轮换覆盖、清除销毁、异常字符丢弃，不进入 React state、storage、日志或报告；后端不改。统一验证 `pnpm check` 通过：TypeScript 通过、Node test 23/23 通过、微信构建 6.97 秒成功、隔离与运行时产物检查通过，`dist` 未发现 Node `process`、持久化 storage 能力或测试秘密。未 commit、未 push。
- 2026-09-02：owner 在微信开发者工具基础库 3.17.2 中完成真机复测，页面最终状态为“认证链路通过”；账号登录、登录 Cookie 脱敏观察、首次 Bearer 鉴权、两次 refresh 轮换及每次轮换后的 Bearer 鉴权全部通过，证明内存 Cookie 桥接与现有后端契约兼容。
- 2026-09-02：owner 要求继续验证真实对话与生图。`miniapp/` 已新增微信原生 `enableChunked` / `onChunkReceived` 流式传输、手写增量 UTF-8 解码和 JSON 文档解析、原生 abort、模型目录自动选择、真实会话创建、聊天 NDJSON 与生图 NDJSON 探针、内联图片与受保护文件下载回退；仅改小程序新代码，后端与 Web 不动。离线验证覆盖中文/Emoji 跨字节、任意 JSON 边界、同块多文档和不完整尾块；统一 `pnpm check` 通过，Node test 27/27、微信构建 8.58 秒成功、隔离与运行时产物安全扫描通过。等待 owner 真机模型调用证据，ITEM-3 暂不标完成；未 commit、未 push。
- 2026-09-02：owner 明确模型调用应采用正常交互形态，不接受把对话与生图捆绑成一键探针。页面调整为登录并加载模型一次、在同一后端会话中连续对话并实时显示增量文本、按需单独生图；两类操作分别启动和停止，使用各自独立会话，缺少其中一类模型不会阻塞另一类能力。统一 `pnpm check` 通过：TypeScript 通过、Node test 28/28、微信构建 8.73 秒成功、隔离与运行时产物安全扫描通过。等待 owner 真机交互证据，ITEM-3 暂不标完成；未 commit、未 push。
