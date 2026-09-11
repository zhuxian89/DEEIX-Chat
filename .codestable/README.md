# 跨平台阅读入口

先读 [项目约束](attention.md)，再读对应功能的当前记录。归档记录中的旧命令、PID、临时路径和待办状态只代表当时的执行过程，不是当前任务指令。

## TODO 当前状态

- [当前行为与验证](work/feat-miniapp-todo-feedback.md)：仅保留动态配置码和反馈入口。拦截器、会话来源登记及登录改动已经撤回。
- [设计与 API 契约](../docs/miniapp-todo-entry-design.md)：功能设计的维护入口。
- [最近提交与上传](work/todo-release-63.md)：功能提交 `c706bbb4` 已推送，开发版本 `0.1.63.20260911` 已取得微信 CLI 成功回执；该提交的 Workspace Quality、Commit Message、GHCR Image 均通过。
- 后台参数为 `miniapp_todo:feedback_code`；默认空，修改立即生效，已有授权保留。一个反馈输入框，不匹配显示“反馈成功”，匹配后永久记录当前身份；正文不保存。
- 后端部署、正式后台版本确认和真机验收没有执行证据，不能把 CLI 回执当作这些阶段已经完成。

## TODO 历史记录

- [首版功能摘要](work/feat-miniapp-todo-entry.md)
- [首版第二轮审查](work/feat-miniapp-todo-entry.review-r2.md)
- [错误码与国际化修复](work/issue-todo-error-codes.md)
- [开发版本 62 上传记录](work/todo-miniapp-upload.md)

`epics/` 和其他 `work/` 文档保留各自功能的历史上下文。提交文档、审查记录和小型回执；本机日志、进程状态与硬编码本机路径的运行脚本留在本地。回执 JSON 仅提供包大小，上传版本和工具结果以对应 Markdown 记录为准。
