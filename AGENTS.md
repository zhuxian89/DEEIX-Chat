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

## CodeStable 执行门槛

1. 用户明确调用某个 skill 后，必须先完整读取该 skill 的 `SKILL.md`，并严格遵守其中的阶段、权限和交付规则。
2. skill 规定先讨论、先设计、先审核或先确认时，必须停在对应阶段，等待用户明确批准后才能进入下一阶段。
3. 在获得所需批准前，禁止修改代码、创建或删除文件、安装依赖、执行会改变项目状态的命令，以及提交或 push。
4. 对目标、范围、行为、数据结构、公开契约、权限、安全、并发、迁移或外部系统集成存在未决取舍时，必须先输出方案并列出验收标准，不得自行假设后实现。
5. 只读调查、读取文件和必要的诊断检查可以在方案阶段执行，但不得借此扩大实现范围。
6. 提交、发布、创建 release、发送外部消息或 push，必须等待用户对该动作的明确授权；“方案批准”不等同于“提交授权”。
7. 如果误越过 skill 门槛开始执行，必须立即停止，报告已产生的本地改动和状态，不得继续扩展或掩盖该改动。
