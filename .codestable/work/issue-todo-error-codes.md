# TODO error-code CI fix

Archived, completed at commit `56e95e1`. The staging instructions, local runner paths and process IDs below describe the historical execution only. Current state: [TODO feedback](feat-miniapp-todo-feedback.md) and [release 63](todo-release-63.md). Local logs/scripts mentioned below are not part of the shared handoff.

The original feature is commit `d5422a1dc80c82d4cea68467bbc2aa9b69ec5dd1` on `dev`, already pushed to `origin/dev`. Two Workspace Quality guards rejected generic task-capacity errors and Chinese service error constructors. Both failures were reproduced before this fix; guard tests are unchanged.

The 12 intended fix files are staged: TODO domain/service/store/HTTP handler and tests, additive shared response fallback entries, both locale catalogs, the new bilingual resolver test, miniapp client and test, and the canonical design document. Preserve all unrelated user changes, `.mindfs` files, local reports, scripts and verification logs. Only commit the intended 12 files. Commit/push are authorized as continuation of the user's push request. Commit text must contain only TODO information and never the prohibited product or switching terms. Use `fix: correct todo error codes and translations`.

Six explicit TODO codes, typed English errors and translations are implemented. The HTTP handler passes an empty message to select the code fallback; generic shared normalization rules are unchanged. Two invalid-request fallback messages use the normalizer's lowercase spelling. HTTP status codes and persistence behavior are preserved.

Passed: both original CI guards; HTTP contracts and all seven error-response cases; TODO domain/application/persistence tests; miniapp client tests (5), typecheck and isolation; bilingual resolver tests (2); targeted frontend lint; formatting and staged whitespace checks. Backend verification exit 0 is recorded in `todo-verify-backend.status` at 2026-09-11 08:44:27 UTC. Do not rerun these without a relevant change.

Interrupted foreground runs could not complete API generation. `api:generate` was invoked previously, but its final result was lost. `api:check` now regenerates all outputs and checks consistency. All staged source/test files were verified unchanged before resuming.

At 2026-09-11 09:59:25 UTC, the local `.codestable/work/todo-ci-verify.ps1 -Resume` runner resumed through WMI as a hidden independent process, PID 3496, saved in `todo-verify-runner.pid`. It skips passed phases and performs API check, miniapp build, then dist check. Output and per-phase exit codes are durable in `todo-verify-{backend,api,build,dist}.{log,status}`, plus `todo-verify-runner.log`. A read-only watcher is exec 36197. After interruption, inspect these files and the independent process before starting anything; never launch duplicate checks. The log confirms API check started normally.

API check passed with exit 0 at 2026-09-11 10:08:33 UTC. Output explicitly confirms Swagger/TypeScript contracts are up to date and the API package typecheck ran successfully. Miniapp build passed at 10:11:49 UTC and dist checks passed at 10:11:58 UTC. All four durable verification phases completed successfully.

Committed exactly the intended 12 files as `56e95e141f4f66876b3004cfe4d0f499813e15df`, with the exact message `fix: correct todo error codes and translations`, and pushed to `origin/dev`. The remote branch hash matches. An empty index lock left at 08:50 UTC initially prevented commit; verified no Git processes existed and the lock was over an hour old, then removed only that stale `.git/index.lock`. No other user changes or local reports were staged. Real Git is `C:/Program Files/Git/cmd/git.exe`; the PATH wrapper is slow. No deployment or miniapp upload occurred.

Complete: at 2026-09-11 10:20:57 UTC, GitHub API confirmed all three checks completed successfully for this exact commit: Workspace Quality run 34557669474, GHCR Image run 34557669584, and Commit Message run 34557669481. The two original CI failures are fixed and the pushed revision is green. All local verification phases finished; retain local evidence without committing it.
