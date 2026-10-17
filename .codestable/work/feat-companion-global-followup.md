# Companion global context and proactive recovery

User approved implementation on 2026-10-16 after reporting both foreground silence and missing offline contact. Scope: recover failed proactive/mark-read attempts, preserve delivered home messages, provide diagnostic reasons, read same-account conversations and TODO with exclusions and provenance, maintain follow-ups and deliver authorized WeChat subscription notifications from a server worker. No upstream table/schema/auth/billing changes. No Git commit/push or production release authorized by this approval.

Existing unrelated changes and timestamp feature (development upload 0.1.64.20261016) must be preserved. AGENTS.md now permits accurate descriptions beyond TODO; commit/upload text must still exclude the prohibited keyword case-insensitively.

Acceptance:
- Transient failures recover with bounded backoff; concurrent ticks never duplicate an accepted message. Read acknowledgement retries without falsely advancing confirmed state.
- Previously accepted home messages survive revisits; diagnostic reasons distinguish cooldown, unread, preference, unavailable model, and actual silence.
- Global retrieval reads only current user's live ordinary conversation content and TODO; exclusions/deletions/forgetting apply before inference. Sources and dates accompany retrieved context.
- Follow-ups retain due time, state and source; edits/completion/cancellation invalidate old deliveries. Background work is bounded, survives restarts and claims work atomically across instances.
- Subscription credentials stay server-side; notifications require a configured compatible template, binding and explicit user consent. Unknown delivery outcomes are not retried blindly.
- Relevant application, persistence, transport, client tests, typecheck, API contracts and production build pass. Live notification delivery remains contingent on actual platform template/authorization; no fabricated success.

Implemented foreground recovery, account-scoped source retrieval, follow-up persistence and edits, bounded background processing, notification consent/sending adapter, HTTP contracts, and the miniapp “记忆与跟进” interface. The user has now supplied the subscription template; its local deployment configuration is prepared, but has not been activated on a server or verified with a real notification. See `docs/companion-assistant.md` for behavior, configuration and limits.

Validation complete: 197 miniapp tests; TypeScript typecheck; isolation; production WeChat build and runtime bundle checks; Swagger/TypeScript contract check; backend companion application, persistence, notification adapter, HTTP and app registration tests; architecture/protocol/error-language guards. The four companion/notification packages also passed `go test -race`; `go vet` passed for those packages plus app registration.

Windows toolchain: existing Go CC pointed to a missing GCC. A portable compiler was extracted under ignored `.cache/toolchains/w64devkit`; SQLite headers came from the already installed go-sqlite3 module into `.cache/toolchains/sqlite-include`. Test processes set CC/PATH/CGO_CFLAGS locally, without changing system or Go-wide settings. GOMEMLIMIT=512MiB and GOMAXPROCS=2 bound Go build memory. Miniapp test/build logs are in `.codex/previews/assistant-miniapp-*.log`.

No commit, push, upload, deployment, live model invocation or real WeChat notification was performed for this extension. Previous development upload remains unchanged remotely.

## Conversational reminders — 2026-10-17

User subsequently authorized a general reminder lifecycle directly in conversation: relative one-shot times, recurring calendar reminders, query/countdown, update, complete, snooze, pause/resume and delete. User also requested review of the scheduler and capacity for 100–500 users. Implemented in new companion reminder domain/application/persistence/HTTP files and miniapp reminder components, with additive changes to the fork-owned companion integration only.

New tables: companion_reminders, companion_reminder_events, companion_reminder_turns, companion_reminder_owners. Text commands execute synchronously and persist their own dialogue; ordinary chat does not fabricate a saved reminder. Scheduling is independent of the chat lease, model and quiet setting. Five-second scans create durable occurrences; a separate dispatcher uses eight workers and 100ms request pacing per service instance. Pending events survive restarts; overdue/uncertain external sends follow the documented conservative rules. Explicit reminders survive memory forgetting; account cleanup includes the new feature tables.

The miniapp merges durable reminder turns into chat, shows live server-calibrated countdown cards, supports lifecycle actions, and previews due reminders on the home entry. New Swagger and TypeScript contracts are generated. See docs/companion-reminders.md for exact capabilities, capacity assumptions, time semantics and remaining real-WeChat validation.

Validation: targeted domain, application, persistence, HTTP and app integration tests passed; reminder application/persistence race tests passed, including 500 users, 5000 plans, 500 simultaneous due events and two schedulers (12.01s local SQLite with race enabled). A fake sender processed 500 tasks with an observed maximum of eight concurrent requests; its accelerated clock is not a throughput claim. Miniapp 199 tests, TypeScript checks and production build passed, as did API contract consistency and go vet across the reminder and app integration packages. Logs: .codex/previews/reminder-miniapp-tests.log and reminder-build.log. No new commit, upload or deployment authorization was given.

## Subscription template configuration — 2026-10-17

The supplied screenshot identifies the one-time 日程提醒 template (public number 571, 备忘录 category) with title field thing5 and time field time30. Added docker-compose.companion.yml with the supplied template ID and overridable fields/state; no upstream Compose files were changed. Default state is formal, with developer/trial overrides documented for device verification. This prepares local deployment configuration only: the backend image and miniapp still need the reminder changes deployed before consent and real delivery can be verified. No remote deployment or real notification was performed.

Validation: the installed YAML parser accepted the override; checked all four variable defaults against the supplied template and the app service in each of the three existing Compose variants. Existing subscription sender tests passed (`go test ./internal/infra/wechatminiapp -run TestSubscription -count=1`), using local HTTP simulation only. Docker CLI is unavailable locally, so actual `docker compose config` and container startup were not tested.

## Release review — 2026-10-17

The user authorized commit/push to the fork and continuation of the miniapp development upload, specified the exact description `完善消息提醒`, and will deploy the backend image produced by GitHub Actions. Do not deploy the server from this workstation. Development build version: 0.1.65.20261017. Formal-account review/public release and real notification delivery remain separate steps.

Reviewed the reminder lifecycle, account boundaries, notification consent, concurrent claims, restart handling and startup migrations. Two issues were fixed before release: workers now share a drained-queue result while continuing past full batches of ineligible candidates; card actions and untargeted commands now select the same next occurrence, including when a snoozed occurrence falls after a future calendar occurrence. Explicit actions on unavailable historical occurrences are disabled. Regression checks cover these cases.

All eight new tables are registered in NewStore AutoMigrate, called synchronously by registerCompanion before worker startup. The startup integration test now checks all eight with the feature enabled by default; the existing migration test checks repeated startup without losing deployed profile or memory data. Upstream tables and Compose files remain unchanged.

Final review validation: domain/application/persistence/sender/HTTP/app package tests passed, including default startup migration and repeated migrations preserving data. Reminder application and persistence race checks passed after the review fixes; the 500-account/5000-plan/two-scheduler case completed its scheduling phase in 14.54 seconds on local SQLite. The new earliest-occurrence and candidate-batch regressions passed. Related Go vet checks, regenerated Swagger/TypeScript consistency, miniapp typecheck and all 200 miniapp tests passed. These local results do not establish production throughput or real WeChat delivery.
