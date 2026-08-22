# Local Toolbox 0.5.0 Windows/Chrome acceptance matrix

Run this matrix on supported Windows x64 with the unpacked 0.5.0 extension loaded in current stable Chrome and the 0.5.0 Native Messaging helper registered. Record the Windows and Chrome versions, test URL (without private tokens), result, and evidence for every row. Never attach cookies or authenticated URLs to evidence.

| Area | Setup and action | Expected result | Result / evidence |
| --- | --- | --- | --- |
| Direct MP4 | Open a page containing a public MP4, select its detected card, and download it. | One useful candidate is shown; strategy is direct HTTP; progress stays below 100% until rename/finalization; the playable file has the expected size. | ☐ Pass ☐ Fail — |
| HLS/m3u8 | Open a public, non-DRM HLS page and download the detected stream. | Segments are not shown as separate media; the manifest routes through the stream/remux path; stages distinguish download and processing; output plays. | ☐ Pass ☐ Fail — |
| YouTube | Paste a public video page URL and download the default quality. | Page analysis succeeds through yt-dlp, a compatible merged output is produced, and no raw format flood appears in the main panel. | ☐ Pass ☐ Fail — |
| Facebook | Paste a public Facebook media URL and download it. | Public content downloads or an actionable upstream extraction/unavailable error is shown. | ☐ Pass ☐ Fail — |
| Instagram | Paste a public post/Reel URL and download it. | Public content downloads or an actionable authentication/unavailable error is shown. | ☐ Pass ☐ Fail — |
| X / Twitter | Paste a public post with media and download it. | Public media downloads or an actionable extraction/unavailable error is shown. | ☐ Pass ☐ Fail — |
| Browser session privacy | Use content that requires the current Chrome session, then inspect persisted jobs/history and the helper temp directory after completion. | Context is scoped to the active host/task; cookies enable the task when required but are absent from storage, history, retry payloads, logs, and disk after completion. | ☐ Pass ☐ Fail — |
| MP3 extraction | Choose **Download MP3** for a supported page and repeat with a local media file. | Download/processing stages are truthful, configured bitrate is applied, and the MP3 is playable. | ☐ Pass ☐ Fail — |
| Subtitles | Download manual and automatic subtitles using configured languages on content offering them. | Available requested languages are saved; unavailable subtitles produce a concise actionable result rather than a false success. | ☐ Pass ☐ Fail — |
| Thumbnail | Download the thumbnail for supported public content. | The thumbnail is saved as a valid image without downloading the full video. | ☐ Pass ☐ Fail — |
| Batch URLs | Paste multiple URLs including a duplicate, with playlist mode off, and enqueue. Repeat with a supported playlist and playlist mode on. | Unique jobs are queued, accidental duplicates are rejected, per-item states appear, and playlist mode follows the selected setting. | ☐ Pass ☐ Fail — |
| Queue limits | Set network concurrency to 2 and processing concurrency to 1; enqueue at least four mixed jobs. | No more than two network downloads and one heavy processing job run concurrently; queued jobs start FIFO as capacity opens. | ☐ Pass ☐ Fail — |
| Cancellation and retry | Cancel queued, downloading, and processing jobs; retry cancelled and deliberately failed jobs. | Each reaches `cancelled` without reverting; partial output is not presented as complete; retry reconstructs intent but not credentials. | ☐ Pass ☐ Fail — |
| Restart recovery | While a job runs, close/reopen the Side Panel and restart Chrome. | Side Panel restart preserves current state; after a browser/service-worker interruption the job is reconciled or marked `interrupted` and offers retry, never false completion. | ☐ Pass ☐ Fail — |
| Progress and metrics | Observe a download followed by merge/remux. | Speed, transferred/total, elapsed time, and meaningful ETA update; processing has its own stage; 100% appears only after overall success. | ☐ Pass ☐ Fail — |
| Unicode/open location | Save to a path containing Arabic characters and spaces, then use **Open file location** from completion and history. | Correct Explorer folder/file opens for both fresh completion and persisted history. | ☐ Pass ☐ Fail — |
| Detector noise/blob/DRM | Exercise a page with HLS segments and a `blob:` player, then a known protected stream. | Noise and duplicate segments are suppressed; blob guidance prefers an underlying candidate; protected content is explicitly unsupported and never reported downloadable. | ☐ Pass ☐ Fail — |
| Update compatibility | From installed 0.4.0, apply a locally hosted copy of reviewed candidate metadata/package in an isolated test feed. | Existing size/SHA-256 verification succeeds, updater v2 is installed side-by-side, extension/helper become 0.5.0, and normal upgrades require no manual ZIP handling. | ☐ Pass ☐ Fail — |

## Release sign-off

- [ ] Automated release-candidate gate passed from the exact commit under test.
- [ ] No regression was found in working 0.4.0 download, conversion, settings, update, or open-path behavior.
- [ ] Known limitations were compared with `product/README.md` and remain accurate.
- [ ] `latest.json` and `releases/` were not modified or published.
- [ ] Human reviewer approved publication separately from this feature PR.
