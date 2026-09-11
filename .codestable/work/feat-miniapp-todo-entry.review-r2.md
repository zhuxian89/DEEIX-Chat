# 首版 TODO 第二轮审查（归档）

本报告属于首版功能 d5422a1，不是后续反馈入口改造的审查结果。冻结 patch 是当时的本地材料，未随本记录提交；当前实现与状态见 [反馈入口记录](feat-miniapp-todo-feedback.md)。

审查目标：`.codestable/work/feat-miniapp-todo-entry.review-r2.patch`

新 SHA-256：`13dec74c6d361ed76da72687afed26f3ef7c13f267eabc6a7c150291d9c4bb0e`

上一轮正确 SHA-256：`a192ef891adeb46d862c37625c41c740ab569c19f2d0e44a92108844b4f74b2b`

哈希采用冻结任务包提供值；本轮环境未独立复算。审查覆盖完整 R2 候选及 manifest。

Overall：**可合**

### Resolved

1. **离线 overlay 与后端子步骤级联语义不一致：resolved**

   operation-specific marker 现在可以区分父操作产生的完成/删除状态；撤销只影响由对应父操作改变且之后未被独立修改的步骤。既有删除状态得到保留，无效状态重复操作也不再错误递增版本。

   这消除了上一轮两种确定性版本漂移：父任务完成后撤销再操作步骤，以及先删除步骤再删除/恢复父任务。新增回归测试覆盖了后续独立修改场景。

2. **导出跨多个 OFFSET 查询导致缺项或重复：resolved**

   导出现在解析身份一次，并通过 `Repository.ExportTasks` 的单条 SELECT 获取一致的语句级快照，不再依据可变 `updated_at` 做跨查询 OFFSET 分页。10000 行、8 MiB、owner isolation、过滤校验和 CSV 转义均有针对性测试。

3. **“今天/明天”受设备本地时区影响：resolved**

   快捷日期和页面日期筛选已统一到共享北京时间日历逻辑，并覆盖非本地时区及跨年。逾期判定也纳入了当天具体时间，前后端保存语义一致。

### Unresolved

无。

### New findings

- Blocking：无。
- Important：无。
- Nit：无。

相关补强——编辑器保持打开时捕获的版本、身份切换时重建 workspace、拒绝旧 store callback，以及忽略卸载后的异步搜索/导出/解锁结果——没有发现新的数据归属或并发回归。

任务包报告的后端测试、API 生成与校验、miniapp typecheck，以及完整 `pnpm check`（测试、微信构建和 dist 校验）均已通过。真实 PostgreSQL 与微信真机仍未验证，属于剩余验收证据缺口，不构成本轮代码 finding。未编辑任何文件，也未创建或委派代理。
