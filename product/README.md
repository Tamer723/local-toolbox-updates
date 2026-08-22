# Local Toolbox 0.5.0 release candidate

This source tree contains the Arabic-first Chrome Side Panel, Native Messaging helper, and updater for the 0.5.0 Downloader Complete release candidate.

## Architecture

- `extension/background.js` owns the native connection, bounded persistent active jobs/history, scoped browser-session context, network media detection, canonical duplicate prevention, and restart recovery.
- `extension/media-detector.js` combines DOM and Resource Timing candidates, while the background worker adds response metadata from `webRequest`.
- `helper-src` routes stable files to direct HTTP and extraction/streams/post-processing to yt-dlp and FFmpeg. Separate schedulers default to two network jobs and one processing job.
- `updater-src` preserves the existing manifest → size/SHA-256 verification → updater handoff protocol and safely installs only `payload/` entries.

Credentials are ephemeral: cookies are sent only with the active native request, written to a temporary Netscape file only while yt-dlp runs, and omitted from persistent job/history retry specifications.

## Known limitations

- DRM/protected streams are not supported.
- A `blob:` URL cannot itself be downloaded; the detector prefers an underlying HTTP or manifest candidate and explains when only a blob was observed.
- Pause/resume is intentionally not presented because it is not reliable across all direct, yt-dlp, and FFmpeg backends. Cancellation and credential-free retry are supported.
- Authenticated platform behavior depends on the current browser session and upstream yt-dlp extractor support.

## Candidate build

Run `./build-candidate.sh`. It creates a reproducible un-published candidate under `dist/`; it does not touch the production `latest.json` or `releases/` feed.
