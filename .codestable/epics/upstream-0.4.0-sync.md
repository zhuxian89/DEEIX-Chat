---
status: active
created: 2026-08-29
work: ../work/epic-upstream-0.4.0-sync.md
---
# 同步上游 0.4.0

## 起点

- fork 分支：`dev`，本地与 `origin/dev` 均为 `e416938ca314ee3fb8d9007109e9b70cd98911ca`。
- 上游仓库：`DEEIX-AI/DEEIX-Chat`，默认分支 `dev`；本次锁定目标为 `2e25037e5a17949898312e91df4805044e2f93a1`，版本 `0.4.0`。
- merge base：`026c87718576526fb111c947e240d7db3897ced7`。
- 分叉规模：fork 独有 15 个提交，上游独有 194 个提交；两侧共同修改 31 个文件。
- Git 虚拟合并结果：11 个真实冲突，20 个可自动合并但需要语义复核的重叠文件。
- 当前主工作区存在与本 Epic 无关的 `.mindfs` 会话和上传文件改动；这些文件必须保留，不得进入升级提交。
- 当前机器已核实：Go `1.26.5`、`CGO_ENABLED=1`、GCC `16.2.0`、Node `20.18.1`、pnpm `10.17.0`。

## 目标

将 fork 升级到锁定的上游 0.4.0 基线，完整获得知识库、内容审核、临时会话、消息分叉、模型与计费增强等上游能力，同时保留 fork 的注册码、邀请奖励和微信公众号功能。升级结果必须能构建、能迁移现有数据库、能继续注册和发码，并解决已确认的 iOS 16.1 Safari 首屏白屏问题。

## 范围

- 集成锁定的上游提交 `2e25037`，处理全部上游变更，而不是只升级 `streamdown`。
- 重点处理 31 个双方重叠文件，并对 11 个冲突逐项人工决议。
- 保留 fork 的注册码注册、OAuth 注册码校验、邀请关系与奖励、微信公众号回调和后台管理。
- 接受上游 0.4.0 自身新增的表、字段和迁移；fork 自有功能仍只使用自有新表，不再修改上游既有表结构。
- 重建 Swagger、TypeScript API contract 和 pnpm lockfile，不手工拼接生成产物。
- 对 Safari/iOS Safari 16.0+ 增加明确兼容处理，并以 iOS 16.1 实机结果作为验收证据。
- 在独立集成 worktree 和分支中执行，避免污染当前带 `.mindfs` 改动的主工作区。

## 非目标

- 不在本次升级中重新设计注册码、邀请奖励或微信公众号产品逻辑。
- 不顺手重构上游认证、账号删除、计费、知识库或路由框架。
- 不修改 `identity_users`、`billing_*` 等上游既有表来承载 fork 功能。
- 不追逐执行期间上游 `dev` 新增的提交；如上游前进，另行比较后再决定是否扩展目标。
- 不自动提交、不自动 push、不自动部署；方案批准也不等于这些动作获得授权。
- 不处理或清理当前工作区中的 `.mindfs` 文件。

## 验收标准

1. 合并历史明确包含 fork 当前头和锁定上游头；31 个重叠文件都有处理结论，仓库中无冲突标记。
2. 上游 0.4.0 的组合根、路由、知识库、内容审核、优雅关停、模型和计费逻辑完整保留。
3. 邮箱注册和首次 OAuth 注册都必须校验并原子消费注册码；无效或已用注册码不会创建残缺账号。
4. 邀请码生成、邀请关系唯一性和双方奖励仍在注册事务内工作；重复邮箱或重复关系不重复发奖。
5. 微信 `13004` 默认规则、可配置关键词/模板、OpenID 幂等发码、发放记录和通用日志仍可用；账号删除后，已消费注册码永久保持 `used`，对应 OpenID 再次请求时返回“账号已注销”提示，不重新发码。
6. 账号删除完整采用上游 0.4.0 的内置知识库文件引用保护和用户知识库清理；不为 fork 数据向 `DeleteAccountHard` 或既有删除路由追加处理步骤，注册码、邀请和微信审计继续由 fork 自有表保留既有事实。
7. fork 自有迁移只操作 fork 自有表；上游 0.4.0 对上游表的迁移原样保留。带 0.3.4 存量数据的生产形态 fixture 可连续执行两次升级迁移，数据与 seed/backfill 结果保持幂等。
8. `pnpm api:generate` 后 Swagger 三件套与 `types.generated.ts` 来自同一份合并后源码，`pnpm api:check` 通过。
9. Go 全量测试与 vet 通过；前端 lint、typecheck、Vitest 和生产构建通过；PostgreSQL 迁移/注册关键路径测试通过。
10. 最终浏览器产物不再包含已确认会导致 Safari 16.1 解析失败的两处 lookbehind 正则；iOS 16.1 实机可打开登录页、显示完整样式并进入注册流程。
11. 部署制品必须从通过 final acceptance review 的候选 SHA 构建；部署后 `/api/v1/version` 返回版本 `0.4.0` 和该 SHA，且注册码、微信回调、管理后台、知识库和一次普通对话通过冒烟验证。

