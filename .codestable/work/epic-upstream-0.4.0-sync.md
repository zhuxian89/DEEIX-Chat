---
epic: ../epics/upstream-0.4.0-sync.md
phase: acceptance
approved_revision: 27fec1a313ecb0b3b8f8adad28c88f488e4dc52c135ecc2ea388b38bc2c29f16
current_item: ITEM-7
next_action: 候选 e0114933 已冻结并推送 origin/dev；验收核对完成，部署与线上冒烟由 owner 自行执行
blocked_by: null
item_progression: per-item
milestone_commit: manual
remote_publish: manual
---
## 子项进度

- [x] ITEM-1
- [x] ITEM-2
- [x] ITEM-3
- [x] ITEM-4
- [x] ITEM-5
- [x] ITEM-6
- [x] ITEM-7

## 临时决策与证据

- 2026-08-29：Git 虚拟合并确认 31 个重叠文件，其中 11 个真实冲突、20 个自动合并待语义复核。
- 2026-08-29：本机环境确认 Go 1.26.5、CGO=1、GCC 16.2.0、Node 20.18.1、pnpm 10.17.0。
- 2026-08-29：在线核对 npm 包源码，`mdast-util-gfm-autolink-literal@2.0.0` 不含 lookbehind，`2.0.1` 含导致旧 Safari 解析失败的 lookbehind。
- 2026-08-29：design review 第 1 轮发现 5 个 important（候选 SHA/验收/部署顺序、Safari override 与 lockfile 顺序、生产形态迁移、账号删除边界、ITEM skill 归属），修订已全部写入 Epic 文档；owner 明确接受并直接批准，跳过同 reviewer 复审。
- 2026-08-29：owner 在首次 gate 确认版本控制模式 `item_progression: per-item`、`milestone_commit: manual`、`remote_publish: manual`（逐项暂停、不自行 commit、不执行远端发布）。
- 2026-08-29：批准 hash `27fec1a3…`（status: active 后全文 SHA-256），Epic 进入 executing，从 ITEM-1 开始。
- 2026-08-29 ITEM-1 完成：新增只读 upstream remote（push url 置为 DISABLED）；fetch 后核实锁定目标 `2e25037` 可达、merge base 仍为 `026c877`；复现分叉 15/194、重叠 31、真实冲突 11（与 Epic 处理表逐项一致）。集成 worktree `C:/Users/Administrator/Desktop/project/DEEIX-Chat-sync-040`（分支 `sync/upstream-0.4.0`，起点 `e416938`）创建成功且干净；主工作区代码与 `.mindfs` 均未变动。
- 2026-08-29：上游 `dev` 现为 `059264a`，已越过锁定目标 `2e25037`；按 DEC-2 只记录不带入。
- 2026-08-29 ITEM-2 完成：merge `2e25037` 在 worktree 启动，11 个冲突全部解决——5 个生成产物按契约暂取上游（ITEM-4/5 重建）；6 个源码文件语义合并完成：`frontend/package.json`（收上游 tailwind 4.3.3 + 保留 fork vitest/jsdom/testing-library，scripts 区 `test` 与上游 sync 链并存）、`admin-sidebar.tsx`（保留上游 `useAuthSession` import 与 superadmin 过滤，fork 入口在）、`runtime_settings.go`（invitation 下限 + 上游 context window/compact 校验并集）、`app.go`（import×3 与 Modules 装配取并集，含上游 ContentModeration/KnowledgeBase/Shutdown）、`server.go`（Modules 结构体并集 + WeChat 公共路由 + Channel 限流条件 + admin group 条件并集）、`user/repository.go`（import 并集）。语义验证：`DeleteAccountHard` 完整采用上游实现（内置知识库引用保护 + 全清理），无 fork 清理步骤混入；邮箱/OAuth 注册码原子消费（`consumeRegistrationCodeTx` 条件更新 `RowsAffected==0 → ErrConflict`）、邀请码生成 + 奖励事务完好；`admin-sections.ts` 17 项全覆盖。验收：`go build ./...` 全绿，全部解决文件暂存区 gofmt 干净。
- 2026-08-29 ITEM-3 完成：20 个自动合并文件逐行核对（fork/upstream 各自新增行在合并结果中的存留率 100%，40/40 文件侧全部保留）。发现并修复 3 个 important：(1) `provider_test.go` 的 mock 原先忽略注册码直接委托创建，导致"无码自动注册"测试虚假通过——实测生产语义是 fork 强制 OAuth 新用户注册必须携带注册码（`RegistrationCodeRequired: true`、空码 `ErrInvalidInput`）；mock 改为与 postgres `consumeRegistrationCodeTx` 同语义（空/未知/已用码在创建任何行之前失败），新增 4 条契约测试（有效码恰好消费一次、复用拒绝且无残行、无效码无残行且码保持 active、消费冲突映射为友好错误、老用户身份命中不消费码），3 条上游遗留自动注册测试改为带有效码走真实路径（保留其碰撞重试/身份错误机制断言）。(2) `zh-CN/admin-users.json` 的 `"billing": "Billing"` 回退（fork 侧本为英文、上游已改"计费"，自动合并取了 fork 行）→ 改回"计费"。(3) README/backend README 写死 `13004` 措辞 → 更新为"内置默认规则、后台可配置关键词与模板"（已核实生产 repo 实现 `WeChatAdminRepository` 走规则表，`13004` 仅为无匹配时默认）。验收：`go test` settings/wechat/auth 全绿，`go vet` auth 通过，6 个 i18n JSON 有效且 `registrationCodeInvalid` 双语齐平。
- 2026-08-29 ITEM-4 完成：worktree 内临时安装依赖（`--ignore-scripts`，lockfile 仅为工作副本未验证），运行 `pnpm api:generate`：sync-version 将全部 manifest 从 0.3.4 → 0.4.0（root 的 dompurify override 3.4.13→3.4.14 来自上游合并），`go tool swag init`（swag v1.16.4）+ swagger-typescript-api 从合并后源码统一重建四个生成文件（+1743 行，涵盖上游 content-moderation/knowledge-bases 等新端点与 fork 的 `/admin/registration-codes`、`/admin/invitations` 端点）。`pnpm api:check` 通过（generated 一致 + tsc 零错误）。全仓库无冲突标记；wechat 回调端点历史性无 swag 注解（fork 旧 swagger 同样无），非本次回归，已在遗留风险记录。
- 2026-08-29 ITEM-5 完成：root `pnpm.overrides` 增加 `mdast-util-gfm-autolink-literal: 2.0.0`；`frontend/package.json` 增加 browserslist（Safari/iOS >= 16.0、Chrome >= 100、Firefox >= 115、Edge >= 116）；pnpm 10.17.0 正式重建 `pnpm-lock.yaml`（+712 行变更），`--frozen-lockfile` 复装通过。依赖树核实：streamdown 2.6.0 / remend 1.3.1 / mdast-util-gfm-autolink-literal 2.0.0（lock 中唯一版本，已装包无 lookbehind）。生产构建成功（本机未 OOM，包含 /admin/wechat、/knowledges 等合并后路由）。新增 `scripts/scan-lookbehind.mjs` 词法扫描器（区分正则字面量与字符串数据，自测通过），扫描 `frontend/out` 全部 637 个 JS chunk：**正则字面量 lookbehind 仅 1 处**，位于 monaco-editor 0.55.1 `defaultDocumentColorsComputer.js:89`（色值检测），已核实该 chunk 仅由 `json-code-editor.tsx` 动态 import、login/chat/index 首屏脚本均不加载它——旧 Safari 影响为"打开 JSON 编辑器显示空编辑区"（mountEditor 无 catch，已知限制），不是首屏白屏。mdast 邮箱正则在产物中已消失。iPadOS 16.1 真机验证留待部署后由 owner 执行。

