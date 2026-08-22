function ltMeta(selectors, attr = 'content') {
  for (const selector of selectors) {
    const el = document.querySelector(selector);
    if (!el) continue;
    const v = attr === 'textContent' ? el.textContent : el.getAttribute(attr);
    if (v && String(v).trim()) return String(v).trim();
  }
  return '';
}

function ltDurationSeconds(raw) {
  if (!raw) return 0;
  const n = Number(raw);
  if (Number.isFinite(n) && n > 0) return n;
  const m = String(raw).match(/^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?$/i);
  if (!m) return 0;
  return (Number(m[1])||0)*3600 + (Number(m[2])||0)*60 + (Number(m[3])||0);
}

function ltPagePreview() {
  let title = ltMeta([
    'meta[property="og:title"]', 'meta[name="twitter:title"]'
  ]) || document.title || '';
  title = title.replace(/\s*-\s*YouTube\s*$/i, '').trim();

  const thumbnail = ltMeta([
    'meta[property="og:image"]', 'meta[name="twitter:image"]', 'link[itemprop="thumbnailUrl"]'
  ], document.querySelector('link[itemprop="thumbnailUrl"]') ? 'href' : 'content');

  let uploader = ltMeta([
    'meta[name="author"]', 'meta[itemprop="author"]', 'meta[property="article:author"]'
  ]);
  if (!uploader) {
    const channel = document.querySelector('ytd-channel-name a, #owner-name a, a[href*="/channel/"], a[href*="/@"]');
    uploader = channel?.textContent?.trim() || '';
  }

  let duration = ltDurationSeconds(ltMeta([
    'meta[itemprop="duration"]', 'meta[property="video:duration"]'
  ]));
  if (!duration) {
    const video = document.querySelector('video');
    if (video && Number.isFinite(video.duration) && video.duration > 0) duration = video.duration;
  }

  return { title, thumbnail, uploader, duration, pageUrl: location.href };
}

chrome.runtime.onMessage.addListener((m, sender, sendResponse) => {
  if (m?.target !== 'local_toolbox_page_preview') return;
  try { sendResponse({ ok:true, info:ltPagePreview() }); }
  catch (e) { sendResponse({ ok:false, error:e?.message || String(e) }); }
  return true;
});