## 关键决策

- **DEC-1 · 合并而非 rebase**：从 fork 当前 `dev` 建立独立集成分支，合并锁定的 `upstream/dev` 提交。15 个 fork 提交不逐个 rebase，避免重复解决同一批冲突，并保留 fork 与上游的真实历史关系。
- **DEC-2 · 锁定上游快照**：本次只接收 `2e25037`。执行开始和结束都记录上游最新头；目标之外的新提交只报告，不静默带入。
- **DEC-3 · 以语义合并为准**：既有上游文件以 0.4.0 逻辑为骨架，将 fork 扩展重新接入；禁止对高风险文件直接使用整文件 `ours` 或 `theirs`。
- **DEC-4 · 隔离当前工作区**：执行时创建 `sync/upstream-0.4.0` 分支和独立 worktree。当前 `dev` 与 `.mindfs` 改动保持不变，最终合入 `dev` 另等 owner 授权。
- **DEC-5 · 生成文件统一重建**：Swagger 三件套与 API contract 由合并后源码统一生成；Safari manifest/override 确定后才最终重建 lockfile。所有生成结果都不进行人工业务编辑。
- **DEC-6 · fork 数据边界不变**：上游可按 0.4.0 自身需要迁移上游模型；注册码、邀请、微信继续落在 fork 自有表，不能借升级向上游表加 fork 字段。
- **DEC-7 · Safari 修复分两层**：升级 `streamdown` 到上游的 `2.6.0`/`remend 1.3.1`；同时把 `mdast-util-gfm-autolink-literal` 锁定到不含 lookbehind 的 `2.0.0`。已核实 `2.0.0` 无 lookbehind、`2.0.1` 含 lookbehind。Browserslist 只声明 Safari/iOS Safari 16.0+ 目标，不把转译配置误当作正则语法修复。
- **DEC-8 · 版本控制权限**：推荐 `item_progression: per-item`、`milestone_commit: manual`、`remote_publish: manual`。每个可提交检查点停下报告；commit、合入 `dev` 和 push 分别等待 owner 明确授权。
- **DEC-9 · 候选 SHA 是验收与部署基线**：ITEM-6 全部通过后，先请求 owner 授权创建候选 commit，再冻结其 SHA 做 final acceptance review。任何审查修复都会使旧 SHA 失效，必须在 owner 授权新 commit 或 amend 后冻结新 SHA 并重新审查。通过后只允许将 `dev` 快进到该 SHA；若不能快进则停止并重新评估。push 与部署继续分别授权，部署只使用该已审 SHA。

## 31 个重叠文件处理表

### 真实冲突：人工合并源码或清单

| 文件 | 处理契约 |
| --- | --- |
| `backend/internal/app/app.go` | 以上游 0.4.0 组合根为骨架，保留内容审核、知识库、对象存储、抽取引擎、优雅关停等新装配；重新接入 registration-code、invitation、wechat repo/service/module，确认初始化顺序和依赖均非 nil。 |
| `backend/internal/application/settings/runtime_settings.go` | 保留上游 context window/Mistral OCR 配置及规范化；追加 fork 的 `invitation:*` 映射和邀请码长度下限，不能恢复上游已淘汰的 token 配置键。 |
| `backend/internal/infra/persistence/postgres/user/repository.go` | 保留上游内置知识库文件引用检查与用户知识库清理；保留 fork 邮箱/OAuth 注册码原子消费、邀请码生成、邀请关系和奖励事务。删除路径只采用上游实现，不加入 fork 清理步骤：已消费注册码保持 `used`，邀请/微信历史留在 fork 表。若合并时发现必须修改上游删除函数或路由，立即停下，由 owner 在“只使用 fork 新表”与“扩大上游冲突面”之间拍板。 |
| `backend/internal/transport/http/server.go` | 保留上游 Shutdown/readyz、Channel 公共路由、知识库和内容审核路由；重新加入微信公共回调、邀请用户路由及注册码/微信/邀请管理路由，检查 AdminOnly 边界。 |
| `frontend/features/admin/components/admin-sidebar.tsx` | 保留上游 superadmin 对内容审核菜单的可见性限制及知识库入口；加入注册码和微信公众号入口，不扩大其既有管理员权限。 |
| `frontend/package.json` | 接收上游 0.4.0 版本、资产同步脚本、Next/Streamdown/PDF/Tailwind 更新；保留 fork 的 Vitest、jsdom、Testing Library 和 `test` 脚本。 |

