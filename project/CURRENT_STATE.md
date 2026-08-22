# Local Toolbox — Current State

## Production
- Installed/public update feed baseline: 0.4.0.
- Production `latest.json` and `releases/` remain unchanged during normal development.

## Integration
- Integration branch: `codex/0.5.0`.
- PR #7 was reviewed, product CI passed, and it was squash-merged into `codex/0.5.0`.
- Code integration commit: `00a9604f265670238500564df55a6c08b8d33699`.
- The older PR #2 was closed as superseded.

## Verified fixes in the integrated 0.5.0 candidate
- Chrome media-detection lifecycle listeners are registered once.
- Tab navigation/removal clears both detected-media and blob state.
- 0.5.0 builds and packages `local-toolbox-updater-v2.exe` side-by-side rather than replacing the running 0.4.0 updater.
- Helper 0.5.0 prefers updater v2 and falls back to the legacy updater when needed.
- Regression tests cover listener duplication, blob cleanup, updater selection and side-by-side migration.
- Product CI validates JSON, JS syntax/tests, Go tests, Windows cross-builds, candidate contents and production-feed protection.

## Parallel 0.5.0 tracks
Ready to run from pre-created branches:
- #3 Media Detector QA → `feat/0.5-media-detector`
- #4 Download Engine & Platforms → `feat/0.5-download-engine`
- #5 Job Manager & Performance → `feat/0.5-job-manager`
- #6 UI & Regression QA → `feat/0.5-ui-qa`

Each track must open a PR back to `codex/0.5.0` and must not publish `latest.json` or `releases/`.

## Shared contract
Canonical 0.5 contract:
- `product/contracts/contracts-0.5.json`
- `product/extension/contracts.js`
- `product/helper-src/protocol.go`
- `docs/architecture/contracts-0.5.md`

## Release policy
- Internal commits and PRs may be frequent.
- User-visible release remains 0.5.0; do not create visible patch churn during this development cycle unless a critical production hotfix is explicitly approved.
- Final 0.5.0 must be integrated, then tested on real Windows + Chrome before publishing through the existing self-update feed.
