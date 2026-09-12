# 小程序会话访问门禁

本功能将入口码升级为服务端访问权限，取代 `miniapp-todo-entry-design.md` 中“仅控制页面分流、不检查 AI 权限”的旧边界。

## 会话来源

微信 code 验证后，签发的小程序 sessionID 在返回凭证前写入 `miniapp_access_sessions`，登记 userID、AppID、OpenID。登记失败不返回凭证。后续基于验签后的 sessionID 判断来源，不使用客户端 Header、User-Agent、Referer 或账号是否绑定过微信。刷新保持 sessionID，因此不会改变来源。

普通 Web 登录流程和 JWT 格式不变。Web 请求增加一次来源表主键查询，但不检查微信解锁状态，不改变原有模型权限、计费、资源归属检查。来源数据库不可用时返回 503，不将查询失败当成 Web。

小程序解锁继续读取 `miniapp_entry_unlocks`，精确匹配已登记 AppID/OpenID 和有效微信绑定。已有永久解锁保留。新一次小程序登录仅使相同身份已登记的小程序会话失效，不依据可变 User-Agent 撤销 Web 会话。

## 访问规则

门禁在 Gin 注册任何路由前挂载，覆盖上游与 fork 后续注册的路由。它是附加限制，不替代现有 JWT、会话有效性及资源归属鉴权。

未解锁的小程序会话仅允许以下方法与路径：

- GET `/api/v1/miniapp-entry/status`
- GET `/api/v1/miniapp-todo/snapshot`
- GET `/api/v1/miniapp-todo/tasks`
- GET `/api/v1/miniapp-todo/export`
- POST `/api/v1/miniapp-todo/sync`
- POST `/api/v1/miniapp-todo/feedback`
- POST `/api/v1/auth/refresh`（刷新凭证仍由原认证服务验证）

其他携带该会话的 API 请求返回 403 `miniapp.access_locked`。未知新 API 默认拒绝，不使用宽泛 TODO 前缀放行。公共接口仍可匿名访问；本门禁不把公开分享或公开元数据改成私有资源。合法 Web 凭证在任何客户端使用时均按 Web 会话处理。

## 首次启用及旧会话

用户已确认来源不明的旧会话强制重新登录，其中可能包括 Web 会话。

首次初始化以事务创建自有来源表和 `miniapp_access_migrations` 标记表。仅成功的 `login`、`provider_login`、`two_factor_verify`、`email_register` 事件携带的 sessionID 与 userID 同时匹配且无小程序来源冲突时，保留对应有效 Web 会话。其他有效旧会话（含已确认小程序会话）登记为需要重新登录。日志不存在、JSON 损坏、仅存在刷新/活动事件均不作为 Web 证据。

旧会话访问和刷新均返回 401 `miniapp.reauthentication_required`，不修改旧会话表结构或原用户数据。初始化标记持久化，重启和日志清理不会重新放行旧会话，也不会重复强制新 Web 会话登录。小程序重新登录后永久解锁从原表恢复。

**首次部署须停止所有旧版本后端实例，再启动新版本。禁止新旧版本混跑：旧实例不登记新签发的小程序会话，也不执行门禁。** 初始化必须成功后才能接流量。回滚旧二进制将失去门禁保护；保留新表不能使旧代码执行门禁，回滚后再次升级前必须重新审计回滚期间签发的会话。不要删除迁移标记或来源记录作为常规恢复操作。

## 验证与边界

测试覆盖旧来源切换、损坏日志、跨 userID/AppID 隔离、重启、日志清理、解锁实时读取、新登录替换旧小程序会话、注册失败不泄露凭证、数据库故障拒绝、真实 Gin 全局挂载以及 Token 更换 JTI 后仍按 sessionID 判断。

SQLite 自动化验证不能代替 PostgreSQL 生产迁移演练。多实例升级须遵守停旧实例的部署顺序。更改配置码只影响新增授权，既有永久解锁不自动撤销。
