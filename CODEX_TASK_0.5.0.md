# Codex task — Local Toolbox 0.5.0: Downloader Complete

## Mission
Build one substantial release, **0.5.0**, on top of production 0.4.0. The goal is to make the download/media-discovery subsystem feel complete enough for daily use before moving on to Whisper and the broader media toolbox.

Do not ship feature-by-feature user releases. Internal commits are welcome; the deliverable is one reviewed 0.5.0 release candidate.

## Starting point
1. Read `AGENTS.md` completely.
2. Run `scripts/bootstrap-product.sh` to reconstruct the 0.4.0 source under `product/`.
3. Audit the existing architecture before editing. Identify current message types, job states, storage keys, updater protocol, media-detector flow, and yt-dlp/FFmpeg command construction.
4. Preserve all working 0.4.0 behavior unless this spec explicitly changes it.

## Scope

### 1. Smart Media Detector v2
Improve the current DOM + Performance Resource Timing + `webRequest` detector.

Required behavior:
- Detect direct video/audio files and streaming manifests: MP4, WebM, MOV/M4V, MP3, M4A/AAC, OGG/Opus/WAV/FLAC, HLS/m3u8, DASH/mpd.
- Deduplicate the same media seen through DOM, performance entries, redirects, query-string variants, and network events when they represent the same usable resource.
- Filter obvious noise: tiny tracking media, icons, beacons, short irrelevant fragments, duplicate HLS segment requests, and non-media URLs mislabeled by extension.
- Extract/display useful metadata when available: type/container, resolution, bitrate, approximate/exact size, source (`DOM`, `network`, `stream`), and whether direct download is safe.
- Group related video/audio representations when they clearly belong to the same media item instead of flooding the UI with raw fragments.
- Handle `blob:` URLs gracefully: explain that the blob itself is not directly downloadable and prefer the underlying network/stream candidate when available.
- Keep DRM/protected media explicitly unsupported; never pretend a failed protected stream is downloadable.

### 2. Download strategy router
Implement one explicit strategy decision per media item/job:

`direct HTTP` -> for stable direct files
`yt-dlp` -> for supported site extraction, adaptive streams, playlists, or when direct download is insufficient
`FFmpeg` -> for HLS/DASH remux/merge/post-processing when required

Requirements:
- Prefer the least expensive reliable path.
- Do not invoke yt-dlp unnecessarily for a stable direct file.
- Pass scoped Referer/User-Agent/Cookies only for the active task and never persist secrets.
- Preserve current child-environment sanitization.
- Produce a human-readable strategy/stage in the UI without exposing noisy raw logs by default.

### 3. Platform flows
Provide robust primary actions for:
- YouTube
- Facebook
- Instagram Posts/Reels
- X / Twitter

Requirements:
- Page URL should be enough for normal public content.
- Authenticated content may use the current browser session only when necessary and only with scoped ephemeral cookie handling.
- Keep a graceful fallback from browser detection to yt-dlp extraction.
- Do not add site-specific scraping hacks when yt-dlp already handles the case reliably.
- Surface useful errors: authentication required, unavailable/private content, unsupported/DRM media, expired URL, HTTP 403, extraction failure.

### 4. Batch URLs and playlists
Add a practical batch workflow:
- Paste multiple URLs (one per line) and enqueue them in one action.
- Detect playlist URLs supported by yt-dlp and allow either single-item or playlist mode.
- Show number of queued items and per-item state.
- Prevent accidental duplicate jobs for the same canonical URL + operation unless the user explicitly retries.
- Preserve serial/parallel safety: default to **2 concurrent network downloads** and **1 heavy media-processing job**; expose limits in Settings.

### 5. Job Manager v2
Unify direct-download and yt-dlp/FFmpeg jobs behind a consistent state model.

Required states should cover at least:
`queued`, `analyzing`, `downloading`, `processing`, `completed`, `failed`, `cancelled`, and an interrupted/recoverable state when appropriate.

