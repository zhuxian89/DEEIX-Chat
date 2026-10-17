# Repository Instructions

## File reading — pick the tool for YOUR harness (read this first)

Reliable local file reading is non-negotiable. Never default to a tool that
isn't connected in your current harness, and the moment a read fails, switch
to the correct tool — do **not** retry the same failed call.

- **Claude Code harness (native):** use `Read`, `Glob`, `Grep`. Do **not** use
  `mcp__fastctx__*` (not connected here) and never `read_mcp_resource`.
- **Codex harness:** use `mcp__fastctx__read` / `mcp__fastctx__grep` /
  `mcp__fastctx__glob`; batch multiple files in one call. Do **not** use
  `cat`/`Get-Content`, `rg`/`findstr`, or `dir`/`ls -R`.

### `read_mcp_resource` is forbidden on fastctx (the recurring failure)

`fastctx` is a tools-only MCP server: it publishes zero `resources` and zero
`resourceTemplates`, so it does not implement the MCP `resources/read` method.
Any `read_mcp_resource(server:"fastctx")` returns `-32601: Method not found`.
This is exactly the loop that kept recurring — on the first failure, switch to
the correct tool above; never retry.

<!-- fastctx:begin -->
## Local file inspection

For reading, searching, and finding local files, prefer the FastCtx MCP
tools — `mcp__fastctx__read`, `mcp__fastctx__grep`, `mcp__fastctx__glob` —
over `cat`/`Get-Content`, `rg`/`findstr`/`Select-String`, and `dir`/`ls -R`.
Read only what the task needs. When you need several files, pass them to
one read call as files=[{"path": ...}, ...] instead of one call per file.
Pass absolute paths. The last line of every result says `Complete` or
`Partial` — continue only with the exact parameters a `Partial` note provides.

### Batch replacement

Use `mcp__fastctx__replace` for mechanical find-and-replace across files.
It preserves each file's encoding and line endings, supports dry-run previews,
and rejects concurrent changes before writing. Use apply_patch for generated
content, semantic rewrites, or small local edits.
### Never use read_mcp_resource on fastctx

`fastctx` is a tools-only MCP server — it registers zero `resources` and
zero `resourceTemplates`, so it does not implement the MCP `resources/read`
method. Any call to `read_mcp_resource` with `server: "fastctx"` returns
`-32601: Method not found` and must never be used. Always read files via
`mcp__fastctx__read` (single file via `file_path`, batch via `files=[...]`),
never via `read_mcp_resource`. Reserve `read_mcp_resource` for MCP servers
that actually publish resources.
<!-- fastctx:end -->

## Fork 上游同步约束（硬规则）

本仓库是上游项目的 fork，需要长期同步上游。实现任何新功能时，**默认只动新功能自己的新代码、新表、新文件、新包**，不要修改上游已有的代码与表结构，否则 merge 上游时冲突面会无法收拾。

- ✅ 允许：新建表、新建 domain/repo/service/transport 包、新建前端文件；在本仓库自有的新功能代码里随便改。
- ✅ 允许：新增配置项（settings namespace）、新增常量、新增流水类型等「纯追加」改动。
- ❌ 禁止：修改上游已有表的结构（加列/改列/改索引也算，例如 `identity_users`、`billing_*`、账号/订阅/会话表）。
- ❌ 禁止：为了新功能去改上游既有的删除/账号/认证/计费等框架代码与路由（如账号注销、管理员删除、登录注册主流程）。新功能的扩展点用新表或新函数吸收，不要改老逻辑。
- ⚠️ 若需求看起来必须改上游既有代码才能实现（例如「同邮箱重复注册只奖励一次」天然落在账号删除链路上），**先停下来报告**：说明会动哪些上游文件、冲突面多大，给出一个「只动新功能自己的新表」的替代方案让 owner 拍板，不得自行扩大改动。
- 教训：曾为了邀请奖励去改账号删除/管理员删除/相关路由（8 个文件 + 2 路由），导致无法干净同步上游；正确做法是把规则落在邀请关系新表（如 `invitee_email` 唯一索引）上，上游账号框架一行不动。

## 配置管理：禁止新增环境变量（硬规则）

owner 于 2026-10-17 明确要求：**本项目后续禁止新增任何环境变量。所有新增配置统一接入现有管理后台「参数配置」系统。**

- 禁止为新功能新增环境变量、读取新的环境变量名，或把环境变量作为新配置的覆盖值、兼容入口、回退值。
- 此规则覆盖源码、启动与部署脚本、Docker/Compose、CI、`.env` 示例和文档；不得通过这些位置新增环境变量或要求使用者设置环境变量。
- 需要可配置的业务参数，使用现有 settings namespace、参数校验、数据库持久化和运行时配置机制；管理端复用「参数配置」入口。
- 新参数在启动时自动补齐默认值，保留管理员已经保存的值；需要动态生效的参数通过现有运行时配置更新，禁止仅在启动时读取后永久缓存。
- 每次提交前检查本次差异，确认没有新增环境变量及相关配置说明；不得以临时方案、部署便利或兼容为由绕过本规则。

