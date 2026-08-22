importScripts('contracts.js', 'media-detection.js');
const {Protocol, JobState, NativeCommand, NativeEvent, nativeRequest, progressEvent, job:contractJob, mediaItem} = LocalToolboxContracts;
const MediaDetection = LocalToolboxMediaDetection;
const HOST = 'com.localtoolbox.helper';
const ACTIVE_JOBS_KEY = 'activeJobsV2';
let port = null;
let reconnectTimer = null;
let restoredJobs = false;
const jobs = new Map();
const pendingQuick = new Map();
const detectedByTab = new Map();
const blobByTab = new Set();
const MAX_DETECTED_PER_TAB = 24;

function canonicalJobURL(raw='') {
  try {
    const u = new URL(String(raw).trim());
    u.hash = '';
    for (const key of [...u.searchParams.keys()]) {
      if (/^(utm_|fbclid$|gclid$|si$|feature$)/i.test(key)) u.searchParams.delete(key);
    }
    u.hostname = u.hostname.toLowerCase().replace(/^www\./, '');
    if (u.hostname === 'youtu.be') {
      const id = u.pathname.split('/').filter(Boolean)[0];
      if (id) return `https://youtube.com/watch?v=${encodeURIComponent(id)}`;
    }
    return u.href.replace(/\/$/, '');
  } catch { return String(raw || '').trim(); }
}

function duplicateJob(payload={}) {
  if (!payload.url || payload.retry) return null;
  const key = `${payload.action}|${canonicalJobURL(payload.url)}|${payload.playlist ? 'playlist' : 'single'}`;
  return [...jobs.values()].find(j => !['error','cancelled','complete'].includes(j.event) &&
    `${j.kind}|${canonicalJobURL(j.request?.url || j.url)}|${j.request?.playlist ? 'playlist' : 'single'}` === key) || null;
}

function supportedCookieDomains(rawUrl) {
  try {
    const host = new URL(rawUrl).hostname.toLowerCase().replace(/^www\./,'');
    if (host === 'youtube.com' || host === 'm.youtube.com' || host === 'youtu.be') return ['youtube.com', 'google.com'];
    if (host === 'instagram.com' || host === 'm.instagram.com') return ['instagram.com'];
    if (host === 'facebook.com' || host === 'm.facebook.com' || host === 'web.facebook.com' || host === 'fb.watch') return ['facebook.com'];
    if (host === 'x.com' || host === 'twitter.com' || host === 'mobile.twitter.com') return ['x.com', 'twitter.com'];
  } catch {}
  return [];
}

async function getSiteContext(rawUrl) {
  const domains = supportedCookieDomains(rawUrl);
  const seen = new Map();
  try {
    const exact = await chrome.cookies.getAll({ url: rawUrl });
    for (const c of exact) {
      const key = `${c.domain}|${c.path}|${c.name}`;
      seen.set(key, {
        domain:c.domain, path:c.path || '/', secure:!!c.secure, hostOnly:!!c.hostOnly,
        expirationDate:Number(c.expirationDate)||0, name:c.name, value:c.value
      });
    }
  } catch {}
  for (const domain of domains) {
    try {
      const list = await chrome.cookies.getAll({ domain });
      for (const c of list) {
        const key = `${c.domain}|${c.path}|${c.name}`;
        seen.set(key, {
          domain:c.domain, path:c.path || '/', secure:!!c.secure, hostOnly:!!c.hostOnly,
          expirationDate:Number(c.expirationDate)||0, name:c.name, value:c.value
        });
      }
    } catch {}
  }
  return { cookies:[...seen.values()], userAgent:navigator.userAgent };
}


function mediaExt(rawUrl='') {
  return MediaDetection.extension(rawUrl);
}

function classifyDetected(rawUrl='', contentType='') {
  return MediaDetection.classify(rawUrl, contentType);
}

