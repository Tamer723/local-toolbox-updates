# Codex Queue — Local Toolbox 0.5.0

This queue is coordinated around integration branch `codex/0.5.0`.

## Gate 0 — PR #2 integration
Do not start the parallel implementation/QA tracks below until PR #2 is merged into `codex/0.5.0` after its blocking review findings are fixed and CI passes.

Current blockers tracked on PR #2:
- remove duplicate Chrome listeners and stale `blobByTab` state
- make updater migration safe for the real 0.4.0 → 0.5.0 path on Windows
- pass `.github/workflows/product-ci.yml`

## Track A — Media Detector QA
Audit deduplication, signed/transient URLs, HLS/DASH detection, blob fallback, quality/size inference and badge correctness across navigation. Preserve privacy and avoid capturing DRM/protected streams.

## Track B — Download Engine / Platforms
Exercise direct HTTP vs yt-dlp/FFmpeg routing, YouTube/Facebook/Instagram/X flows, playlists/batch URLs, scoped cookies/referer/user-agent, 403/retry behavior and file naming.

## Track C — Job Manager / Performance
Stress queue limits, concurrent downloads vs processing lane, cancellation, interrupted/restart recovery, duplicate prevention, cache behavior, progress monotonicity, speed/ETA and history/open-path behavior.

## Track D — UI / Regression
Review Arabic RTL Side Panel, detected-media cards, batch flow, settings, update UX, error states, long filenames, empty states and regression against working 0.4.0 functions.

## Integration rule
Each track uses its own feature/QA branch created from the post-merge `codex/0.5.0` head and opens a PR back to `codex/0.5.0`. Do not modify production `latest.json` or `releases/` in these tracks.
