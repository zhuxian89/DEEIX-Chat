---
status: active
created: 2026-09-01
work: ../work/epic-wechat-miniapp-validation.md
---

# 微信小程序客户端技术验证

## 起点

DEEIX Chat 已有成熟的 Next.js Web 客户端和 Go HTTP API，仓库提供生成的 TypeScript API contract。当前没有微信小程序客户端；仓库中的既有“微信”能力是微信公众号回调与管理能力，不能作为小程序客户端实现复用。

现有认证由 Bearer access token 与仅通过 `HttpOnly` Cookie 轮换的 refresh token 组成；聊天响应使用 `application/x-ndjson` 流。二者都具备小程序适配的协议基础，但 Cookie 生命周期、分块响应、UTF-8 跨块解码和真机行为尚未验证。

## 目标

在仓库根目录新增完全隔离的 `miniapp/` 技术验证工程，采用成熟、通用且适配现有 React/TypeScript 与 HTTP API 的技术栈，验证现有后端在不修改代码的前提下能否安全支撑微信小程序的登录续期、鉴权请求、流式聊天和基础文件传输，并给出有证据的 go / conditional-go / no-go 结论。

## 范围

- 在独立 `miniapp/` 目录内建立 Taro 4.x、React 18、TypeScript strict、pnpm 技术验证工程，首期只编译微信小程序端；目录内自带 workspace 边界和 lockfile，不向上写入根 workspace。
- 通过 `file:../packages/api-contract` 只读复用现有生成类型；不复制或修改 contract 源文件。
- 实现验证专用的请求、Cookie 观察、access token、分块响应与 UTF-8/NDJSON 解析适配器。
- 验证登录、连续 refresh 轮换、应用重启后的会话恢复、受保护请求、流式消息、停止、按序恢复、图片上传和受保护文件下载。
- 用自动化测试、微信开发者工具和可用时的真机证据记录验证结果。
- 把不接触真实账号/模型的离线协议测试与连接既有后端的集成验证分开；真实后端地址、测试账号和允许产生的模型调用必须由 owner 明确指定。
- 验证阶段发现后端不兼容时只记录证据与最小替代建议，不修改后端。

## 非目标

- 不交付可上线的完整小程序或正式产品 UI。
- 不实现微信一键登录、OpenID/UnionID 账号绑定、微信支付、订阅消息或小程序审核材料。
- 不移植 Web 管理后台、知识库管理、MCP 配置、PDF/DOCX 预览、Mermaid、Monaco 或 HTML Artifact。
- 不使用 WebView 套壳作为核心聊天方案。
- 不修改根 `package.json`、根 lockfile、`pnpm-workspace.yaml`、`turbo.json`、`frontend/`、`backend/` 或 `packages/`。
- 不提交、不 push、不发布，也不配置外部微信后台或生产域名，除非 owner 另行明确授权。
- 不默认连接生产环境、创建生产数据或调用付费模型；验证凭据、Cookie、token、AppID 私密配置和真实 API 地址不得提交或写入验证报告。

## 共享语言与概念边界

- **微信小程序客户端**：由 `miniapp/` 内的 Taro 工程编译并运行在微信小程序环境中的第二前端客户端。
  - 不包括：现有微信公众号回调、公众号管理页面、H5 WebView 套壳。
  - 关系 / 不变量：与 Web 客户端共享同一后端业务事实，但不共享 React 运行时或浏览器 UI 组件。
  - 来源：本 Epic 的目标与范围。
- **后端零改动**：验证和候选路线不修改 `backend/` 源码、HTTP 契约或既有数据表。
  - 不包括：微信公众平台侧的合法域名配置、测试 AppID 配置，以及部署环境已有 HTTPS 域名的只读使用。
  - 关系 / 不变量：一旦安全登录续期必须依赖新接口，验证结论必须降为 `conditional-go` 或 `no-go`，不得在本 Epic 内自行扩展后端。
  - 来源：owner 约束与仓库 fork 上游同步规则。