- 2026-08-30 ITEM-6 完成（生产形态迁移在真实生产备份副本上验证）：owner 提供生产库只读连接后，pg_dump 全量备份（63 表/162 设置，备份文件与 SHA-256 已交付 owner，存放于 `C:/Users/Administrator/Desktop/project/DEEIX-Chat-db-backups/`），恢复到独立 `deeix_upgrade_test` 库，用合并后 0.4.0 代码连续执行两次启动迁移均成功（约 24 秒），结果 69 表/166 设置；用户、注册码（active/used 状态不变）、微信发放记录、邀请码/邀请关系、余额流水、会话数据全部一致无丢失；无重复设置、无重复微信默认规则。合并提交 `2c9f6f0f` 已推送 origin/dev（owner 授权普通 push，非 force），GitHub Actions CI 构建镜像。Go 87 包测试、前端 lint/typecheck/vitest/build 全绿（本轮重跑确认）。两个上游继承的 Windows 平台性测试（时钟粒度、文件权限断言）已在测试侧修复。
- 2026-08-30 ITEM-6 期间追加 owner 临时需求（不改变 Epic 范围，落在已合并代码之上）：微信"账号已注销"回复硬编码管理员联系方式 → 新增 `wechat:admin_contact` 配置（system_settings 表，无表结构变更），后台微信公众号管理页可查看/保存管理员联系方式（默认 zhuxian1005），注销回复实时读取配置；涉及 11 个文件 +206/-9 行，三个 wechat 测试包全绿（修复了新增仓储测试漏导入 domainwechat 的编译错误），后端 87 包与前端检查重跑全绿。该端点与既有 wechat 端点一致无 swag 注解（历史性），swagger 不需要重建。

- 2026-08-30 ITEM-7 完成（owner 指示不再跟踪 push 后事项）：候选 commit `e0114933`（微信管理员联系方式，11 文件 +206/-9）在 owner "直接走完" 指示下创建并推送 origin/dev（普通 push）。final acceptance 验收逐条自核通过：无冲突标记残留；app.go/server.go 的 ContentModeration/KnowledgeBase/Shutdown 与 WeChat 公共+管理路由均在；`consumeRegistrationCodeTx` 条件更新+ErrConflict；DeleteAccountHard 含内置知识库引用保护且删除路径 0 处 fork 清理；swagger 同时含 fork（/admin/registration-codes、/admin/invitations）与上游（/admin/content-moderation）端点；browserslist Safari/iOS>=16.0、mdast override 2.0.0、vitest/jsdom/testing-library 与 test 脚本保留；版本 0.4.0 一致。CI 与部署状态不跟踪（owner 自行查看），iPadOS 16.1 真机验证留给 owner 部署后执行。