function normalizedDetectedItem(item={}) {
  try {
    const u = new URL(String(item.url || '').trim());
    if (!/^https?:$/.test(u.protocol)) return null;
    const c = classifyDetected(u.href, item.contentType || '');
    if (!c) return null;
    const inferred = MediaDetection.infer(u.href, item);
    return mediaItem({
      url:u.href, kind:item.kind || c.kind, ext:item.ext || c.ext,
      source:item.source || 'network', contentType:item.contentType || '',
      size:inferred.size, sizeExact:inferred.sizeExact, width:inferred.width, height:inferred.height, bitrate:inferred.bitrate,
      label:item.label || '', pageUrl:item.pageUrl || '', title:item.title || '',
      firstSeen:Number(item.firstSeen)||Date.now(), directSafe:c.kind === 'direct' && !item.protected, protected:!!item.protected
    });
  } catch { return null; }
}

async function updateMediaBadge(tabId) {
  if (!Number.isInteger(tabId) || tabId < 0) return;
  const count = detectedByTab.get(tabId)?.size || 0;
  try {
    await chrome.action.setBadgeText({tabId, text: count ? String(Math.min(count,99)) : ''});
    if (count) {
      await chrome.action.setBadgeBackgroundColor({tabId, color:'#202327'});
      await chrome.action.setTitle({tabId, title:`Local Toolbox — ${count} وسائط مكتشفة`});
    } else {
      await chrome.action.setTitle({tabId, title:'Local Toolbox'});
    }
  } catch {}
}

function recordDetected(tabId, rawItem) {
  if (!Number.isInteger(tabId) || tabId < 0) return;
  const item = normalizedDetectedItem(rawItem);
  if (!item) return;
  const map = detectedByTab.get(tabId) || new Map();
  const key = MediaDetection.identity(item.url);
  const old = map.get(key) || {};
  map.set(key, MediaDetection.merge(old, item));
  while (map.size > MAX_DETECTED_PER_TAB) map.delete(map.keys().next().value);
  detectedByTab.set(tabId,map);
  updateMediaBadge(tabId);
  emit({event:'detected_media_updated', tabId, count:map.size, items:[...map.values()]});
}

function clearDetected(tabId) {
  detectedByTab.delete(tabId);
  blobByTab.delete(tabId);
  updateMediaBadge(tabId);
}

async function scanTabMedia(tabId) {
  if (!Number.isInteger(tabId) || tabId < 0) return;
  try {
    const r = await chrome.tabs.sendMessage(tabId,{target:'local_toolbox_scan_media'});
    for (const item of (r?.items || [])) recordDetected(tabId,item);
  } catch {}
}

chrome.sidePanel.setPanelBehavior({ openPanelOnActionClick: true }).catch(() => {});

function emit(message) {
  chrome.runtime.sendMessage({ source: 'local-toolbox', ...message }).catch(() => {});
}

function safeRetrySpec(payload = {}) {
  const action = payload.action || '';
  const out = { action };
  for (const key of ['url','path','quality','bitrate','languages','referer','mediaType','filename','playlist']) {
    if (payload[key] !== undefined && payload[key] !== null && payload[key] !== '') out[key] = payload[key];
  }
  return out;
}

async function persistJobs() {
  const now = Date.now();
  const list = [...jobs.values()]
    .filter(j => !['complete'].includes(j.event) && now - Number(j.updatedAt || j.createdAt || now) < 24*60*60*1000)
    .slice(-40)
    .map(j => ({...j, request: j.request ? safeRetrySpec(j.request) : undefined}));
  await chrome.storage.local.set({ [ACTIVE_JOBS_KEY]: list });
}

