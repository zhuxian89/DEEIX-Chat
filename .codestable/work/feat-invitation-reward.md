---
slug: invitation-reward
type: feat
status: reviewed-ready-to-commit
owner: zhuxian89
created: 2026-08-09
updated: 2026-08-10
rev: v7（独立 change review 通过，等待 owner commit 授权）
---

# feat: 用户邀请码 + 邀请注册奖励额度 (+ 注册码 REG- 格式迁移)

> **状态**:design v5（纳入 owner 全部反馈），等待 owner review。**owner 批准前不实施任何代码改动。**
>
> **范围收缩**:owner 撤销「微信 13004 生成邀请码」，邀请码仅在网页（注册页 / 用户中心）流转，不接入微信链路。

## owner 反馈处置表（v1 → v3）

| # | owner 反馈 | 处置 |
|---|---|---|
| 1 | 无效邀请码宽松放行 | ✅ 改为宽松：无效码忽略、正常注册、不发奖、不报错 |
| 2 | 用户可看邀请链接、已邀请用户、奖励 | ✅ 用户中心加「邀请面板」：邀请链接 + 邀请码 + 已邀请列表 + 奖励明细 |
| 3 | **不改用户表**（fork 项目要同步上游，老表不动） | ✅ **改方案**：邀请码改存独立表，不新增 `identity_users` 列 |
| 4 | 线上是 period 模式，余额奖励还能发吗 | ✅ **已用代码证实可发**：period 模式 BillingAccount 余额是周期额度用尽后的超额备用额度，奖励有效。载体不变 |
| 5 | 默认奖励 0.5 | ✅ `invitee/inviter_reward_credit_usd` 默认各 0.5 USD |
| 6 | 邀请码加固定前缀，区分注册码 | ✅ 邀请码加前缀 `INV-`（如 `INV-AB3KX9M2`） |
| 7 | 注册时显示邀请码、不可修改、从链接提取 | ✅ 注册页 disabled 输入框，从 URL `?invite=` 提取，用户可见不可改 |
| 8 | 邀请码表软删除 | ✅ `invitation_codes` 用 `BaseModel`（软删除） |
| 9 | 启用独立 review | ✅ 实现完成后跑独立 change review（异构 reviewer） |
| 10 | 注册码也加 `REG-` 前缀（全部迁移） | ✅ 新生成码改 `REG-` 格式。**确认注册码是昨天刚加、未上线的本地功能**（commit 均 8/9），无存量用户码、无上游 diff → **干净迁移**，无需双格式兼容、无旧码失效风险、无需存量数据迁移。只改 `generateCode` + 测试断言，不改表结构/消费逻辑 |
| 11 | ~~微信发 `13004` 返回专属邀请码~~ | ⏸ **owner 撤销**。邀请码仅在网页流转，不接入微信链路。简化范围、避免 wechat 改动。未来需要可另起 |


## 目标

1. 每个用户拥有一个**固定、可分享的邀请码**(invitation code),带前缀 `INV-`,注册时生成、此后不变。
2. 新用户**通过邀请码注册**时,系统向**被邀请人**和**邀请人**各发放**一次性按量余额奖励**,金额由后台配置决定(默认各 0.5 USD)。
3. 邀请码与现有「注册码(registration code,管理员发放的一次性准入码)」**正交、不互斥**——两套机制独立运行。
4. 奖励金额、开关均可在后台配置,无需改代码即可调整。
5. 用户可在「用户中心」看到自己的**邀请链接、邀请码、已邀请用户列表、获得的奖励明细**。

### 非目标(本期不做)

- **防刷/防滥用**:owner 明确「先不做,注册即发」。不做真实活跃判定、IP/设备频次限制、人机校验加码。→ 见「未决」中的已知风险标注。
- 邀请人**持续分成**:owner 未选,不做。
- **微信 OAuth 注册路径**接入邀请码:owner 未要求,本期仅做邮箱注册路径,OAuth 路径列为边界(见影响面)。
- 被邀请人**事后补填**邀请码:邀请码仅在注册时消费,注册后不可补。
- 邀请统计/排行榜:仅做「我的邀请码 + 已邀请列表 + 奖励明细」,不做全局排行。

