# Local Toolbox 0.5 shared contracts

The canonical machine-readable vocabulary is `product/contracts/contracts-0.5.json`. The extension enforces it through `extension/contracts.js`; the helper mirrors it in `helper-src/protocol.go`. Contract tests fail when their protocol version, states, strategies, or commands diverge.

## Compatibility and envelope

Native Messaging remains length-prefixed JSON through `com.localtoolbox.helper`. Every new request and response carries `protocolVersion: 1`. The helper accepts an omitted/zero version for installed 0.3.1/0.4.0 clients, but rejects versions newer than it understands with `invalid_request`. Every response also retains the 0.4-compatible `event`, `message`, `version`, and optional metrics fields.

## Domain contracts

### `MediaItem`

Represents one deduplicated usable media resource: stable `id`, `url`, optional `pageUrl`, `kind`, `container`, MIME type, source, dimensions, bitrate, byte size and exactness, plus `directSafe` and `protected`. `blob:` values are observations rather than downloadable `MediaItem` URLs. `protected: true` must never be routed as downloadable.

### `DownloadRequest`

Contains the non-secret operation intent (`jobId`, URL/path, operation, quality, bitrate, subtitle languages, playlist mode, filename and requested strategy). `referer`, `userAgent`, and cookies are task-scoped transport context. The extension's persisted retry projection must omit cookies and user-agent; the helper may materialize cookies only in an ephemeral file removed after the child process exits.

### `Job` and `JobState`

`Job` binds an ID, sanitized request, strategy, timestamps, state and progress. The states are `queued`, `analyzing`, `downloading`, `processing`, `completed`, `failed`, `cancelled`, and `interrupted`. Terminal states are completed/failed/cancelled; interrupted is explicitly retryable after process or browser restart. Legacy native event names are mapped at the boundary rather than used as the persistent state model.

### `ProgressEvent`

Carries job/state/stage, percentage, transferred/total bytes, speed, ETA and elapsed time. Both boundaries clamp non-completed progress to 99.5%; only `state: completed` may report 100.

### `DownloadResult`

The successful terminal value contains `jobId`, `state: completed`, final Unicode-safe path and actual strategy. Failure is represented by the shared error model, not a partially successful result.

### `DownloadStrategy`

- `direct_http`: stable, direct HTTP media; no extractor process.
- `yt_dlp`: site extraction, adaptive streams, subtitles, thumbnails and playlists.
- `ffmpeg`: local conversion or explicit processing. yt-dlp jobs may invoke FFmpeg internally while their router strategy remains `yt_dlp`; the stage communicates merging/remuxing.

## Native commands and events

The exhaustive lists live in the JSON contract. Unknown commands are rejected before dispatch. Commands cover health/settings/tools/pickers, page analysis, all download operations, cancellation and self-update. Events cover command results, lifecycle/progress, errors, cancellation and updater progress. The Go writer always supplies the protocol version and derives canonical state and strategy for legacy event producers.

## Error model

Errors have `code`, localized `message`, optional technical `details`, `retryable`, and optional `httpStatus`. Stable codes include invalid request, unsupported/DRM, authentication/unavailable/expired URL, HTTP 403, extraction/tool/I/O/update failures, cancellation and internal failure. The legacy top-level message/details remain during the compatibility window.

## Capability flags

The `pong` response advertises direct HTTP, discovered yt-dlp/FFmpeg/ffprobe/Deno tools, playlists, subtitles, thumbnails, scoped browser-session support, cancellation, retry and restart recovery. UI enablement should prefer these flags; tool-status events remain available for detailed paths and versions.

## Change rules

Additive optional fields do not require a protocol bump. Removing or renaming fields, changing semantics, or changing enum wire values requires a new protocol version and a backward-compatible helper dispatch path. Update the JSON, JavaScript, Go, documentation and both contract test suites in the same checkpoint.
