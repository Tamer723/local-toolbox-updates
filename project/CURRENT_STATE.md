# Local Toolbox — Current State

## Production
- Installed/public update feed baseline: 0.4.0.
- Production `latest.json` and `releases/` must not be changed during normal feature PRs.

## Integration
- Integration branch: `codex/0.5.0`.
- PR #2 contains the 0.5.0 release-candidate source and shared protocol contracts.
- PR #2 is not ready to merge until blocking review findings are fixed and product CI passes.

## Shared contract
Canonical contract is introduced by PR #2 under:
- `product/contracts/contracts-0.5.json`
- `product/extension/contracts.js`
- `product/helper-src/protocol.go`
- `docs/architecture/contracts-0.5.md`

## Release policy
- Internal commits and PRs may be frequent.
- User-visible release remains 0.5.0; do not create user-facing 0.4.x/0.5.x patch releases during this development cycle unless a critical production hotfix is explicitly approved.
- Final 0.5.0 must be reviewed, tested on real Windows + Chrome, then published through the existing self-update feed.