## 术语(canonical,本期引入新词,边界需对齐)

| 术语 | 定义 | 与现有词的边界 |
|---|---|---|
| **邀请码**(invitation code) | 每个用户固定的、可分享的注册凭证,用于标识邀请人 | ≠ **注册码**(registration code):管理员发放、一次性、仅作准入控制、不发额度 |
| **邀请人**(inviter) | 分享邀请码的已有用户 | — |
| **被邀请人**(invitee) | 通过某邀请码注册的新用户 | — |
| **邀请关系**(invitation relationship) | 一次邀请事件的持久记录(inviter → invitee) | 一对一,一个被邀请人只能建立一次 |
| **邀请奖励**(invitation reward) | 注册成功后向双方发放的一次性按量余额 | 载体是 `BillingAccount.BalanceNanousd`,与 free 计划的隐式月度额度 `PeriodCreditNanousd` 是两套独立余额 |

## 现场(已核实的仓库事实 + 来源)

### 注册与注册码

- 邮箱注册入口 `RegisterWithEmailAndRegistrationCode` 在 [registration.go:158](../../backend/internal/application/auth/registration.go),内部默认传 `subscriptionPlanID=0, subscriptionPriceID=0`([registration.go:230-237](../../backend/internal/application/auth/registration.go)),**不创建 subscription 行**。
- 注册码消费是事务内原子 UPDATE `consumeRegistrationCodeTx`([postgres/user/repository.go:535](../../backend/internal/infra/persistence/postgres/user/repository.go)),`status active→used` + 写 `used_by_user_id`/`used_at`,`RowsAffected==0` 即冲突。
- 用户创建底层 `createWithCredentialTx`([postgres/user/repository.go:644](../../backend/internal/infra/persistence/postgres/user/repository.go)):建 user + credential + 可选 subscription,**不发任何余额**。
- 注册码管理员生成 [application/registrationcode/service.go:28](../../backend/internal/application/registrationcode/service.go),16 位去歧义字符集 `ABCDEFGHJKLMNPQRSTUVWXYZ23456789`([service.go:84-95](../../backend/internal/application/registrationcode/service.go))。
- 注册码 HTTP 四件套:[transport/http/registrationcode/](../../backend/internal/transport/http/registrationcode)(handler/router/module/dto),挂在 admin group([router.go:5-9](../../backend/internal/transport/http/registrationcode/router.go))。

### 计费与余额(奖励载体)

- `BillingAccount.BalanceNanousd`([models/billing.go:107](../../backend/internal/infra/persistence/models/billing.go))是**按量余额**,单位纳美元。
- free 计划 `PeriodCreditNanousd=1000000000`($1/月)在 [schema.go:365-369](../../backend/internal/infra/persistence/schema/schema.go) seed,**这是隐式月度额度,与 BillingAccount 余额是两套**。计费时找不到活跃付费订阅就回退 free([billing/service.go:1255-1278](../../backend/internal/application/billing/service.go))。
- **增量余额 + 流水的现成模板**:`applyRedemptionBalance`([postgres/billing/repository.go:2289-2319](../../backend/internal/infra/persistence/postgres/billing/repository.go)),用 `gorm.Expr("balance_nanousd + ?", delta)` 原子增量,并写一条 `BalanceTransaction` 流水。**当前是 `RedeemCode` 事务内私有函数,不是独立可调方法。**
- 余额账户按需创建:`getOrCreateBillingAccountForUpdate`(`SetBillingAccountBalance` 路径里,[postgres/billing/repository.go:785](../../backend/internal/infra/persistence/postgres/billing/repository.go))。
- 流水类型常量在 [domain/billing/types.go:92-103](../../backend/internal/domain/billing/types.go)(如 `BalanceTransactionTypeRedemption`、`BalanceTransactionTypeAdminSet`)。**需新增一个 `BalanceTransactionTypeInvitation`。**
- 单位换算 `usdToNanousd`([billing/service.go:3038](../../backend/internal/application/billing/service.go)):`int64(math.Round(usd * 1e9))`。

### 配置(Settings)