async function restoreJobs() {
  if (restoredJobs) return;
  restoredJobs = true;
  try {
    const data = await chrome.storage.local.get(ACTIVE_JOBS_KEY);
    const list = Array.isArray(data[ACTIVE_JOBS_KEY]) ? data[ACTIVE_JOBS_KEY] : [];
    const now = Date.now();
    for (const j of list) {
      if (!j?.id && !j?.jobId) continue;
      if (now - Number(j.updatedAt || j.createdAt || now) > 24*60*60*1000) continue;
      const id = j.id || j.jobId;
      const restored = {...j, id, jobId:id};
      const age = now - Number(j.updatedAt || j.createdAt || now);
      if (['queued','job_started','progress','cancel_requested'].includes(restored.event) && age > 45000) {
        restored.event = 'error';
        restored.state = JobState.INTERRUPTED;
        restored.message = 'انقطعت المهمة عند إغلاق Chrome أو Local Helper. يمكنك إعادة المحاولة.';
        restored.details = restored.details || 'Recovered stale active job from local storage.';
        restored.progress = Math.min(Number(restored.progress)||0,95);
        restored.updatedAt = now;
      }
      jobs.set(id, restored);
    }
  } catch {}
}

async function addHistory(item) {
  const { history = [] } = await chrome.storage.local.get('history');
  const next = [item, ...history.filter(x => x.id !== item.id)].slice(0, 30);
  await chrome.storage.local.set({ history: next });
  emit({ event: 'history_updated', history: next });
}

function compareVersions(a, b) {
  const pa = String(a||'').replace(/^v/i,'').split(/[.+-]/)[0].split('.').map(x=>Number(x)||0);
  const pb = String(b||'').replace(/^v/i,'').split(/[.+-]/)[0].split('.').map(x=>Number(x)||0);
  const n = Math.max(pa.length,pb.length,3);
  for (let i=0;i<n;i++) { const x=pa[i]||0,y=pb[i]||0; if(x<y)return -1; if(x>y)return 1; }
  return 0;
}

function scheduleReconnect(delay = 1200) {
  clearTimeout(reconnectTimer);
  reconnectTimer = setTimeout(() => { if (!port) connect(); }, delay);
}

async function maybeReloadAfterUpdate(message) {
  if (message?.event === 'update_restarting' && message?.version) {
    await chrome.storage.local.set({ updateTargetVersion: message.version });
    return;
  }
  if (message?.event !== 'pong' || !message?.version) return;
  const { updateTargetVersion = '' } = await chrome.storage.local.get('updateTargetVersion');
  if (updateTargetVersion && compareVersions(message.version, updateTargetVersion) >= 0) {
    await chrome.storage.local.remove('updateTargetVersion');
    emit({ event:'update_ready', version:message.version, message:'تم تثبيت التحديث. جارٍ إعادة تحميل الإضافة…' });
    setTimeout(() => chrome.runtime.reload(), 450);
  }
}

function rememberJobRequest(payload = {}) {
  if (!payload.jobId) return;
  const action = payload.action || '';
  if (!['download_video','download_audio','download_thumbnail','download_subtitles','convert_mp3','download_detected','download_stream','extract_detected_audio'].includes(action)) return;
  const old = jobs.get(payload.jobId) || {};
  jobs.set(payload.jobId, {
    id:payload.jobId,
    jobId:payload.jobId,
    kind:action,
    event:old.event || 'queued',
    progress:Number(old.progress)||0,
    createdAt:old.createdAt || Date.now(),
    updatedAt:Date.now(),
    request:safeRetrySpec(payload),
    ...old
  });
  persistJobs().catch(()=>{});
}

