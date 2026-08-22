(() => {
  'use strict';

  const DIRECT_EXTENSIONS = new Set(['mp4', 'webm', 'mov', 'm4v', 'mp3', 'm4a', 'aac', 'ogg', 'opus', 'wav', 'flac']);
  const SEGMENT_EXTENSIONS = new Set(['m4s', 'ts', 'cmfv', 'cmfa']);
  const TRANSIENT_QUERY = /^(?:utm_.+|fbclid|gclid|token|access_token|auth|authorization|expires?|exp|signature|sig|policy|key-pair-id|x-amz-.+|x-goog-.+)$/i;

  function parsedURL(raw, base) {
    try {
      const url = new URL(String(raw || '').trim(), base);
      return /^https?:$/.test(url.protocol) ? url : null;
    } catch { return null; }
  }

  function extension(raw) {
    const url = parsedURL(raw);
    const match = url?.pathname.toLowerCase().match(/\.([a-z0-9]{2,5})$/);
    return match ? match[1] : '';
  }

  function classify(raw, contentType = '') {
    const ext = extension(raw);
    const type = String(contentType).toLowerCase().split(';')[0].trim();
    if (ext === 'm3u8' || type.includes('mpegurl')) return {kind: 'hls', ext: ext || 'm3u8'};
    if (ext === 'mpd' || type.includes('dash+xml')) return {kind: 'dash', ext: 'mpd'};
    if (DIRECT_EXTENSIONS.has(ext)) return {kind: 'direct', ext};
    if (type.startsWith('video/')) return {kind: 'direct', ext: type.slice(6) || 'video'};
    if (type.startsWith('audio/')) return {kind: 'direct', ext: type.slice(6) || 'audio'};
    return null;
  }

  function isSegment(raw) {
    return SEGMENT_EXTENSIONS.has(extension(raw));
  }

  // Authentication values remain on the MediaItem URL, but never participate in
  // identity. A refreshed signed URL therefore updates one card instead of
  // flooding the detector with duplicates.
  function identity(raw) {
    const url = parsedURL(raw);
    if (!url) return '';
    url.hash = '';
    for (const key of [...url.searchParams.keys()]) {
      if (TRANSIENT_QUERY.test(key)) url.searchParams.delete(key);
    }
    url.hostname = url.hostname.toLowerCase();
    return url.href;
  }

  function positiveNumber(value) {
    const number = Number(value);
    return Number.isFinite(number) && number > 0 ? number : 0;
  }

  function infer(raw, metadata = {}) {
    const url = parsedURL(raw);
    const text = `${url?.pathname || ''} ${url?.search || ''}`;
    const dimensions = text.match(/(?:^|[^0-9])(\d{3,5})[xX](\d{3,5})(?:[^0-9]|$)/);
    const heightLabel = text.match(/(?:^|[^0-9])(\d{3,4})p(?:[^0-9]|$)/i);
    const bitrateLabel = text.match(/(?:^|[^0-9])(\d{2,6})\s*k(?:bps)?(?:[^a-z]|$)/i);
    return {
      width: positiveNumber(metadata.width) || positiveNumber(dimensions?.[1]),
      height: positiveNumber(metadata.height) || positiveNumber(dimensions?.[2]) || positiveNumber(heightLabel?.[1]),
      bitrate: positiveNumber(metadata.bitrate) || positiveNumber(bitrateLabel?.[1]) * 1000,
      size: positiveNumber(metadata.size),
      sizeExact: Boolean(metadata.sizeExact)
    };
  }

  function merge(previous = {}, current = {}) {
    const merged = {...previous, ...current};
    for (const key of ['width', 'height', 'bitrate', 'sizeBytes']) {
      merged[key] = positiveNumber(current[key]) || positiveNumber(previous[key]);
    }
    merged.sizeExact = Boolean(current.sizeExact || previous.sizeExact);
    merged.protected = Boolean(current.protected || previous.protected);
    if (merged.protected) merged.directSafe = false;
    merged.firstSeen = positiveNumber(previous.firstSeen) || positiveNumber(current.firstSeen) || Date.now();
    return merged;
  }

  const api = Object.freeze({parsedURL, extension, classify, isSegment, identity, infer, merge});
  globalThis.LocalToolboxMediaDetection = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})();
