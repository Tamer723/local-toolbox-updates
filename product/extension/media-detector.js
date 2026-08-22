(() => {
  const {mediaItem} = LocalToolboxContracts;
  const MediaDetection = LocalToolboxMediaDetection;
  const seen = new Map();
  let sendTimer = null;

  function normalize(raw) {
    try {
      return MediaDetection.parsedURL(raw, location.href)?.href || '';
    } catch { return ''; }
  }

  function canonical(raw) {
    return MediaDetection.identity(raw);
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
    const c = MediaDetection.classify(url, meta.contentType || '');
    if (!c) return;
    if (MediaDetection.isSegment(url)) return;
    if (Number(meta.size) > 0 && Number(meta.size) < 32768 && c.kind === 'direct') return;
    const key = canonical(url);
    const old = seen.get(key) || {};
    const inferred = MediaDetection.infer(url, meta);
    const item = mediaItem({
      url,
      kind:c.kind,
      ext:c.ext,
      source:meta.source || old.source || 'page',
      contentType:meta.contentType || old.contentType || '',
      width:inferred.width, height:inferred.height, bitrate:inferred.bitrate,
      size:inferred.size, sizeExact:inferred.sizeExact,
      label:meta.label || old.label || '',
      pageUrl:location.href,
      title:document.title,
      firstSeen:old.firstSeen || Date.now(), directSafe:c.kind === 'direct' && !meta.protected, protected:!!meta.protected
    });
    seen.set(key,MediaDetection.merge(old,item));
    queueSend();
  }

  function scanDOM() {
    document.querySelectorAll('video,audio,source').forEach(el => {
      const tag = el.tagName.toLowerCase();
      const src = el.currentSrc || el.src || el.getAttribute('src');
      const protectedMedia = !!el.mediaKeys;
      if (src) add(src,{source:`dom:${tag}`, width:el.videoWidth||0, height:el.videoHeight||0, protected:protectedMedia});
      if (tag === 'video') {
        el.querySelectorAll('source[src]').forEach(s=>add(s.src || s.getAttribute('src'),{source:'dom:source',contentType:s.type||'',width:el.videoWidth||0,height:el.videoHeight||0,protected:protectedMedia}));
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