function connect() {
  if (port) return port;
  restoreJobs().catch(()=>{});
  try {
    port = chrome.runtime.connectNative(HOST);
    port.onMessage.addListener(async (message) => {
      maybeReloadAfterUpdate(message).catch(() => {});
      if (message.id && pendingQuick.has(message.id)) {
        const pending = pendingQuick.get(message.id);
        if (message.event === 'media_info') {
          pendingQuick.delete(message.id);
          const jobId = crypto.randomUUID();
          const request = { action:pending.action, url:pending.url, jobId };
          jobs.set(jobId, { id:jobId, jobId, kind:pending.action, event:'queued', url:pending.url, request:safeRetrySpec(request), createdAt:Date.now(), updatedAt:Date.now() });
          persistJobs().catch(()=>{});
          postNative({ ...request, cookies:pending.ctx.cookies || [], userAgent:pending.ctx.userAgent || navigator.userAgent });
          emit({ event:'queued', jobId, kind:pending.action, request:safeRetrySpec(request), message:'تم التحقق من الرابط وإضافة المهمة من القائمة السريعة.' });
        } else if (message.event === 'error') {
          pendingQuick.delete(message.id);
        }
      }
      if (message.jobId) {
        const old = jobs.get(message.jobId) || { id: message.jobId, jobId:message.jobId, kind: message.kind, createdAt: Date.now() };
        const normalized = progressEvent(message);
        const updated = contractJob({ ...old, ...normalized, id:message.jobId, jobId:message.jobId, updatedAt: Date.now() });
        jobs.set(message.jobId, updated);
        await persistJobs().catch(()=>{});
        if (['complete', 'error', 'cancelled'].includes(message.event)) {
          if (message.event === 'complete') {
            await addHistory({
              id: message.jobId,
              kind: message.kind,
              path: message.path || '',
              message: message.message || '',
              elapsedSeconds: Number(message.elapsedSeconds) || 0,
              completedAt: Date.now(),
              request: old.request ? safeRetrySpec(old.request) : undefined
            });
            setTimeout(() => {
              jobs.delete(message.jobId);
              persistJobs().catch(()=>{});
            }, 3500);
          }
        }
      }
      emit(message);
    });
    port.onDisconnect.addListener(() => {
      const message = chrome.runtime.lastError?.message || 'تم إغلاق الاتصال بالـ Local Helper.';
      port = null;
      emit({ event: 'disconnected', message });
      scheduleReconnect(1200);
    });
    clearTimeout(reconnectTimer);
    try { port.postMessage(nativeRequest({ id: crypto.randomUUID(), action:NativeCommand.PING })); } catch {}
    return port;
  } catch (error) {
    emit({ event: 'disconnected', message: error.message });
    scheduleReconnect(1800);
    return null;
  }
}

function postNative(payload) {
  const duplicate = duplicateJob(payload);
  if (duplicate) return {ok:false, error:'هذه المهمة موجودة بالفعل في قائمة الانتظار.'};
  const p = connect();
  if (!p) return { ok: false, error: 'تعذر الاتصال بالـ Local Helper.' };
  rememberJobRequest(payload);
  p.postMessage(nativeRequest({ id: crypto.randomUUID(), ...payload }));
  return { ok: true };
}

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message?.target === 'native') {
    sendResponse(postNative(message.payload || {}));
    return true;
  }
  if (message?.target === 'get_state') {
    (async()=>{
      await restoreJobs();
      const { history = [] } = await chrome.storage.local.get('history');
      sendResponse({ ok: true, jobs: [...jobs.values()], history });
    })().catch(e=>sendResponse({ok:false,error:e.message}));
    return true;
  }
  if (message?.target === 'dismiss_job') {
    const id = String(message.jobId || '');
    if (id) jobs.delete(id);
    persistJobs().then(()=>sendResponse({ok:true})).catch(e=>sendResponse({ok:false,error:e.message}));
    return true;
  }
  if (message?.target === 'media_detector_report') {
    const tabId = sender?.tab?.id;
    if (Number.isInteger(tabId)) {
      for (const item of (message.items || [])) recordDetected(tabId,{...item,pageUrl:message.pageUrl || item.pageUrl,title:message.title || item.title});
    }
    sendResponse({ok:true});
    return true;
  }
  if (message?.target === 'media_detector_blob') {
    if (Number.isInteger(sender?.tab?.id)) blobByTab.add(sender.tab.id);
    sendResponse({ok:true});
    return true;
  }
  if (message?.target === 'get_detected_media') {
    const tabId = Number(message.tabId);
    (async()=>{
      await scanTabMedia(tabId);
      const items = [...(detectedByTab.get(tabId)?.values() || [])];
      sendResponse({ok:true, items, count:items.length, blobDetected:blobByTab.has(tabId)});
    })().catch(e=>sendResponse({ok:false,error:e.message}));
    return true;
  }
  if (message?.target === 'get_site_context') {
    getSiteContext(message.url).then(ctx => sendResponse({ ok:true, ...ctx })).catch(e => sendResponse({ ok:false, error:e.message }));
    return true;
  }
  if (message?.target === 'get_page_preview') {
    const tabId = Number(message.tabId);
    if (!tabId) { sendResponse({ok:false,error:'tab id missing'}); return true; }
    chrome.tabs.sendMessage(tabId, {target:'local_toolbox_page_preview'}).then(r => sendResponse(r || {ok:false})).catch(e => sendResponse({ok:false,error:e.message}));
    return true;
  }
});

