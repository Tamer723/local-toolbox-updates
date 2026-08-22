# Local Toolbox — Current State

## Production
- Installed/public update feed baseline: 0.4.0.
- Production `latest.json` and `releases/` remain unchanged during normal development.

## Integration
- Integration branch: `codex/0.5.0`.
- PR #7 established the reviewed 0.5.0 release-candidate source and shared contracts.
- PR #8 (Media Detector QA) passed product CI and was squash-merged.
- PR #9 (Job Manager & Performance) passed product CI and was squash-merged.
- PR #10 (UI & Regression QA) passed product CI and was squash-merged.
- PR #11 (Download Engine & Platforms) passed product CI and has been reviewed as ready for integration, but remains open.
- Issues #3, #5 and #6 are completed. Issue #4 remains open until PR #11 is integrated.

## Verified fixes in the integrated 0.5.0 candidate
- Chrome media-detection lifecycle listeners are registered once.
- Tab navigation/removal clears both detected-media and blob state.
- Smart Media Detector uses shared detection logic, signed/transient URL de-duplication, segment suppression, metadata inference and protected-media guards.
- Job Manager uses bounded FIFO download/processing lanes with queued cancellation and concurrency tests.
- Side Panel job rendering uses canonical 0.5 job states and prevents non-completed jobs from presenting as 100%.
- 0.5.0 builds and packages `local-toolbox-updater-v2.exe` side-by-side rather than replacing the running 0.4.0 updater.
- Helper 0.5.0 prefers updater v2 and falls back to the legacy updater when needed.
- Product CI validates JSON, JS syntax/tests, Go tests, Windows cross-builds, candidate contents and production-feed protection.

## Open integration blocker
- Issue #12 tracks a cross-track cancellation regression: `cancel_requested` can overwrite the prior canonical state/progress and may regress a terminal `cancelled` job to a non-terminal state.
- This must be fixed and covered by JS regression tests before the final 0.5.0 Windows/Chrome acceptance test.

## Shared contract
Canonical 0.5 contract:
- `product/contracts/contracts-0.5.json`
- `product/extension/contracts.js`
- `product/helper-src/protocol.go`
- `docs/architecture/contracts-0.5.md`

## Release policy
- Internal commits and PRs may be frequent.
- User-visible release remains 0.5.0; do not create visible patch churn during this development cycle unless a critical production hotfix is explicitly approved.
- Final 0.5.0 must be fully integrated, pass the cancellation regression fix, then be tested on real Windows + Chrome before publishing through the existing self-update feed.