## 提交与小程序上传文案（硬规则）

owner 于 2026-09-12 再次确认，以下约束持续生效：

- 后续所有 Git commit 标题、正文，以及微信小程序每次上传填写的提交说明、版本描述和备注（包括 CLI `--desc`、上传脚本与开发者工具中填写的信息），应准确描述本次实际改动，不限于 TODO 功能。
- 上述文案不得包含 `AI`，不区分大小写（包括 `AI`、`Ai`、`ai`、`aI`）；不得出现“解锁 AI”“隐藏 AI”“切换 AI”等表述，也不得描述切换、解锁或进入其他界面的功能。
- 每次提交或上传前，必须检查最终实际使用的完整文案，包括脚本自动生成或拼接的内容；发现不符合约束的文案，先修正再执行。不能为了符合文案要求编造改动；若无法准确描述，先向 owner 说明。
- 本规则仅约束后续提交与上传文案，不要求删除源码或技术文档中的术语，不授权改写历史记录，也不构成 commit、push、上传或发布授权。

## 提交前代码 Review（硬规则）

owner 于 2026-10-17 明确要求：**每次写完代码，必须先完成 code review，之后才能提交。此规则没有例外。**

- 每次提交前审查本次完整差异，检查需求是否满足、权限与数据隔离、状态变化、并发与重试、兼容性、迁移和失败路径；测试通过不能代替代码 review。
- Review 发现问题时，先修复，再复查修改并重新运行受影响检查；不得带着已知未处理的问题提交。
- Review 之后又修改代码，必须审查新增差异后才能提交，不得用之前的 review 结论覆盖后续修改。
- 交付时简要说明 review 结果、发现并修复的问题以及未验证的限制。

## Workspace Quality 提交与交付检查（硬规则）

owner 于 2026-10-17 要求将 Workspace Quality 失败检查写入项目规则。**功能包测试通过不等于工作区质量检查通过，推送成功也不等于 CI 成功。**

- 修改代码后、提交前，必须按 `.github/workflows/frontend-quality.yml` 的实际入口，在仓库根目录完整运行 `pnpm check` 和 `pnpm test`。定向测试用于定位问题，不能替代完整工作区检查；使用项目指定的 pnpm 版本及对应工具链。
- 新增或移动后端文件时必须检查全局分层约束。`backend/internal/transport/http` 根包包含分层、协议边界等守护测试，不能只运行其业务子包；完整后端测试入口是 `backend` 目录中的 `go test ./...`。
- `domain` 类型不得带 `json`、`gorm`、`form` 标签；应用层公开类型不得带 HTTP/JSON 协议标签。配置 JSON 的编解码放在应用层私有解析结构中，公开值类型保持协议无关。不得通过删除守护测试、跳过测试或扩大白名单绕过失败。
- 修复 CI 时先读取失败提交的具体 job/step 日志，定位首个实际错误；修复后既复验失败项，也运行完整工作区检查。若受本机平台或工具限制无法完成，明确记录未验证项及原因，不得声称全部通过。
- 每次推送后核对远端提交 SHA，并跟踪该 SHA 对应的 Workspace Quality 到结束。`queued`、`in_progress`、`cancelled` 和 `skipped` 都不能报告为通过；失败时继续排查修复，CI 修复任务以该工作流 `success` 为交付条件。
- 对本次验证记录清楚提交 SHA、执行命令、结果和工作流链接；提交后再次改动代码，必须重新完成受影响检查，避免用旧结果证明新提交。

## CodeStable 执行门槛

1. 用户明确调用某个 skill 后，必须先完整读取该 skill 的 `SKILL.md`，并严格遵守其中的阶段、权限和交付规则。
2. skill 规定先讨论、先设计、先审核或先确认时，必须停在对应阶段，等待用户明确批准后才能进入下一阶段。
3. 在获得所需批准前，禁止修改代码、创建或删除文件、安装依赖、执行会改变项目状态的命令，以及提交或 push。
4. 对目标、范围、行为、数据结构、公开契约、权限、安全、并发、迁移或外部系统集成存在未决取舍时，必须先输出方案并列出验收标准，不得自行假设后实现。
5. 只读调查、读取文件和必要的诊断检查可以在方案阶段执行，但不得借此扩大实现范围。
6. 提交、发布、创建 release、发送外部消息或 push，必须等待用户对该动作的明确授权；“方案批准”不等同于“提交授权”。
7. 如果误越过 skill 门槛开始执行，必须立即停止，报告已产生的本地改动和状态，不得继续扩展或掩盖该改动。