async function setupMenus() {
  await chrome.contextMenus.removeAll();
  chrome.contextMenus.create({ id: 'lt-root', title: 'Local Toolbox', contexts: ['page', 'link'] });
  chrome.contextMenus.create({ id: 'lt-video', parentId: 'lt-root', title: 'تحميل الفيديو', contexts: ['page', 'link'] });
  chrome.contextMenus.create({ id: 'lt-mp3', parentId: 'lt-root', title: 'تحميل MP3', contexts: ['page', 'link'] });
  chrome.contextMenus.create({ id: 'lt-thumb', parentId: 'lt-root', title: 'تحميل الصورة المصغرة', contexts: ['page', 'link'] });
  chrome.contextMenus.create({ id: 'lt-subs', parentId: 'lt-root', title: 'تحميل الترجمة', contexts: ['page', 'link'] });
}

chrome.runtime.onInstalled.addListener(setupMenus);

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const map = { 'lt-video': 'download_video', 'lt-mp3': 'download_audio', 'lt-thumb': 'download_thumbnail', 'lt-subs': 'download_subtitles' };
  const action = map[info.menuItemId];
  if (!action) return;
  const url = info.linkUrl || info.pageUrl || tab?.url;
  if (!url || !/^https?:/i.test(url)) return;
  try { if (tab?.id) await chrome.sidePanel.open({ tabId: tab.id }); } catch {}
  const ctx = await getSiteContext(url);
  const preflightId = crypto.randomUUID();
  pendingQuick.set(preflightId, { action, url, ctx });
  postNative({ action:'fetch_info', id:preflightId, url, cookies:ctx.cookies || [], userAgent:ctx.userAgent || navigator.userAgent });
  emit({ event:'quick_check', message:'جارٍ التحقق من الرابط قبل إضافة المهمة…' });
});


try {
  chrome.webRequest.onHeadersReceived.addListener((details) => {
    if (!Number.isInteger(details.tabId) || details.tabId < 0) return;
    const headers = {};
    for (const h of (details.responseHeaders || [])) headers[String(h.name || '').toLowerCase()] = h.value || '';
    const contentType = headers['content-type'] || '';
    const classified = classifyDetected(details.url, contentType);
    if (!classified) return;
    // Avoid noisy byte-range fragments; manifests are kept, final media files are kept.
    if (MediaDetection.isSegment(details.url)) return;
    const contentRange = headers['content-range'] || '';
    const rangeTotal = Number(contentRange.match(/\/(\d+)\s*$/)?.[1]) || 0;
    const contentLength = Number(headers['content-length']) || 0;
    recordDetected(details.tabId, {
      url:details.url, kind:classified.kind, ext:classified.ext, source:'network', contentType,
      size:rangeTotal || contentLength, sizeExact:!!rangeTotal || (!!contentLength && !contentRange)
    });
  }, {urls:['http://*/*','https://*/*']}, ['responseHeaders']);
} catch {}

chrome.tabs.onUpdated.addListener((tabId, changeInfo) => {
  if (changeInfo.status === 'loading' || changeInfo.url) clearDetected(tabId);
  if (changeInfo.status === 'complete') setTimeout(()=>scanTabMedia(tabId),350);
});
chrome.tabs.onRemoved.addListener(tabId => clearDetected(tabId));

restoreJobs().finally(()=>connect());
