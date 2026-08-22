(() => {
  const {mediaItem} = LocalToolboxContracts;
  const DIRECT_EXTS = new Set(['mp4','webm','mov','m4v','mp3','m4a','aac','ogg','opus','wav','flac']);
  const STREAM_EXTS = new Set(['m3u8','mpd']);
  const seen = new Map();
  let sendTimer = null;

  function normalize(raw) {
    try {
      const u = new URL(String(raw || '').trim(), location.href);
      if (!/^https?:$/.test(u.protocol)) return '';
      return u.href;
    } catch { return ''; }
  }

  function canonical(raw) {
    try {
      const u = new URL(raw);
      u.hash = '';
      for (const key of [...u.searchParams.keys()]) {
        if (/^(utm_|fbclid$|gclid$|token$|expires$|signature$|policy$|key-pair-id$)/i.test(key)) u.searchParams.delete(key);
      }
      return u.href;
    } catch { return raw; }
  }

  function extOf(raw) {
    try {
      const p = new URL(raw).pathname.toLowerCase();
      const m = p.match(/\.([a-z0-9]{2,5})$/i);
      return m ? m[1] : '';
    } catch { return ''; }
  }

  function classify(url, contentType='') {
    const ext = extOf(url);
    const ct = String(contentType || '').toLowerCase();
    if (STREAM_EXTS.has(ext) || ct.includes('mpegurl')) return {kind:'hls', ext:ext || 'm3u8'};
    if (ext === 'mpd' || ct.includes('dash+xml')) return {kind:'dash', ext:'mpd'};
    if (DIRECT_EXTS.has(ext)) return {kind:'direct', ext};
    if (ct.startsWith('video/')) return {kind:'direct', ext:ct.split('/')[1].split(';')[0] || 'video'};
    if (ct.startsWith('audio/')) return {kind:'direct', ext:ct.split('/')[1].split(';')[0] || 'audio'};
    return null;
  }

  function queueSend() {
    clearTimeout(sendTimer);
    sendTimer = setTimeout(() => {
      const items = [...seen.values()].slice(-30);
      chrome.runtime.sendMessage({target:'media_detector_report', pageUrl:location.href, title:document.title, items}).catch(()=>{});
    }, 160);
  }

  function add(raw, meta={}) {
    if (String(raw || '').startsWith('blob:')) {
      chrome.runtime.sendMessage({target:'media_detector_blob', pageUrl:location.href}).catch(()=>{});
      return;
    }
    const url = normalize(raw);
    if (!url) return;
    const c = classify(url, meta.contentType || '');
    if (!c) return;
    if (/\.(?:m4s|ts|cmfv|cmfa)(?:$|\?)/i.test(url)) return;
    if (Number(meta.size) > 0 && Number(meta.size) < 32768 && c.kind === 'direct') return;
    const key = canonical(url);
    const old = seen.get(key) || {};
    const item = mediaItem({
      url,
      kind:c.kind,
      ext:c.ext,
      source:meta.source || old.source || 'page',
      contentType:meta.contentType || old.contentType || '',
      width:Number(meta.width || old.width || 0),
      height:Number(meta.height || old.height || 0),
      label:meta.label || old.label || '',
      pageUrl:location.href,
      title:document.title,
      firstSeen:old.firstSeen || Date.now(), directSafe:c.kind === 'direct', protected:false
    });
    seen.set(key,item);
    queueSend();
  }

  function scanDOM() {
    document.querySelectorAll('video,audio,source').forEach(el => {
      const tag = el.tagName.toLowerCase();
      const src = el.currentSrc || el.src || el.getAttribute('src');
      if (src) add(src,{source:`dom:${tag}`, width:el.videoWidth||0, height:el.videoHeight||0});
      if (tag === 'video') {
        el.querySelectorAll('source[src]').forEach(s=>add(s.src || s.getAttribute('src'),{source:'dom:source',contentType:s.type||'',width:el.videoWidth||0,height:el.videoHeight||0}));
      }
    });
    const metaSelectors = [
      ['meta[property="og:video"]','og:video'],['meta[property="og:video:url"]','og:video'],
      ['meta[property="og:audio"]','og:audio'],['meta[property="og:audio:url"]','og:audio'],
      ['meta[name="twitter:player:stream"]','twitter:stream']
    ];
    for (const [sel,label] of metaSelectors) {
      document.querySelectorAll(sel).forEach(m=>add(m.content,{source:'meta',label}));
    }
  }

  function scanPerformance() {
    try {
      performance.getEntriesByType('resource').slice(-600).forEach(r=>add(r.name,{source:'performance'}));
    } catch {}
  }

  scanDOM();
  scanPerformance();

  try {
    const po = new PerformanceObserver(list => {
      for (const e of list.getEntries()) add(e.name,{source:'performance'});
    });
    po.observe({type:'resource', buffered:true});
  } catch {}

  const mo = new MutationObserver(() => {
    clearTimeout(mo._t);
    mo._t = setTimeout(scanDOM,250);
  });
  try { mo.observe(document.documentElement,{subtree:true,childList:true,attributes:true,attributeFilter:['src']}); } catch {}

  chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
    if (message?.target === 'local_toolbox_scan_media') {
      scanDOM(); scanPerformance();
      sendResponse({ok:true, items:[...seen.values()].slice(-30)});
      return true;
    }
  });
})();