### 真实冲突：禁止手工拼接的生成产物

| 文件 | 处理契约 |
| --- | --- |
| `backend/docs/docs.go` | 由 `pnpm api:generate` 从合并后 handler 注释重建。 |
| `backend/docs/swagger.json` | 同上，并作为 API contract 的规范输入。 |
| `backend/docs/swagger.yaml` | 同上，必须与 JSON/Go 文档版本一致。 |
| `packages/api-contract/src/types.generated.ts` | 由合并后的 Swagger 自动生成，不手改接口。 |
| `pnpm-lock.yaml` | 在最终 package manifests 和 override 确定后用 pnpm 10.17.0 重建，再用 frozen lockfile 安装验证。 |

### Git 可自动合并：必须进行语义复核

| 文件 | 处理契约 |
| --- | --- |
| `.gitignore` | 合并上游新增缓存/资产忽略项与 fork 微信草稿忽略项；不得扩大到忽略源码或迁移文件。 |
| `README.md` | 保留上游 0.4.0 部署说明和微信 Token 配置；把写死的 `13004` 描述更新为“默认规则、后台可配置”，避免文档与功能不符。 |
| `backend/README.md` | 同步上游后端配置与 fork 微信回调说明，保留明文模式和独立表边界，修正固定关键词措辞。 |
| `backend/internal/application/auth/provider.go` | 同时保留上游 provider logo URL 安全校验与 fork OAuth 注册码传递/消费入口。 |
| `backend/internal/application/auth/provider_test.go` | 合并上游安全测试和 fork repo mock 新接口；覆盖 OAuth bridge 开启与关闭两种首次注册路径。两条路径都断言有效注册码只消费一次，无效或并发消费不会留下 user、credential、identity 残行。 |
| `backend/internal/application/auth/service.go` | 接受上游 port interface、GeoResolver typed-nil 修复和结构化删除错误；保留 registration/invitation repo 扩展接口。 |
| `backend/internal/application/settings/seed.go` | 接受上游 context/OCR/EPay seed 变化和 obsolete keys；追加 `invitation` namespace，不覆盖上游默认值。 |
| `backend/internal/infra/config/config.go` | 接受上游关停、上下文窗口、Mistral OCR、模型选项等配置；保留 `WECHAT_CALLBACK_TOKEN` 和 invitation runtime 字段及默认值。 |
| `backend/internal/infra/persistence/postgres/billing/repository.go` | 接受上游并发安全扣费、兑换记录、活动热力数据；保留 `ApplyInvitationReward`，验证流水类型与增量余额不会被覆盖。 |
| `backend/internal/infra/persistence/schema/schema.go` | 合并全部上游新模型与 fork 新模型；保留 embedding 失效处理、上下文/usage backfill、微信默认规则 seed、邀请码 backfill，并明确幂等执行顺序。 |
| `backend/internal/shared/response/error_code.go` | 合并上游错误码调整并保留 `auth.registration_code_invalid`；清除仅格式变化，检查前后端错误 key 一致。 |
| `backend/internal/transport/http/auth/handler.go` | 保留 fork 邮箱/OAuth 注册码和邀请码参数；接受上游账号删除知识库 409 与结构化验证错误处理。 |
| `frontend/features/admin/model/admin-sections.ts` | 合并上游内容审核/知识库与 fork 注册码/微信 section，确保联合类型和排序稳定。 |
| `frontend/features/auth/hooks/use-auth-login-page.ts` | 保留 URL 邀请码预填、注册码状态和 OAuth bridge 存储；接受上游 Promise catch 类型修复，覆盖直接注册链接。 |
| `frontend/i18n/messages/en-US/admin-users.json` | 合并上游 content moderation/knowledge base 文案与 fork registration/wechat 文案。 |
| `frontend/i18n/messages/en-US/errors.json` | 合并上游错误文案并保留 registration code 错误 key。 |
| `frontend/i18n/messages/en-US/settings.json` | 合并上游 activity/thinking/tool 设置文案与 fork invitation 页面文案。 |
| `frontend/i18n/messages/zh-CN/admin-users.json` | 同步全部菜单文案；避免把既有中文“计费”意外退回英文 `Billing`。 |
| `frontend/i18n/messages/zh-CN/errors.json` | 合并上游错误文案并保留注册码中文错误。 |
| `frontend/i18n/messages/zh-CN/settings.json` | 合并上游设置文案与 fork 邀请页面中文文案。 |

## 子项契约

