Work on GitHub issue #1 in this repository using branch `codex/0.5.0`.

First read `AGENTS.md` and `CODEX_TASK_0.5.0.md` completely. Then run `scripts/bootstrap-product.sh` to reconstruct the current 0.4.0 source under `product/` and audit the existing architecture before changing code.

Implement the full 0.5.0 scope as one release candidate. You may split the work into internal subtasks/commits, but do not publish incremental user-visible releases. Preserve the stable extension key/ID, Native Messaging host, privacy rules, and self-update compatibility. Do not modify production `latest.json` or publish 0.5.0 to installed users.

Run all checks required by `AGENTS.md`, build and verify a Windows x64 candidate update package, document known limitations and manual test scenarios, then push the finished work to `codex/0.5.0` and open a PR to `main`. Do not merge or publish the production update feed without human approval.

If a requirement conflicts with the existing implementation, prefer preserving backward compatibility and document the trade-off in the PR rather than silently breaking installed clients.