- **技术验证**：只为证明平台兼容性而存在的最小工程、自动化测试和真机探针。
  - 不包括：正式视觉设计、完整导航、产品级错误文案和上线承诺。
  - 关系 / 不变量：验证代码可以成为后续客户端基础，但本 Epic 的验收只对兼容性结论负责。
  - 来源：本 Epic 范围。

端到端边界场景：用户通过既有账号登录，小程序获得 access token 和服务端 Cookie；access token 失效后在不把 refresh token 明文写入普通持久化存储或日志的前提下完成两次轮换；随后向既有聊天接口发送消息，首个分块在请求结束前到达，中文与 Emoji 无损显示，用户可中止并按服务端序号恢复。任一安全或协议不变量失败都必须显式降低最终结论。

## 验收标准

1. `miniapp/` 以目录内 `pnpm-workspace.yaml` 和 lockfile 建立独立安装边界，能独立安装、类型检查、测试并构建微信小程序产物；除本 Epic 文档与游标外，本任务 diff 不触及隔离边界外的文件，根 lockfile 安装前后保持不变。
2. Taro 依赖锁定到执行时验证过的 4.x 稳定版本，React 锁定在 Taro 官方 peer 支持的 18.x，不借用 Web 的 React 19 运行时。
3. 小程序代码能直接消费 `@deeix/api-contract` 类型，且不修改生成 contract。
4. 认证矩阵至少覆盖：登录、Bearer 鉴权、连续两次 refresh 轮换、错误 Cookie、access token 失效、应用重启后的恢复；不得在日志或普通持久化存储中泄露 refresh token。
5. 流式矩阵至少覆盖：首块早于请求完成、任意 JSON 边界、中文 UTF-8 跨块、Emoji、多个文档同块、尾块、中止、错误事件与 `afterSeq` 恢复。
6. 文件矩阵至少覆盖一张图片上传和一个鉴权文件下载；若受 AppID、合法域名或真机资源阻塞，必须把已完成的本地证据与未完成的外部门槛分开报告，不得宣称整体通过。
7. 离线通道使用确定性 fixture/mock 验证请求封装和分块解析；集成通道只有在 owner 明确提供或指定非生产目标后才发送真实请求。两类证据在报告中分栏，不用 mock 结果冒充后端或真机兼容性。
8. `.env.example` 只包含占位符；真实环境变量、Cookie、token、账号、AppID 私密配置和 API 地址只进入被忽略的本地配置且不输出到日志、快照或报告。
9. 最终报告逐项给出环境、操作、证据、结果和限制，并作出 `go`、`conditional-go` 或 `no-go` 之一；`conditional-go` 必须列出生产前不可省略的后端或平台条件。
10. 现有 Web 与后端行为保持不变，不以验证为由修改上游既有代码、表或认证主流程。

## 关键决策

- **DEC-1 · 独立目录隔离**：全部技术验证工程、局部 `pnpm-workspace.yaml`、局部依赖锁、源码、测试和验证记录只放在根级 `miniapp/`；不把它注册进根 workspace/turbo，也不修改根 lockfile，从结构上缩小上游合并冲突面。
- **DEC-2 · Taro React 路线**：采用 Taro 4.x + React 18 + TypeScript；Taro 与 uni-app 都活跃，但 Taro 能延续本项目 React/TypeScript 能力并保留直接调用微信原生 API 的逃生口，Remax 发布停滞，微信原生方案则增加长期工程与跨端成本。
- **DEC-3 · 只共享契约与纯逻辑**：小程序不导入 Next.js、React 19 或浏览器 UI 依赖；只复用 API contract 和经确认不依赖 DOM 的纯 TypeScript 逻辑。
- **DEC-4 · 原生网络适配器**：普通请求封装 Taro Request；流式请求显式使用 `enableChunked`、`RequestTask.onChunkReceived` 与 `abort`，不以 axios/fetch polyfill 隐藏微信平台语义。
- **DEC-5 · 认证是安全停止门槛**：能读到响应 Cookie 不等于可以安全持久化 refresh token。若平台不能在保持敏感令牌边界的前提下完成重启后续期，本 Epic 不把后端零改动判为生产可行。
- **DEC-6 · 验证期不绑定大型 UI 库**：探针只使用 Taro/微信基础组件。正式 UI 库等后续产品 Epic 结合包体、可访问性和设计需求决策；不把 beta 组件库变成路线前提。
- **DEC-7 · 离线与集成验证分道**：解析器、Cookie 策略和请求适配器先用确定性 fixture/mock 做本地验证；连接真实后端、账号、模型和微信 AppID 属于单独集成通道，只有 owner 明确指定目标后才执行，且不得使用生产凭据作为默认值。