- **ITEM-1 · 冻结并建立集成基线**：`cs-refactor`；创建隔离 worktree/`sync/upstream-0.4.0`，配置只读上游来源，确认 fork、upstream、merge base 与 31 文件清单未漂移；依赖：无；验收：当前 `dev` 和 `.mindfs` 不变，目标仍为 `2e25037`，虚拟合并结果可复现。
- **ITEM-2 · 解决六个源码/清单冲突**：`cs-feat`；按处理表完成 `app.go`、runtime settings、user repository、server、sidebar、frontend manifest 的语义合并；依赖：ITEM-1；验收：上游与 fork 两侧行为均有明确落点，不使用整文件选边，关键 Go/TS 文件可格式化和编译；删除路径未增加 fork 步骤。
- **ITEM-3 · 复核二十个自动合并文件**：`cs-review` + 对发现问题使用对应 `cs-issue`；逐文件检查认证、配置、迁移、计费、前端状态和翻译；依赖：ITEM-2；验收：20 个文件逐项签收，重点注册/邀请/删除事务有测试，文档不再写死可配置关键词。
- **ITEM-4 · 重建四个 API 生成文件**：`cs-refactor`；运行版本同步和 `pnpm api:generate`，统一重建 Swagger 三件套与 `types.generated.ts`；依赖：ITEM-3；验收：四个文件无冲突标记、无手工业务修改，`pnpm api:check` 通过。
- **ITEM-5 · 修复 iOS 16.1 首屏兼容并重建 lockfile**：`cs-issue`；采用 Streamdown 2.6.0/remend 1.3.1，在根 manifest 对 mdast autolink 依赖施加 2.0.0 override，声明 Safari/iOS Safari 16.0+ 目标，最后用 pnpm 10.17.0 重建 `pnpm-lock.yaml`；依赖：ITEM-4；验收：frozen-lockfile 安装通过，依赖树版本正确，构建产物静态扫描无已知 lookbehind，iOS 16.1 实机登录/注册页不白屏且样式完整。
- **ITEM-6 · 完整回归和生产形态迁移验证**：运行权威验证；依赖：ITEM-5；验收：Go test/vet、前端 lint/typecheck/test/build、API contract check 和关键注册集成测试全部通过。另从 `e416938` 的 0.3.4 schema 建立 PostgreSQL fixture，至少包含 active/used 注册码、微信发放记录、邀请关系与奖励、用户/会话/计费/文件数据；连续执行两次 0.4.0 启动迁移，断言业务行数、注册码 `used` 状态、OpenID 关系、邀请唯一约束、余额流水、seed/backfill 结果均未重复或丢失。失败必须修复或由 owner 明确接受，不以“Git 已合并”代替质量结论。
- **ITEM-7 · 冻结部署候选并最终验收**：依赖：ITEM-6；先输出全部验证证据并请求 owner 授权候选 commit，冻结 commit SHA 后对该 SHA 执行独立 `cs-review` final acceptance。审查修复须形成 owner 授权的新候选 SHA并重新审查。通过后交付版本/迁移/环境变量清单、数据库备份命令和经过演练的恢复步骤；备份与恢复记录是部署前硬验收。随后分别请求 owner 授权把 `dev` 快进到已审 SHA、push 和部署；线上 `/api/v1/version` 与核心冒烟检查通过后由 owner 最终接受。

## 最终交付索引

- 集成分支与最终 merge commit：待执行。
- 31 文件处理证据：待执行，记录在 work 游标。
- 生成命令与测试结果：待执行，记录在 work 游标。
- iOS 16.1 实机证据：待执行，记录在 work 游标。
- 部署版本与线上冒烟结果：待执行，记录在 work 游标。

## 整体验收

ITEM-6 完成且 owner 授权候选 commit 后，以固定候选 SHA 和本文件的 11 条验收标准执行独立 final acceptance review。审查通过只证明该 SHA 可作为部署候选，不自动授权快进 `dev`、push 或部署；Agent 只报告证据和遗留风险，不代替 owner 接受升级结果。

## 遗留风险

- Next.js 16.3.0 对旧 Safari 的官方支持边界可能高于 iOS 16.1；两处已知 lookbehind 修复后仍必须实机检查其他 chunk，新语法问题不能仅靠 Browserslist 推断已解决。
- `user/repository.go` 同时承载上游账号删除和 fork 注册事务，是本次最高风险文件；缺少 PostgreSQL 真实事务测试时不得进入部署候选。
- schema 启动迁移同时包含上游 embedding 标记失效、知识库模型和 fork seed/backfill；即使生产形态 fixture 通过，生产部署前仍必须完成数据库备份并演练可执行恢复步骤。
- 上游 `dev` 在执行期间可能继续前进；本 Epic 的成功只代表已同步锁定提交 `2e25037`。
