# Local Toolbox — Codex instructions

## Project
Local Toolbox is a Windows-first local productivity/media toolbox composed of:
- Chrome Extension (Manifest V3, Side Panel)
- Go Native Messaging helper (`com.localtoolbox.helper`)
- Go updater used by the self-update flow
- Local external tools: `yt-dlp`, `FFmpeg/ffprobe`, `Deno`

Current production baseline: **0.4.0**.
Target of this branch: **0.5.0 — Downloader Complete**.

## Source layout
The 0.4.0 source snapshot is stored as base64 chunks under `sources/0.4.0/source.b64.*`.
Run `scripts/bootstrap-product.sh` once to reconstruct the editable source tree under `product/`.
After reconstruction, all application changes should be made in `product/`.

## Non-negotiable compatibility rules
- Keep the Chrome extension `manifest.key` unchanged. The extension ID must remain stable.
- Keep Native Messaging host name exactly `com.localtoolbox.helper`.
- Preserve the existing self-update protocol and backward compatibility from installed 0.3.1/0.4.0 clients.
- Do not require users to manually download ZIP files for normal upgrades.
- Do not publish or modify production `latest.json` / `releases/` from this feature branch unless explicitly instructed after review.
- Do not remove working 0.4.0 functionality while refactoring.

## Security and privacy rules
- Local processing is the default.
- Never instruct users to disable antivirus/security software.
- Do not use encoded PowerShell or suspicious installer tricks.
- Never log, persist, or write browser cookies/session tokens to history, retry data, or disk unless strictly required; current design expects scoped, ephemeral cookies per task.
- Keep Referer/User-Agent/Cookies scoped to the requested media task.
- Sanitize child-process environment as existing code does (including problematic SSL key logging variables).
- Do not send media to third-party processing websites.
- DRM bypass is out of scope.

## Architecture boundaries
- Extension: discovery, UI, tab/page context, task intent, settings.
- Native helper: filesystem access, HTTP direct download, process management, yt-dlp, FFmpeg, local jobs.
- Updater: verify metadata, size, SHA-256, install package safely, restart/reload as already implemented.
- Prefer direct HTTP download when a stable direct media URL is available; use yt-dlp/FFmpeg when stream extraction, HLS/DASH, merging, or audio extraction is required.

## UX rules
- Arabic-first RTL UI, compact and clean.
- Keep common actions in the main Side Panel; move advanced/default options to Settings.
- Main UI should emphasize: current page, detected media, quick actions, active jobs, history.
- Show truthful states: analyzing, downloading, processing/merging, complete, failed, cancelled.
- Never show 100% before the entire job actually succeeds.

## Development style
- You may use multiple internal commits/subtasks, but the next user-visible release is one release: **0.5.0**.
- Do not create user-facing 0.4.1/0.4.2/0.5.1 releases while implementing this task.
- Refactor when useful, but keep changes understandable and testable.
- Prefer deterministic, explicit code over clever shortcuts.
- Add tests around pure parsing/deduplication/job-state logic where practical.

## Required checks before marking work complete
From `product/`:
1. `gofmt` all Go files.
2. `go test ./...` in `helper-src`.
3. `go test ./...` in `updater-src`.
4. Build Windows x64 helper with `GOOS=windows GOARCH=amd64 CGO_ENABLED=0`.
5. Build Windows x64 updater with the same target.
6. `node --check` every extension `.js` file.
7. Validate all JSON files.
8. Verify `manifest.json` version is exactly `0.5.0` for the release candidate.
9. Verify update package contains at least `payload/extension/manifest.json` and `payload/local-toolbox-helper.exe`.
10. Verify candidate package size and SHA-256.

## Completion behavior
When implementation is ready:
- Do **not** publish the production update feed automatically.
- Push the work to `codex/0.5.0`.
- Open a PR to `main` with a concise architecture summary, tests run, known limitations, and manual test checklist.
- Wait for human review before publishing 0.5.0 through the self-update feed.