## 子项契约

- **ITEM-1 · 建立隔离验证工程**：`cs-feat`；在 `miniapp/` 创建自带 workspace 边界与 lockfile 的 pnpm/Taro/React/TypeScript 工程、最小微信页面、API contract 本地依赖、被忽略的私密配置入口和验证脚本；依赖：无；验收：独立安装、类型检查、单测、微信构建通过，根 workspace/lock/turbo 与现有应用零改动，敏感配置不会进入版本控制。
- **ITEM-2 · 验证认证与普通 HTTP 契约**：`cs-feat`；实现最小请求与会话探针，先用 fixture/mock 验证客户端策略，再对 owner 指定的非生产目标执行认证矩阵并记录 Cookie 处理事实；依赖：ITEM-1；验收：两类证据边界清楚，敏感令牌不进入日志或普通持久化存储，无法满足时输出明确停止结论而非绕过安全边界。
- **ITEM-3 · 验证 NDJSON 流式聊天**：`cs-feat`；实现增量 UTF-8 与 JSON 文档解析、分块监听、中止和序号恢复探针；依赖：ITEM-1；验收：确定性分块 fixture 的自动化边界测试通过，owner 指定集成目标后由开发者工具证明真实后端分块，真机证据可用时补齐并与工具/mock 结果区分。
- **ITEM-4 · 验证文件与平台门槛并形成结论**：`cs-feat`；验证图片上传、鉴权下载、HTTPS/合法域名/AppID 条件，汇总兼容矩阵；依赖：ITEM-2、ITEM-3；验收：每项有证据与限制，给出唯一 go / conditional-go / no-go 结论和后续产品路线建议，不实现正式客户端。

## 最终交付索引

待执行后填写。

## 整体验收

- 四个子项均按契约给出可复核证据，未完成的真机或平台验证明确标为未完成。
- 隔离边界无越界改动，现有后端与 Web 无行为变化。
- 认证安全与真实流式传输两个核心结论均不是基于开发者工具的假成功或未验证假设。
- owner 根据最终 `go`、`conditional-go` 或 `no-go` 报告决定是否另立正式小程序产品 Epic；本 Epic 不代替该产品决策。

## 遗留风险

- 微信开发者工具不能完全代表 iOS/Android 真机网络、Cookie 和分块行为；缺少可用小程序 AppID、合法域名或真机时只能形成条件性结论。
- 微信平台审核、生成式 AI 合规、内容标识、支付和隐私要求不属于本次技术验证，正式产品立项前仍需单独确认。
- `file:` 依赖能隔离根工作区修改，但未来正式客户端若需要统一 CI、版本发布或跨包构建，可能需要另行批准一个纯追加的根级接入方案。
- 当前主机是 Node.js 20.18.1 / pnpm 10.17.0；Taro 4.x 声明 Node.js 18+，但正式客户端立项时应重新选择仍受维护的 Node LTS，并在 CI 中固定，不把本次探针环境自动升级为长期基线。
