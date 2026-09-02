---
epic: ../epics/wechat-miniapp-production.md
phase: executing
approved_revision: 3cb65024764640f3cc998cb6cfb2410040d27be18ca47e21f97b764f1a427bda
current_item: ITEM-5
next_action: 部署后端并按 miniapp/ACCEPTANCE.md 完成正式客户端真机与微信平台验收
blocked_by: 正式后端尚未部署，微信后台外部配置与正式客户端真机验收待 owner 执行
item_progression: continuous
milestone_commit: manual
remote_publish: manual
---

## 子项进度

- [x] ITEM-1
- [x] ITEM-2
- [x] ITEM-3
- [x] ITEM-4
- [ ] ITEM-5

## 临时决策与证据

- 2026-09-02：现有技术验证确认 Taro 4.2.1 / React 18.3.1 / TypeScript strict / Webpack 5 可在独立 `miniapp/` workspace 构建微信小程序，并已在真机验证 Bearer 登录、内存 refresh Cookie 轮换、连续流式对话、独立生图与鉴权文件下载；来源：`../epics/wechat-miniapp-validation.md` 及其 work 证据。
- 2026-09-02：owner 确认当前只做小程序一键登录和正式核心产品，不在本期融合 Web/公众号身份；小程序首次登录不要求邮箱、密码或注册码，创建标准 DEEIX 用户。
- 2026-09-02：owner 确认仍需保存微信返回的 UnionID，为后续平台融合留档；本期只以 AppID/OpenID 登录，不使用 UnionID 匹配或合并用户。
- 2026-09-02：仓库现有微信公众号发码关系只保存公众号 OpenID 与注册码 ID，注册码消费后通过 `used_by_user_id` 关联标准用户；本 Epic 不修改该链路。
- 2026-09-02：全局禁止创建子 agent；此前技术验证 Epic 的 reviewer 豁免只记录于该 Epic，本 Epic 尚无独立 reviewer 权限或豁免，因此 proposed 文档完成后保持 planning，不越过 design review 门槛。
- 2026-09-02：owner 为本正式小程序 Epic 明确豁免独立 reviewer，要求主 agent 直接审查并连续实现；继续禁止所有子 agent，不提交、不推送。执行策略按该指令采用 `continuous / manual / manual`。
- 2026-09-02：owner 确认新建会话提供两个主要快捷入口“AI 对话”“AI 生图”和一个次级“更多模型”入口；普通用户不选择模型，快捷模型由服务端配置并固定到新会话，高级入口保留模型选择。
- 2026-09-02：主 agent 按 owner 豁免完成 design review。首轮发现一个 important：快捷入口虽要求服务端模型配置，但 ITEM-1 未明确交付给客户端的配置契约；已修订为登录响应返回默认对话/生图模型。复审确认该 finding resolved，原子身份创建、OpenID/UnionID 边界、删除后不重建、会话有界、计费复用、上游隔离和三入口行为均可验证，无 blocking / important / new findings。owner 已要求连续实现且不再逐项确认，契约激活。
- 2026-09-02：ITEM-1 完成。新增独立 `wechat_miniapp_bindings`、`code2Session` client、repository/application/transport、五项服务端配置、一键登录接口、标准 session/JWT/refresh 签发、旧小程序会话撤销和 Swagger/TypeScript contract。覆盖首次/重复/并发登录、UnionID 补全/冲突、删除后不重建、密码关闭凭据和配置校验；相关 Go 测试与 `pnpm api:check` 通过。
- 2026-09-02：ITEM-2 完成。`miniapp/` 已改为正式 Taro 客户端：冷启动 `wx.login`、内存 access/refresh、平台网络层、统一错误、标准用户/订阅/余额、最近会话和“AI 对话 / AI 生图 / 更多模型”入口；默认模型按权限独立可用，不互相阻塞且不静默换模。
- 2026-09-02：ITEM-3 完成。交付会话创建/进入、历史消息、多轮 NDJSON 流式对话、中文/Emoji 任意分块、停止、基础 Markdown、代码复制、图片选择/上传/发送和错误恢复；停止与真实失败具有不同 UI 状态。
- 2026-09-02：ITEM-4 完成。交付独立生图流、停止、状态、内联图片、鉴权文件下载、预览和相册保存；request/uploadFile/downloadFile 合法域名要求已写入部署文档。
- 2026-09-02：ITEM-5 本地部分完成。`pnpm check` 全绿（TypeScript、32 项测试、微信生产构建、隔离和产物安全扫描），产物约 361231 bytes；`go vet ./...`、Web TypeScript、API contract 和小程序/认证/计费/会话/注册码/公众号关键回归通过。全量 `go test ./...` 仅有既存 Windows 文件权限断言失败：`internal/infra/persistence/filecache` 得到 `0666`、测试期望 Unix `0644`，与本 Epic diff 无关。主 agent 按 owner 豁免完成 change review，blocking/important/nit 均为 0。部署配置与 `miniapp/ACCEPTANCE.md` 已生成；正式一键登录、对话、生图、上传下载和平台合规仍需部署后真机验收，因此 ITEM-5 保持未完成。
