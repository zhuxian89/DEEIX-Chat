# Project Attention
项目级事实与每次会话需要优先注意的约束。

## 必须遵守

1. 新增或移动 `/admin/*` 页面前，先核对当前仓库中 `admin/layout.tsx` 所在的 route group；页面必须放在同一棵 `admin` 路由树下才能继承 `AdminShell` 和左侧菜单。不要只看最终 URL：不同 route group 可以生成相同 URL，但不会共享 layout。当前 canonical 位置是 `frontend/app/(project)/admin/`。
2. 任何 commit 或 push 都必须在执行前取得用户当次明确同意；实现、测试、方案批准或历史上的提交授权均不自动延续。未获同意时只保留本地改动并报告状态。
3. 部署与 CI：新增或修改后端 HTTP 接口、DTO 或 Swagger 注解后，提交前必须运行 `pnpm api:generate`，纳入 `backend/docs/{docs.go,swagger.json,swagger.yaml}` 与 `packages/api-contract/src/types.generated.ts`，并以 `pnpm api:check` 通过为准。Docker Hub 出现 `Username and password required` 时优先核对仓库 Secrets `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN`；该凭证错误与 GHCR 是否成功相互独立，不要误改业务代码。
4. 微信小程序迁移铁律：实现或修复 `miniapp/` 功能前，必须先定位并完整对照成熟 Web 前端的对应页面、hook、API 客户端、状态模型与测试；请求体、响应、事件、任务 ID、流式续传、历史恢复及计费关联等业务契约必须原样复用或等价移植。小程序只实现微信平台不可复用的适配层（如 `wx` API、生命周期、组件和布局），禁止脱离 Web 另造业务流程、状态机或通用抽象。若平台限制导致无法等价，必须先报告具体差异与验收标准，等待 owner 拍板后再实现。验证优先复用或移植 Web 的自动化测试与契约检查，不得把本可自动证明的回归反复交给 owner 真机重验。
5. 微信小程序上传铁律：始终把 `miniapp/` 作为微信 CLI 的 `--project`，由其 `project.config.json` 中的 `miniprogramRoot: "dist/"` 定位产物，禁止把 `miniapp/dist/` 自身当作工程根上传。每次上传前依次清理微信开发者工具的 `compile` 与 `file` 缓存、执行 `pnpm build:weapp` 和 `pnpm check:dist`，并确认 `dist` 只包含预期构建版本后再上传；上传命令中的版本号只是服务端标签，不能证明代码包新鲜。若扫码页显示新版本、首页却显示旧版本，优先判定为开发者工具上传了旧工程/编译缓存，不得反复上传或让 owner 重复清手机缓存。本规则源于 2026-09-03 的实证：同一 `0.1.16` 标签曾先后运行 `0.1.13`、`0.1.9`，完成双缓存清理、强制构建、产物检查并从 `miniapp/` 根工程上传后，真机才运行 `0.1.16`。
6. 微信正式小程序唯一 AppID 是 `wx59fcdf6143e32cef`；此前测试号已永久废弃。`miniapp/project.config.json`、模板、配置脚本、隔离检查与上传前产物检查必须共同锁定该正式 AppID；任何非正式 AppID 都必须在构建/上传前失败。判断上传成功必须同时满足微信 CLI 成功且正式小程序后台出现对应开发版本，不能只依据本地 CLI 的 `√ upload`。