Required behavior:
- Persistent active/recent jobs across Side Panel/service-worker restarts.
- Correct progress semantics: 100% only after the whole job succeeds.
- Speed, transferred/total size when known, elapsed time, ETA when meaningful.
- Clear processing stages for download, audio extraction, subtitle work, merge/remux, finalization.
- Cancel active jobs.
- Retry failed/cancelled/interrupted jobs without persisting credentials.
- Pause/resume only where technically sound. If true pause is not reliable for a backend, do not fake it; provide a clear cancel/resume/retry behavior instead.
- Opening the completed file/folder must be reliable for Unicode/space-containing Windows paths.
- Keep history bounded and avoid storing sensitive headers/cookies.

### 6. Download outputs
Support these primary operations from the current page, a detected media item, or a pasted URL:
- Download video
- Extract/download MP3
- Download subtitles (manual + auto where yt-dlp supports them; configurable languages)
- Download thumbnail

For video quality:
- Keep a simple default quality setting.
- When format information is available, let the user choose a meaningful quality without exposing dozens of raw yt-dlp format IDs by default.
- Ensure merged video/audio output uses a broadly compatible container where possible.

### 7. UI/UX consolidation
Redesign the Side Panel only as much as needed to make the workflow fast and understandable.

Main panel order:
1. Current page / URL
2. Detected media
3. Quick actions
4. Active jobs
5. Recent history

Settings should contain advanced/default controls such as:
- Output directory
- Default video quality
- MP3 bitrate
- Subtitle languages
- Concurrent network downloads
- Concurrent processing jobs
- Browser-session behavior
- IPv4 preference
- Tool paths/status for yt-dlp, FFmpeg/ffprobe, Deno
- Auto-open/open-folder behavior

UX constraints:
- Arabic-first RTL.
- Avoid dense diagnostic text in the normal flow.
- Technical logs belong under a collapsed details section.
- Error messages should be concise, with an actionable explanation.

### 8. Performance
- Cache tool discovery/version checks; do not spawn version processes for every job.
- Cache page/media info for a short sensible TTL to avoid repeated extraction of the same URL.
- Avoid scanning/storage patterns that grow without bound.
- Keep Chrome service-worker work lightweight; heavy work remains in the helper.

### 9. Tests and regression protection
Add or improve automated checks where feasible.

At minimum verify:
- Media URL/type normalization and deduplication logic.
- Job-state transitions and 100%-only-on-success behavior.
- Duplicate-job prevention.
- Sensitive fields are omitted from persistent history/retry payloads.
- Direct download path works against a local HTTP test server or equivalent deterministic test.
- Go helper builds on Windows x64.
- Extension JS syntax and JSON validity.

## Explicitly out of scope for 0.5.0
- Whisper/transcription
- Image tools
- PDF tools
- Full video editor (trim/compress/merge UI beyond what is needed internally for downloads)
- DRM bypass
- Cloud processing/service dependency

## Release candidate requirements
Before opening the PR:
- Set extension and helper-visible release version to `0.5.0` consistently.
- Run every required check in `AGENTS.md`.
- Produce a candidate self-update ZIP with extension + Windows helper and verify SHA-256/size.
- Do **not** change production `latest.json` to 0.5.0 and do not publish the candidate to installed users yet.
- Include a concise manual test matrix in the PR for Windows Chrome covering at least:
  1. direct MP4 page
  2. HLS/m3u8 page
  3. YouTube video
  4. Facebook/Instagram/X public media
  5. MP3 extraction
  6. subtitles
  7. batch URLs
  8. cancellation/retry
  9. Chrome/Side Panel restart during a job
  10. open file location

## Definition of done
A PR from `codex/0.5.0` to `main` is open, CI/build checks pass, the candidate package is reproducible, known limitations are documented, and no production self-update has been published without human approval.
