const test = require('node:test');
const assert = require('node:assert/strict');
const detector = require('./media-detection.js');

test('classifies direct, HLS, and DASH media while rejecting segments', () => {
  assert.deepEqual(detector.classify('https://cdn.test/video.mp4'), {kind: 'direct', ext: 'mp4'});
  assert.deepEqual(detector.classify('https://cdn.test/live', 'application/vnd.apple.mpegurl'), {kind: 'hls', ext: 'm3u8'});
  assert.deepEqual(detector.classify('https://cdn.test/manifest', 'application/dash+xml'), {kind: 'dash', ext: 'mpd'});
  assert.equal(detector.isSegment('https://cdn.test/chunk.CMFV?token=x'), true);
});

test('deduplicates refreshed signatures without merging quality variants', () => {
  const first = detector.identity('https://CDN.test/movie-1080p.mp4?X-Amz-Signature=old&Expires=1&quality=1080');
  const refreshed = detector.identity('https://cdn.test/movie-1080p.mp4?X-Amz-Signature=new&Expires=2&quality=1080');
  const lowerQuality = detector.identity('https://cdn.test/movie-1080p.mp4?X-Amz-Signature=new&quality=720');
  assert.equal(first, refreshed);
  assert.notEqual(first, lowerQuality);
});

test('infers quality, bitrate, and exact range size deterministically', () => {
  assert.deepEqual(detector.infer('https://cdn.test/video_1920x1080_4500kbps.mp4', {size: 123, sizeExact: true}), {
    width: 1920, height: 1080, bitrate: 4500000, size: 123, sizeExact: true
  });
});

test('merge keeps useful metadata and replaces a stale signed URL', () => {
  const merged = detector.merge(
    {url: 'https://cdn.test/a.mp4?token=old', width: 1920, firstSeen: 10},
    {url: 'https://cdn.test/a.mp4?token=new', height: 1080, firstSeen: 20}
  );
  assert.equal(merged.url, 'https://cdn.test/a.mp4?token=new');
  assert.equal(merged.width, 1920);
  assert.equal(merged.height, 1080);
  assert.equal(merged.firstSeen, 10);
});

test('merge never downgrades a protected observation to direct-safe', () => {
  const merged = detector.merge({protected: true, directSafe: false}, {protected: false, directSafe: true});
  assert.equal(merged.protected, true);
  assert.equal(merged.directSafe, false);
});