- 结构化系统设置表 `system_settings`(namespace+key 唯一,[models/setting.go:6-20](../../backend/internal/infra/persistence/models/setting.go)),已有完整 SettingsRepository/Service。
- 默认配置在 [application/settings/seed.go:18](../../backend/internal/application/settings/seed.go) `defaultSettings()`,已有 `auth`/`billing`/`chat` 等 namespace。**邀请奖励配置新增 `invitation` namespace。**
- 前端通用配置面板 `settings-runtime-panel.tsx`,支持 bool/float/string 等,各 section 复用它。

### 迁移

- GORM AutoMigrate,[schema.go:118-137](../../backend/internal/infra/persistence/schema/schema.go):先 `HasTable` 跳过建表、再 AutoMigrate 只加列/索引,**不删列、不动存量**。
- 新模型在 [schema.go:12-76](../../backend/internal/infra/persistence/schema/schema.go) `Models()` 末尾注册。
- 存量回填范式:`HasColumn` 守卫 + `UPDATE`,参考 `backfillUsageLedgerBillingAt`([schema.go:201-208](../../backend/internal/infra/persistence/schema/schema.go))。

### User 模型

- domain `User`([domain/user/types.go:32-55](../../backend/internal/domain/user/types.go));persistence `identity_users`([models/user.go:28-54](../../backend/internal/infra/persistence/models/user.go));转换 `toDomainUser`([postgres/user/repository.go:1430-1455](../../backend/internal/infra/persistence/postgres/user/repository.go))。
- **本期不修改 user 表**(owner 反馈 #3:fork 上游同步)。邀请码改存独立表 `invitation_codes`(见「边界 1」),`toDomainUser` 不动。

## 边界(design 主体)

### 1. 数据模型

> **fork 约束(owner 反馈 #3)**:本项目是 fork,后续要同步上游。**不修改 `identity_users` 等任何已有表结构**。邀请码与邀请关系全部落在**新建表**,迁移只增不减,backfill 不动存量列。这样上游对老表的改动与本期功能零冲突。

**新增 2 张表,不改任何现有表。**

**(a) 新表 `invitation_codes`** —— 每用户固定的邀请码(1:1 映射用户)

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | uint | pk | |
| `user_id` | uint | **unique** | 所属用户,1:1 |
| `code` | varchar(20) | **unique** | 带前缀的邀请码,如 `INV-AB3KX9M2` |
| `created_at` / `updated_at` | time | | |

- `user_id` 唯一:每用户恰好一个邀请码。
- `code` 唯一:全局唯一,可 `WHERE code=?` 直接定位邀请人。
- 用 `BaseModel`(含软删除)还是 `ControlPlaneModel`(硬删除)?——邀请码是用户的稳定别名,**建议 `BaseModel`** 软删除,与 user 生命周期一致(用户软删时其邀请码逻辑失效)。**此点待 owner 在 review 时拍板**(影响上游同步语义)。

**(b) 新表 `invitation_relationships`** —— 一次邀请事件 + 双方奖励发放状态

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | uint | pk | |
| `inviter_user_id` | uint | index | 邀请人 |
| `invited_user_id` | uint | **unique** | 被邀请人;唯一保证一人只被邀请一次 |
| `invitation_code` | varchar(20) | | 使用的邀请码(冗余,审计) |
| `invitee_reward_nanousd` | bigint | default 0 | 被邀请人实发奖励(快照) |
| `inviter_reward_nanousd` | bigint | default 0 | 邀请人实发奖励(快照) |
| `invitee_rewarded_at` | *time | nullable | 被邀请人奖励发放时间 |
| `inviter_rewarded_at` | *time | nullable | 邀请人奖励发放时间 |
| `created_at` / `updated_at` | time | | |

- 用 `ControlPlaneModel`(硬删除):邀请关系是不可变审计事实,与 `registration_codes`/`wechat_registration_issuances` 一致。
- `invited_user_id` 单列唯一(非联合):一个被邀请人一生只能被奖励一次 → 最严格。

**为什么不把邀请码并进 `invitation_relationships`**:邀请码是用户的长期属性(注册即生成、要对外分享),邀请关系是一次性事件记录;混表会导致「没有邀请任何人的用户存不住自己的邀请码」。两表职责分离,且都是新表,不动老表。

### 2. 邀请码生成与存储

- 前缀 **`INV-`**(owner 反馈 #6),与注册码 `XXXX-XXXX-XXXX-XXXX`(16 位带连字符)格式一眼可辨。
- 随机部分字符集复刻注册码:`ABCDEFGHJKLMNPQRSTUVWXYZ23456789`。
- 随机部分默认长度 **7**(总长 `INV-` + 7 = 11 字符;32^7 ≈ 3.4e10 空间,足够);长度可配置(`invitation.code_length`,默认 7)。
- 完整码 = `INV-` + 大写随机串。前端/接口统一存带前缀的完整码。
- 生成时重试(最多 ~10 次)规避极小概率碰撞;仍碰撞则报错(人工介入)。
- 生成时机:**注册事务内**为新用户在 `invitation_codes` 插入一行。
- 存量用户:**迁移阶段批量 backfill** —— `invitation_codes` 表新建后,给 `identity_users` 里还没有邀请码记录的用户生成(只查不改 user 表,只在 invitation_codes 表插入)。仿 `backfillUsageLedgerBillingAt` 的 `HasTable`/幂等守卫。

### 3. 配置项(invitation namespace)

在 [seed.go](../../backend/internal/application/settings/seed.go) `defaultSettings()` 追加:

| namespace | key | type | 默认 | 说明 |
|---|---|---|---|---|
| `invitation` | `enabled` | bool | `false` | 总开关;关时不接受邀请码、不发奖励 |
| `invitation` | `invitee_reward_credit_usd` | float(string) | `0.5` | 被邀请人奖励($),>0 才发 |
| `invitation` | `inviter_reward_credit_usd` | float(string) | `0.5` | 邀请人奖励($),>0 才发 |
| `invitation` | `code_length` | int | `8` | 邀请码长度 |

> 金额以 USD 配置,发放时 `usdToNanousd` 转纳美元。任一金额 = 0 则对应方不发(只记关系、不发余额)。

### 4. 奖励发放事务流程(核心)

**全部在注册事务内,原子完成。** 不跨 repo、不开第二个事务。

```
RegisterWithEmailAndInvitationCode(email, pwd, emailCode, invitationCode, ...)
└─ tx begin
   ├─ ① 邮箱验证码校验(已有)
   ├─ ② createWithCredentialTx:建 user + credential                       (已有)
   ├─ ③ 为新 user 在 invitation_codes 插入一行(生成 INV-xxxxxxx)         (新增)
   ├─ ④ 解析邀请人(若 invitationCode 非空):
   │     ├─ SELECT user_id FROM invitation_codes WHERE code=? AND user_id<>新user
   │     ├─ 找不到 → 【宽松放行】忽略此码,跳过④后续,正常注册、不发奖、不报错
   │     │            (owner 反馈 #1:无效码不阻断注册)
   │     └─ 若 invitation.enabled=false → 同样宽松放行(不校验不发奖)
   ├─ ⑤ 写 invitation_relationships(inviter, invited=新user, code, 金额快照)
   │     └─ unique 冲突(invited 已被邀请过,如重注册)→ 宽松放行:记录但不重复发奖
   ├─ ⑥ invitee 奖励(若 invitee_reward_credit_usd>0):
   │     getOrCreateBillingAccountForUpdate(新user); balance_nanousd += inviteeReward
   │     写流水(type=invitation, refNo=invitation:invitee:{relationshipID})
   └─ ⑦ inviter 奖励(若 inviter_reward_credit_usd>0):
        getOrCreateBillingAccountForUpdate(inviter); balance_nanousd += inviterReward
        写流水(type=invitation, refNo=invitation:inviter:{relationshipID})
   └─ tx commit
```

- **宽松放行原则(owner 反馈 #1)**:邀请码无效、功能关闭、或被邀请人已被邀请过 —— 都**不阻断注册**,正常建用户,只是不发奖。唯一会回滚的是底层建用户失败。
- 新增内部函数 `applyInvitationReward(tx, userID, deltaNanousd, refNo, desc)`,**仿 `applyRedemptionBalance`**([postgres/billing/repository.go:2289](../../backend/internal/infra/persistence/postgres/billing/repository.go))的 `gorm.Expr` 增量 + 流水写法。
- 流水类型用新常量 `BalanceTransactionTypeInvitation`([domain/billing/types.go](../../backend/internal/domain/billing/types.go) 追加)。
- `refNo` 形如 `invitation:invitee:{relationshipID}` / `invitation:inviter:{relationshipID}`,便于对账。

#### period 模式余额奖励为什么有效(owner 反馈 #4 证实)

代码证据:`AddPeriodUsageAndSettleOverage`([postgres/billing/repository.go:580](../../backend/internal/infra/persistence/postgres/billing/repository.go))是 period 模式的结算主逻辑:

1. 每次用量先扣**周期额度** `PeriodCreditNanousd`(月度),即 `coveredNanousd`(行 645)
2. 周期额度用尽后,超出部分 `overageNanousd` = `chargeNanousd - coveredNanousd`,**从 `BillingAccount.balance_nanousd` 扣**(行 654 `nextBalance := account.BalanceNanousd - overageNanousd`,行 675-681 写库,行 682-689 写 `type=usage` 负向流水)

**结论**:`BillingAccount.balance_nanousd` 在 period 模式下是「周期额度用尽后的超额备用额度」。给被邀请人/邀请人充余额 = 给他们月度额度($1)外的额外预算,奖励完全有效、语义清晰。免费用户拿 $0.5 奖励后,月度额度用完还能多跑 $0.5 用量。**载体无需改变。**

### 5. 注册路径改造

- 新增 `RegisterWithEmailAndInvitationCode`([registration.go](../../backend/internal/application/auth/registration.go)),签名在现有 `RegisterWithEmailAndRegistrationCode` 基础上**叠加** `invitationCode` 参数(两者可同时传入:注册码管准入、邀请码管奖励,正交)。
- HTTP 注册 DTO([transport/http/auth/dto.go](../../backend/internal/transport/http/auth/dto.go))新增可选字段 `invitation_code`。
- **不破坏现有契约**:字段可选、缺省走原流程。

### 6. HTTP 接口契约

| 方法 | 路径 | 侧 | 说明 |
|---|---|---|---|
| POST | `/auth/register`(现有) | 用户 | 新增可选 `invitation_code` 字段;后端宽松处理无效码 |
| GET | `/me/invitation` | 用户 | **我的邀请面板**:`{invitation_code, invite_link, invite_count}` |
| GET | `/me/invitations` | 用户 | **我邀请过的用户列表**(分页,含被邀请人展示名、注册时间、我获得的奖励金额) |
| GET | `/admin/invitations` | 管理 | 邀请关系列表(分页,可按 inviter/invitee 过滤) |

- 「我的邀请码」:从 `invitation_codes` 表读(注册时已生成)。
- 邀请链接:`{PublicWebBaseURL}/register?invite={invitation_code}`。
- 已邀请用户列表(owner 反馈 #2):JOIN `invitation_relationships`(我是 inviter) ↔ `identity_users`(被邀请人),返回脱敏的展示名/用户名 + 注册时间 + 我实得的 `inviter_reward_nanousd`。注意**不泄露被邀请人的邮箱**等敏感信息。

### 7. 前端(owner 反馈 #2、#7)

- **注册页**:加「邀请码」输入框,**从 URL `?invite=` 自动预填,显示但 disabled(不可编辑)**(owner 反馈 #7)。用户能看到自己是通过谁的邀请码注册,但不能手改,防止奖励错发。
- **用户中心**:新增「邀请面板」——
  - 我的邀请码(带 `INV-` 前缀)+ 一键复制
  - 邀请链接 + 复制
  - 已邀请用户数(统计)
  - 已邀请用户列表(展示名 + 注册时间 + 我获得的奖励),分页
- **后台**:新增 `invitation` 配置 section(复用 `settings-runtime-panel`),管理后台导航注册;「邀请关系列表」页(可选)。

### 8. 迁移与 backfill(不动老表)

- `Models()`([schema.go:12-76](../../backend/internal/infra/persistence/schema/schema.go))注册 `InvitationCodes`、`InvitationRelationship` 两个新模型。**`identity_users` 不加任何列、不改任何字段。**
- 新表由 AutoMigrate 自动建(存量无损,GORM 只建表/加索引)。
- 新增 `backfillInvitationCodes(db)`:`HasTable("invitation_codes")` 守卫 → 查 `identity_users` 里 `id NOT IN (SELECT user_id FROM invitation_codes)` 的用户(纯查询,不改 user 表),分批(如每批 500)生成邀请码并插入 `invitation_codes`。幂等:重复迁移只补漏,不重复生成。
- seed 追加 `invitation` namespace 默认项(`enabled=true`、奖励默认 0.5,**默认开启**)。

### 9. 注册码格式迁移到 `REG-`(owner 反馈追加,合并在本期)

> **前提更新**:owner 确认注册码是**昨天(2026-08-09)刚加的本地新功能**(commit `ea6ed84`/`49c07ca`/`358b6cd`/`53e2352`,均为 8/9),**尚未上线,无真实存量用户码**。因此:
> - 不存在「旧码失效」风险 → **不需要双格式兼容**。
> - 不存在「上游 diff」顾虑 → 注册码是本地功能,改格式是改自己的代码。
> - 方案**干净迁移**,不留长期两格式并存的债。

**(a) 新生成格式**

`generateCode()`([application/registrationcode/service.go:84-95](../../backend/internal/application/registrationcode/service.go))改为生成 `REG-` + 随机串(去歧义字符集 `ABCDEFGHJKLMNPQRSTUVWXYZ23456789`)。例:`REG-AB3K9M2X7QP4`。

长度建议与邀请码对称:随机部分长度可配置(`registration.code_length` 或硬编码,默认取与原 16 位等长或更短)。**待 owner review 时定长度**——建议 `REG-` + 12 位(总长 16,与原码体感相近)。

**(b) 消费逻辑**

`consumeRegistrationCodeTx`([postgres/user/repository.go:535](../../backend/internal/infra/persistence/postgres/user/repository.go))`WHERE code = ?` 精确匹配,**无需改动**——新格式码照常匹配。因无存量用户码,不存在兼容问题。

**(c) 微信发码同步**

[wechat/service.go](../../backend/internal/application/wechat/service.go) 走同一 `generateCode`,改格式后微信发的码自动变 `REG-`,无需单独改。微信默认关键词 `13003`([wechat/service.go:14](../../backend/internal/application/wechat/service.go))不变。

**(d) 测试数据清理**

- [wechat/service_test.go](../../backend/internal/application/wechat/service_test.go) 等若有硬编码旧格式 `XXXX-XXXX-XXXX-XXXX` 的断言,需同步更新为新 `REG-` 格式。
- [postgres/registrationcode/repository_test.go](../../backend/internal/infra/persistence/postgres/registrationcode/repository_test.go) 同理。

**(e) 前端配合**

注册码输入框([features/auth/components/login-page.tsx:360-436](../../frontend/features/auth/components/login-page.tsx))是纯文本透传,无格式校验。更新占位符文案 `registrationCodePlaceholder`([i18n zh-CN/login.json:15](../../frontend/i18n/messages/zh-CN/login.json) 等)提示新格式即可(非必须,码是管理员发的,用户原样粘贴)。

**(f) 边界**

- 不改 `registration_codes` 表结构(列/索引不变,只是 `code` 列里的值变了格式)。
- 注册码与邀请码区分:注册码 `REG-` 前缀、邀请码 `INV-` 前缀,清晰可辨。

### 取舍(真实取舍 + 理由)

1. **邀请码存独立表 vs 改 user 表加列** → **选独立表**(owner 反馈 #3 强制)。fork 项目要同步上游,改老表会和上游 user 模型改动冲突;新表上游 merge 时零冲突。代价:多一张表、解析邀请人多一次 join。可接受。
2. **奖励载体:BillingAccount 余额 vs 奖励 Subscription** → 选 BillingAccount 余额(owner 已定)。一次性、不随周期重置、与兑换码语义一致。**period 模式已证实可用**(见第 4 节证据),无需改载体。
3. **邀请码无效时:严格回滚 vs 宽松放行** → **宽松放行**(owner 反馈 #1)。无效码/关功能/重复邀请都不阻断注册,正常建用户,只是不发奖。
4. **发放时机:注册即发 vs 激活后发** → 注册即发(owner 已定)。已知风险:易被脚本批量刷号薅额度,见「未决」。
5. **邀请码可见性:只读 vs disabled vs 可改** → **显示但 disabled**(owner 反馈 #7)。从 URL 提取,用户可见不可改。
6. **邀请关系软删 vs 硬删** → `ControlPlaneModel` 硬删除,不可变审计记录,与注册码一致。
7. **注册码格式:双格式兼容 vs 干净迁移** → **干净迁移**。注册码是昨天刚加、未上线的本地新功能,无存量用户码、无上游 diff 顾虑 → 直接改 `generateCode` 输出 `REG-` 格式,不留两格式并存的长期债。前提:owner 确认未上线。

### 影响面

**必须修改(邀请码功能:新增文件 + 新表,不动老表):**
- 后端:
  - 新增 `domain/invitation`(实体 + 类型)
  - 新增 `repository/invitation.go`(接口)、`postgres/invitation/repository.go`(实现)
  - 新增 `application/invitation/service.go`(生成邀请码、发奖、查询面板)
  - 新增 `transport/http/invitation/*`(handler/router/module/dto,含 `/me/invitation`、`/me/invitations`、`/admin/invitations`)
  - 新增 `models/invitation.go`(两个 GORM 模型)
  - 改 `application/auth/registration.go`(新增 `RegisterWithEmailAndInvitationCode`,事务编排邀请逻辑)
  - 改 `transport/http/auth/dto.go`(注册 DTO 加 `invitation_code` 可选字段)
  - 改 `domain/billing/types.go` + `models/billing.go`(新增 `BalanceTransactionTypeInvitation` 常量)
  - 改 `postgres/billing/repository.go`(新增 `applyInvitationReward` 增量发奖函数,或暴露通用增量方法)
  - 改 `application/settings/seed.go`(追加 `invitation` namespace 默认项)
  - 改 `schema.go`(`Models()` 注册两表 + `backfillInvitationCodes`)
- 前端:
  - 注册页加 `?invite=` 预填的 disabled 邀请码字段
  - 用户中心新增「邀请面板」组件(邀请码、链接、统计、已邀请列表)
  - 后台新增 `invitation` 配置 section + 导航注册

**必须修改(注册码格式迁移,第 9 节):**
- 后端:改 `generateCode()`([registrationcode/service.go:84](../../backend/internal/application/registrationcode/service.go))输出 `REG-` 格式。**不改 `registration_codes` 表结构、不改消费逻辑、无需存量数据迁移(功能未上线)。**
- 测试:更新硬编码旧格式断言([wechat/service_test.go](../../backend/internal/application/wechat/service_test.go)、[registrationcode/repository_test.go](../../backend/internal/infra/persistence/postgres/registrationcode/repository_test.go))。
- 前端:(可选)更新 `registrationCodePlaceholder` 文案。

**需要验证:**
- 邀请:注册事务原子性、`invited_user_id` 唯一、邀请码碰撞重试、无效码/关功能宽松放行、无码注册正常、backfill 幂等、`/me/invitations` 脱敏、老表 schema 零改动。
- 注册码:新码 `REG-` 格式生成正常、微信发码格式跟随、消费逻辑不变、`registration_codes` 表结构不变、相关测试断言更新。

**仍待调查/边界:**
- **billing mode 运行时切换**:period ↔ usage 切换不影响 BillingAccount 余额语义,但若切到不使用余额的模式,奖励余额会"沉淀"。本期不处理,文档说明。
- **微信 OAuth 注册路径**([postgres/user/repository.go:505](../../backend/internal/infra/persistence/postgres/user/repository.go))本期不接邀请码;未来接入时邀请关系用 UserID 关联。

## 证据

见「现场」与第 4 节各条行号引用(均来自当前工作树实测)。period 模式余额有效性证据:`AddPeriodUsageAndSettleOverage` [repository.go:580-689](../../backend/internal/infra/persistence/postgres/billing/repository.go)。注册码生成/消费证据:[registrationcode/service.go:84-95](../../backend/internal/application/registrationcode/service.go)、[postgres/user/repository.go:535](../../backend/internal/infra/persistence/postgres/user/repository.go)。

## 验收标准

**邀请码功能:**
1. **双向发奖**:有效邀请码注册成功后,被邀请人 `BillingAccount.balance_nanousd` 增加 `invitee_reward`,邀请人增加 `inviter_reward`;各有一条 `type=invitation` 的 `BalanceTransaction` 流水,金额与配置一致(默认各 0.5 USD = 各 5e8 nanousd)。
2. **邀请关系**:`invitation_relationships` 写入一行,`invited_user_id` 唯一;同一被邀请人重注册不重复发奖(宽松忽略)。
3. **邀请码固定**:同一用户在 `invitation_codes` 的 `code`(带 `INV-` 前缀)多次查询/重启后不变。
4. **无效码宽松放行**:填不存在/格式错的邀请码,注册正常完成、正常建用户、不发奖、不报错。
5. **无码注册**:不带邀请码注册正常完成,新用户自动在 `invitation_codes` 获得自己的邀请码。
6. **配置可控**:后台改 `invitee_reward_credit_usd` 后,新注册按新值发奖;`enabled=false` 时填邀请码也不发奖。
7. **存量无损**:迁移后存量用户在 `invitation_codes` 获得邀请码;**`identity_users` 等老表零改动**;上游 schema diff 为空(除新表)。
8. **用户面板**:`/me/invitation` 返回邀请码+链接+统计;`/me/invitations` 返回已邀请用户列表(含奖励、脱敏展示名),不含邮箱。
9. **自我邀请防护**:解析邀请人时校验 `inviter <> 新user`。
10. **事务一致**:任一步失败,注册回滚,不留半成品。
11. **邀请码软删除**:`invitation_codes` 用 `BaseModel`,用户软删后其邀请码逻辑失效。

**注册码格式迁移:**
12. **新码格式**:新生成的注册码均为 `REG-` 前缀格式。
13. **消费正常**:`REG-` 格式新码可被 `consumeRegistrationCodeTx` 正常消费注册。
14. **表结构不变**:`registration_codes` 表列/索引未改动;无存量数据迁移(功能未上线)。
15. **微信发码**:微信关键词发码生成的也是 `REG-` 格式。
16. **测试更新**:硬编码旧格式断言已更新为新 `REG-` 格式。

## 状态与未决

- **状态**:design v3(纳入 owner 全部反馈:邀请码 + 注册码格式迁移)已完成,**等待 owner review**。批准后按「影响面」实施,测试先行。
- **已拍板:**
  - ✅ `invitation_codes` 用 **`BaseModel`(软删除)**(owner 反馈)。
  - ✅ **启用独立 change review**(owner 反馈 #3 "我要 review")。
- **review 流程**(按 cs-feat 审查协议):
  1. owner 批准本 design → 进入实现。
  2. 实现完成 + 自测通过后,冻结 staged diff。
  3. 我创建 fresh 异构 reviewer(显式指定最强稳定 model),对 staged diff 跑 `cs-review`。
  4. 处理 blocking/important findings,重跑验证,必要时 follow-up 复审(同 reviewer 同 session,最多 3 轮)。
  5. blocking 清零 + important 处理/接受后,报告可提交状态(**不自行 commit**,等你授权)。
- **已知风险(本期不处理,需 owner 知情)**:
  - 注册即发 + 无防刷 → 可被脚本批量注册薅额度(每号双向 $1)。建议上线后观察。
  - 注册码改格式虽用双格式兼容消除旧码失效风险,但与上游持续 diff(永久)。
- **未来扩展(非本期)**:微信 OAuth 接邀请码、邀请人持续分成、邀请统计/排行面板、防刷体系。
- **后台 invitation 配置 UI(已实现延后)**:邀请配置(`enabled` / 奖励金额 / `code_length`)已存于 `system_settings.invitation` namespace,种子默认 `enabled=true`(owner 确认默认开启)。**自定义后台配置 section 暂未做**(AdminLogin 那套字段编辑器模板量大,且配置已可通过现有 settings API 编辑)。如需可视化后台开关,后续单独补一个 invitation 配置 section(复用 `settings-runtime-panel` / `SettingsFieldEditor`)。
