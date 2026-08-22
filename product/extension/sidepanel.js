const $ = (s) => document.querySelector(s);
const $$ = (s) => [...document.querySelectorAll(s)];
const {Protocol, JobState, NativeCommand, NativeEvent, job:contractJob, downloadRequest} = LocalToolboxContracts;
const {jobPresentation} = LocalToolboxUILogic;

const state = {
  connected: false,
  helperVersion: '',
  settings: {
    outputDir: '', defaultVideoQuality: '1080', defaultAudioBitrate: 192,
    subtitleLanguages: 'ar,en,tr', browserSession: 'auto', openFolderOnComplete: false, forceIPv4: true,
    maxConcurrentDownloads: 2, maxConcurrentProcessing: 1, concurrentFragments: 4,
    updateManifestUrl: 'https://raw.githubusercontent.com/Tamer723/local-toolbox-updates/main/latest.json',
    autoCheckUpdates: true, autoInstallUpdates: false
  },
  tools: null,
  update: null,
  updateBusy: false,
  capabilities: {},
  selectedFile: '',
  jobs: new Map(),
  history: [],
  currentUrl: '',
  validatedUrl: '',
  urlEligibility: { ok: false, platform: '', normalized: '', reason: '' },
  siteContext: null,
  pendingPreflights: new Map(),
  activeTabId: 0,
  detectedMedia: []
};

const kindLabels = {
  download_video: 'تحميل فيديو', download_audio: 'تحميل MP3', download_thumbnail: 'الصورة المصغرة',
  download_subtitles: 'الترجمة', convert_mp3: 'تحويل إلى MP3',
  download_detected: 'تنزيل مباشر', download_stream: 'تنزيل Stream', extract_detected_audio: 'استخراج MP3'
};

function classifyMediaUrl(raw) {
  try {
    const u = new URL(String(raw || '').trim());
    if (!/^https?:$/.test(u.protocol)) return { ok:false, reason:'الرابط ليس HTTP/HTTPS.' };
    const host = u.hostname.toLowerCase().replace(/^www\./,'');
    const path = u.pathname;
    if (host === 'youtu.be') {
      const id = path.split('/').filter(Boolean)[0];
      return id ? {ok:true, platform:'YouTube', normalized:`https://www.youtube.com/watch?v=${encodeURIComponent(id)}`} : {ok:false, reason:'هذا ليس رابط فيديو YouTube مباشرًا.'};
    }
    if (host === 'youtube.com' || host === 'm.youtube.com') {
      let id = '';
      if (path === '/watch') id = u.searchParams.get('v') || '';
      else {
        const m = path.match(/^\/(?:shorts|live)\/([^/?#]+)/i);
        if (m) id = m[1];
      }
      return id ? {ok:true, platform:'YouTube', normalized:`https://www.youtube.com/watch?v=${encodeURIComponent(id)}`} : {ok:false, reason:'صفحة YouTube الحالية ليست فيديو أو Shorts مباشرًا.'};
    }
    if (host === 'instagram.com' || host === 'm.instagram.com') {
      if (/^\/(?:reel|reels|p|tv)\//i.test(path)) return {ok:true, platform:'Instagram', normalized:u.href};
      return {ok:false, reason:'افتح Reel أو منشور Instagram مباشرًا أولًا.'};
    }
    if (host === 'x.com' || host === 'twitter.com' || host === 'mobile.twitter.com') {
      if (/\/status\/\d+/i.test(path)) return {ok:true, platform:'X', normalized:u.href};
      return {ok:false, reason:'افتح منشور X الذي يحتوي الوسائط مباشرة.'};
    }
    if (host === 'fb.watch') return path && path !== '/' ? {ok:true, platform:'Facebook', normalized:u.href} : {ok:false, reason:'افتح فيديو Facebook مباشرًا أولًا.'};
    if (host === 'facebook.com' || host === 'm.facebook.com' || host === 'web.facebook.com') {
      const looksMedia = /\/(?:reel|watch|videos|share\/v|share\/r)\b/i.test(path) || u.searchParams.has('v');
      return looksMedia ? {ok:true, platform:'Facebook', normalized:u.href} : {ok:false, reason:'افتح فيديو أو Reel Facebook مباشرًا أولًا.'};
    }
    return {ok:false, reason:'الموقع غير مدعوم حاليًا. المواقع المدعومة: YouTube وFacebook وInstagram وX.'};
  } catch {
    return { ok:false, reason:'أدخل رابطًا صحيحًا.' };
  }
}

function evaluateUrl(raw = currentUrl()) {
  const e = classifyMediaUrl(raw);
  state.urlEligibility = e;
  if (!e.ok || state.validatedUrl !== e.normalized) {
    state.validatedUrl = '';
    state.siteContext = null;
    renderMediaInfo(null);
  }
  const el = $('#urlStatus');
  if (e.ok) {
    el.className = 'url-status ok';
    el.textContent = `${e.platform} — معاينة فورية من الصفحة. سيتم التحقق الكامل عند بدء التنزيل.`;
  } else {
    el.className = raw ? 'url-status bad' : 'url-status neutral';
    el.textContent = raw ? e.reason : 'ألصق رابط فيديو من YouTube أو Facebook أو Instagram أو X.';
  }
  $('#inspectUrl').disabled = !e.ok;
  updateCapabilities();
  return e;
}

async function getSiteContext(url) {
  if (state.settings.browserSession !== 'auto') return { cookies: [], userAgent: navigator.userAgent };
  try {
    const r = await chrome.runtime.sendMessage({ target:'get_site_context', url });
    return r?.ok ? { cookies:r.cookies || [], userAgent:r.userAgent || navigator.userAgent } : { cookies:[], userAgent:navigator.userAgent };
  } catch {
    return { cookies:[], userAgent:navigator.userAgent };
  }
}

async function preflight(url, onSuccess = null, force = false) {
  const e = evaluateUrl(url);
  if (!e.ok) { toast(e.reason, 'bad'); return false; }
  const requestId = crypto.randomUUID();
  const ctx = await getSiteContext(e.normalized);
  state.pendingPreflights.set(requestId, { url:e.normalized, onSuccess, ctx });
  $('#inspectUrl').textContent = '…';
  const r = await native('fetch_info', { id:requestId, url:e.normalized, force, cookies:ctx.cookies, userAgent:ctx.userAgent });
  if (!r?.ok) {
    state.pendingPreflights.delete(requestId);
    $('#inspectUrl').textContent = 'تحقق';
    toast(r?.error || 'تعذر بدء فحص الرابط', 'bad');
    return false;
  }
  return true;
}

async function startMediaAction(action, extra = {}) {
  const e = evaluateUrl(currentUrl());
  if (!e.ok) return toast(e.reason, 'bad');
  const launch = async () => {
    const ctx = state.siteContext || await getSiteContext(e.normalized);
    startJob(action, { url:e.normalized, cookies:ctx.cookies || [], userAgent:ctx.userAgent || navigator.userAgent, ...extra });
  };
  if (state.validatedUrl === e.normalized) return launch();
  toast('جارٍ التحقق من الرابط قبل بدء المهمة…');
  await preflight(e.normalized, launch);
}

function batchURLs(raw) {
  const unique = new Map();
  for (const line of String(raw || '').split(/\r?\n/)) {
    try {
      const u = new URL(line.trim());
      if (!/^https?:$/.test(u.protocol)) continue;
      u.hash = '';
      unique.set(u.href, u.href);
    } catch {}
  }
  return [...unique.values()];
}

async function enqueueBatch() {
  const urls = batchURLs($('#batchUrls').value);
  if (!urls.length) return toast('أدخل رابط HTTP/HTTPS واحدًا على الأقل.', 'bad');
  const playlist = $('#batchPlaylist').checked;
  let queued = 0;
  for (const url of urls) {
    const ctx = await getSiteContext(url);
    const id = crypto.randomUUID();
    const request = {action:'download_video', url, playlist};
    state.jobs.set(id,{id,jobId:id,kind:'download_video',event:'queued',message:'في قائمة الانتظار',request,createdAt:Date.now()});
    const result = await native('download_video',{jobId:id,url,playlist,quality:state.settings.defaultVideoQuality,cookies:ctx.cookies,userAgent:ctx.userAgent});
    if (result?.ok) queued++; else state.jobs.set(id,contractJob({...state.jobs.get(id),event:'error',state:JobState.FAILED,message:result?.error || 'تعذر إضافة المهمة'}));
  }
  renderJobs();
  $('#batchStatus').textContent = `تمت إضافة ${queued} من ${urls.length} مهمة.`;
  toast(`تمت إضافة ${queued} مهمة`, queued ? 'ok' : 'bad');
}

function toast(text, type = '') {
  const el = $('#toast');
  el.textContent = text;
  el.className = `toast ${type}`.trim();
  clearTimeout(toast._t);
  toast._t = setTimeout(() => el.classList.add('hidden'), 2800);
}

function setConnection(ok, text) {
  state.connected = ok;
  $('#helperDot').className = `dot ${ok ? 'ok' : 'bad'}`;
  $('#helperText').textContent = text;
  const extVersion = chrome.runtime.getManifest().version;
  $('#connectionDetails').textContent = `${text}${state.helperVersion ? `\nHelper: ${state.helperVersion}` : ''}\nExtension: ${extVersion}`;
}

async function native(action, extra = {}) {
  try {
    return await chrome.runtime.sendMessage({ target: 'native', payload: { action, ...extra } });
  } catch (e) {
    return { ok: false, error: e.message };
  }
}

async function loadState() {
  const r = await chrome.runtime.sendMessage({ target: 'get_state' }).catch(() => null);
  if (r?.ok) {
    state.history = r.history || [];
    state.jobs = new Map((r.jobs || []).map(j => { const normalized=contractJob(j); return [normalized.id,normalized]; }));
    renderJobs(); renderHistory();
  }
}

async function ping() {
  $('#helperDot').className = 'dot pending';
  $('#helperText').textContent = 'جارٍ الاتصال…';
  const r = await native('ping');
  if (!r?.ok) setConnection(false, r?.error || 'فشل الاتصال');
}

function cleanPageTitle(title = '', platform = '') {
  let t = String(title || '').trim();
  if (!t) return '';
  if (platform === 'YouTube') t = t.replace(/\s*-\s*YouTube\s*$/i, '').trim();
  if (platform === 'X') t = t.replace(/\s*\/\s*X\s*$/i, '').trim();
  return t;
}

function youtubeThumbFromNormalized(normalized = '') {
  try {
    const u = new URL(normalized);
    const id = u.searchParams.get('v') || '';
    return id ? `https://i.ytimg.com/vi/${encodeURIComponent(id)}/hqdefault.jpg` : '';
  } catch { return ''; }
}

function quickPreviewFromTab(tab, eligibility) {
  if (!tab || !eligibility?.ok) return null;
  return {
    title: cleanPageTitle(tab.title || '', eligibility.platform) || `${eligibility.platform} media`,
    thumbnail: eligibility.platform === 'YouTube' ? youtubeThumbFromNormalized(eligibility.normalized) : '',
    uploader: '',
    site: eligibility.platform,
    duration: 0,
    url: eligibility.normalized,
    instant: true
  };
}

async function hydratePagePreview(tab, eligibility) {
  if (!tab?.id || !eligibility?.ok) return;
  const expected = eligibility.normalized;
  try {
    const r = await chrome.runtime.sendMessage({ target:'get_page_preview', tabId:tab.id });
    if (!r?.ok || state.urlEligibility?.normalized !== expected) return;
    const base = quickPreviewFromTab(tab, eligibility) || {};
    const info = { ...base, ...(r.info || {}), site:eligibility.platform, url:expected, instant:true };
    if (!info.thumbnail && eligibility.platform === 'YouTube') info.thumbnail = youtubeThumbFromNormalized(expected);
    renderMediaInfo(info);
  } catch {}
}

function showInstantPreview(tab, eligibility) {
  if (!eligibility?.ok) return;
  const info = quickPreviewFromTab(tab, eligibility);
  if (info) renderMediaInfo(info);
  // Richer metadata comes from the already-open page itself; no yt-dlp, Deno or cookies.
  hydratePagePreview(tab, eligibility);
}

async function refreshActiveTab() {
  try {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (tab?.url && /^https?:/i.test(tab.url)) {
      state.activeTabId = tab.id || 0;
      state.currentUrl = tab.url;
      $('#urlInput').value = tab.url;
      const e = evaluateUrl(tab.url);
      if (e.ok) showInstantPreview(tab, e);
      loadDetectedMedia(false).catch(()=>{});
      return tab.url;
    }
  } catch {}
  return '';
}

function formatDuration(sec) {
  sec = Math.round(Number(sec) || 0);
  if (!sec) return '';
  const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = sec % 60;
  return h ? `${h}:${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}` : `${m}:${String(s).padStart(2,'0')}`;
}

function formatBytes(v) {
  v = Number(v) || 0;
  if (v <= 0) return '';
  const units = ['B','KB','MB','GB','TB'];
  let i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  const digits = i >= 3 ? 2 : (v >= 100 ? 0 : v >= 10 ? 1 : 2);
  return `${v.toFixed(digits)} ${units[i]}`;
}

function formatSpeed(v) {
  const b = formatBytes(v);
  return b ? `${b}/s` : '';
}

function formatClock(sec) {
  sec = Math.max(0, Math.round(Number(sec) || 0));
  if (!sec) return '';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  return h ? `${h}:${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}` : `${m}:${String(s).padStart(2,'0')}`;
}

function updatePerformanceSummary() {
  const el = $('#performanceSummary');
  if (!el) return;
  const d = Number(state.settings.maxConcurrentDownloads) || 2;
  const p = Number(state.settings.maxConcurrentProcessing) || 1;
  const f = Number(state.settings.concurrentFragments) || 4;
  el.textContent = `${d} تنزيل متزامن • ${p} معالجة محلية • ${f} أجزاء لكل تنزيل`;
}

function updateSettingsUI() {
  const s = state.settings;
  $('#settingsOutputDir').textContent = s.outputDir || '';
  $('#settingQuality').value = s.defaultVideoQuality || '1080';
  $('#settingBitrate').value = String(s.defaultAudioBitrate || 192);
  $('#settingSubs').value = s.subtitleLanguages || 'ar,en,tr';
  $('#settingBrowser').value = ['auto','none'].includes(s.browserSession) ? s.browserSession : 'auto';
  $('#settingAutoOpen').checked = !!s.openFolderOnComplete;
  $('#settingForceIPv4').checked = s.forceIPv4 !== false;
  $('#settingConcurrentDownloads').value = String(s.maxConcurrentDownloads || 2);
  $('#settingConcurrentProcessing').value = String(s.maxConcurrentProcessing || 1);
  $('#settingConcurrentFragments').value = String(s.concurrentFragments || 4);
  $('#settingAutoCheckUpdates').checked = s.autoCheckUpdates !== false;
  $('#settingAutoInstallUpdates').checked = !!s.autoInstallUpdates;
  $('#settingUpdateSource').value = s.updateManifestUrl || 'https://raw.githubusercontent.com/Tamer723/local-toolbox-updates/main/latest.json';
  $('#currentVersionBadge').textContent = chrome.runtime.getManifest().version;
  updatePerformanceSummary();
  const q = s.defaultVideoQuality === 'best' ? 'Best' : `${s.defaultVideoQuality || '1080'}p`;
  $('#defaultQualityChip').textContent = `فيديو ${q}`;
  $('#defaultBitrateChip').textContent = `MP3 ${s.defaultAudioBitrate || 192} kbps`;
  $('#localBitrateChip').textContent = `MP3 ${s.defaultAudioBitrate || 192} kbps`;
  $('#outputSummary').textContent = s.outputDir || 'Downloads\\LocalToolbox';
}

function readSettingsForm() {
  return {
    ...state.settings,
    outputDir: $('#settingsOutputDir').textContent.trim(),
    defaultVideoQuality: $('#settingQuality').value,
    defaultAudioBitrate: Number($('#settingBitrate').value),
    subtitleLanguages: $('#settingSubs').value.trim() || 'ar,en,tr',
    browserSession: $('#settingBrowser').value,
    openFolderOnComplete: $('#settingAutoOpen').checked,
    forceIPv4: $('#settingForceIPv4').checked,
    maxConcurrentDownloads: Number($('#settingConcurrentDownloads').value) || 2,
    maxConcurrentProcessing: Number($('#settingConcurrentProcessing').value) || 1,
    concurrentFragments: Number($('#settingConcurrentFragments').value) || 4,
    updateManifestUrl: $('#settingUpdateSource').value.trim(),
    autoCheckUpdates: $('#settingAutoCheckUpdates').checked,
    autoInstallUpdates: $('#settingAutoInstallUpdates').checked
  };
}

function compareVersionStrings(a='', b='') {
  const pa=String(a).split(/[^0-9]+/).filter(Boolean).map(Number);
  const pb=String(b).split(/[^0-9]+/).filter(Boolean).map(Number);
  const n=Math.max(pa.length,pb.length);
  for(let i=0;i<n;i++){ const x=pa[i]||0, y=pb[i]||0; if(x<y)return -1; if(x>y)return 1; }
  return 0;
}

function renderUpdateStatus(update = state.update, message = '') {
  const box = $('#updateStatusBox');
  const btn = $('#applyUpdate');
  const notes = $('#updateNotes');
  if (!box || !btn || !notes) return;
  if (!update) {
    box.className = 'status-box update-status-box';
    box.textContent = message || 'لم يتم فحص التحديثات بعد.';
    btn.classList.add('hidden');
    notes.classList.add('hidden');
    return;
  }
  state.update = update;
  const current = chrome.runtime.getManifest().version;
  const latest = update.latestVersion || current;
  const available = compareVersionStrings(current, latest) < 0;
  if (available) {
    box.className = 'status-box update-status-box attn';
    box.textContent = message || `يتوفر تحديث ${latest} — الإصدار الحالي ${current}.`;
    btn.textContent = `تحديث الآن إلى ${latest}`;
    btn.classList.remove('hidden');
  } else {
    box.className = 'status-box update-status-box good';
    box.textContent = message || `أنت تستخدم أحدث إصدار (${current}).`;
    btn.classList.add('hidden');
  }
  const list = Array.isArray(update.notes) ? update.notes.filter(Boolean) : [];
  if (list.length) {
    notes.innerHTML = `<ul>${list.map(x=>`<li>${escapeHtml(x)}</li>`).join('')}</ul>`;
    notes.classList.remove('hidden');
  } else notes.classList.add('hidden');
}

function setUpdateProgress(progress = 0, stage = 'تحديث', message = '') {
  const wrap = $('#updateProgressWrap');
  if (!wrap) return;
  const p = Math.max(0, Math.min(100, Number(progress) || 0));
  wrap.classList.remove('hidden');
  $('#updateProgressFill').style.width = `${p}%`;
  $('#updatePercent').textContent = `${Math.round(p)}%`;
  $('#updateStage').textContent = stage || 'تحديث';
  if (message) {
    const box = $('#updateStatusBox');
    box.className = 'status-box update-status-box';
    box.textContent = message;
  }
}

async function checkForUpdates(manual = true) {
  if (!state.connected || state.updateBusy) {
    if (manual && !state.connected) toast('Local Helper غير متصل', 'bad');
    return;
  }
  state.updateBusy = true;
  $('#checkUpdate').disabled = true;
  $('#checkUpdate').textContent = 'جارٍ الفحص…';
  const r = await native('check_update');
  if (!r?.ok) {
    state.updateBusy = false;
    $('#checkUpdate').disabled = false;
    $('#checkUpdate').textContent = 'فحص الآن';
    if (manual) toast(r?.error || 'تعذر بدء فحص التحديث', 'bad');
  }
}

async function installUpdate() {
  if (!state.update?.available || state.updateBusy) return;
  state.updateBusy = true;
  $('#applyUpdate').disabled = true;
  $('#checkUpdate').disabled = true;
  setUpdateProgress(1, 'تحقق', 'جارٍ تجهيز التحديث والتحقق منه…');
  const r = await native('apply_update');
  if (!r?.ok) {
    state.updateBusy = false;
    $('#applyUpdate').disabled = false;
    $('#checkUpdate').disabled = false;
    toast(r?.error || 'تعذر بدء التحديث', 'bad');
  }
}

async function maybeAutoCheckUpdate() {
  if (!state.settings.autoCheckUpdates || !state.connected) return;
  const { lastUpdateCheck = 0 } = await chrome.storage.local.get('lastUpdateCheck');
  if (Date.now() - Number(lastUpdateCheck || 0) < 12 * 60 * 60 * 1000) return;
  await chrome.storage.local.set({ lastUpdateCheck: Date.now() });
  checkForUpdates(false);
}

function renderTools() {
  const el = $('#toolsList');
  const tools = state.tools;
  if (!tools) {
    el.innerHTML = '<div class="empty-state">لم يتم فحص الأدوات بعد.</div>';
    return;
  }
  const items = [
    ['FFmpeg', tools.ffmpeg, true], ['FFprobe', tools.ffprobe, true], ['yt-dlp', tools.ytdlp, true], ['Deno', tools.deno, false]
  ];
  el.innerHTML = items.map(([name, t, required]) => `
    <div class="tool-item">
      <span class="tool-state ${t?.found ? 'ok' : 'warn'}"></span>
      <div><div class="tool-name">${name} — ${t?.found ? 'جاهز' : (required ? 'غير موجود' : 'موصى به ليوتيوب')}</div>
      <div class="tool-path">${escapeHtml(t?.path || 'لم يتم العثور على المسار')}</div></div>
    </div>`).join('');
  updateCapabilities();
}

function updateCapabilities() {
  const y = !!state.tools?.ytdlp?.found;
  const f = !!state.tools?.ffmpeg?.found;
  const p = !!state.tools?.ffprobe?.found;
  const mediaOk = !!state.urlEligibility?.ok;
  $('#downloadVideo').disabled = !(state.connected && y && f && mediaOk);
  $('#downloadAudio').disabled = !(state.connected && y && f && mediaOk);
  $('#downloadThumb').disabled = !(state.connected && y && mediaOk);
  $('#downloadSubs').disabled = !(state.connected && y && mediaOk);
  $('#convertMp3').disabled = !(state.connected && f && p && state.selectedFile);

  const notice = $('#downloadNotice');
  if (!y) {
    notice.textContent = 'yt-dlp غير مثبت بعد. افتح الإعدادات > الأدوات المحلية لتثبيته.';
    notice.classList.remove('hidden');
  } else if (!f) {
    notice.textContent = 'FFmpeg غير جاهز؛ يلزم لدمج الفيديو والصوت وتحويل MP3.';
    notice.classList.remove('hidden');
  } else if (!state.tools?.deno?.found) {
    notice.textContent = 'Deno غير مثبت. بعض روابط YouTube قد تعمل بجودات محدودة حتى تثبيته.';
    notice.classList.remove('hidden');
  } else {
    notice.classList.add('hidden');
  }
}

function escapeHtml(s='') {
  return String(s).replace(/[&<>'"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c]));
}

function renderMediaInfo(info) {
  const p = $('#mediaPreview');
  if (!info) { p.classList.add('hidden'); return; }
  $('#mediaTitle').textContent = info.title || 'وسائط';
  const bits = [info.uploader, info.site, formatDuration(info.duration)].filter(Boolean);
  $('#mediaSub').textContent = bits.join(' • ');
  const img = $('#mediaThumb');
  if (info.thumbnail) { img.src = info.thumbnail; img.style.display = ''; } else img.style.display = 'none';
  p.classList.remove('hidden');
}


function detectedLabel(item) {
  const bits = [];
  if (item.height) bits.push(`${item.height}p`);
  if (item.ext) bits.push(String(item.ext).toUpperCase());
  if (item.kind === 'hls') bits.push('HLS');
  if (item.kind === 'dash') bits.push('DASH');
  if (Number(item.size) > 0) bits.push(formatBytes(item.size));
  return bits.join(' • ') || 'Media';
}

function detectedName(item) {
  try {
    const u = new URL(item.url);
    const base = decodeURIComponent(u.pathname.split('/').filter(Boolean).pop() || 'media');
    return base.length > 72 ? `${base.slice(0,69)}…` : base;
  } catch { return 'media'; }
}

function renderDetectedMedia() {
  const card = $('#detectedMediaCard');
  const list = $('#detectedMediaList');
  const items = state.detectedMedia || [];
  $('#detectedCount').textContent = String(items.length);
  if (!items.length) {
    card.classList.add('hidden');
    list.innerHTML = '';
    return;
  }
  card.classList.remove('hidden');
  list.innerHTML = items.slice(0,16).map((item,i) => {
    const stream = item.kind === 'hls' || item.kind === 'dash';
    const protectedMedia = !!item.protected;
    return `<div class="detected-item">
      <div class="detected-top">
        <div class="detected-main">
          <div class="detected-title" title="${escapeHtml(item.url)}">${escapeHtml(detectedName(item))}</div>
          <div class="detected-meta">
            <span class="media-chip ${stream?'stream':''}">${escapeHtml(detectedLabel(item))}</span>
            <span class="media-chip">${escapeHtml(item.source||'page')}</span>
          </div>
          <div class="detected-source">${protectedMedia ? 'وسائط محمية — التنزيل غير مدعوم' : (stream ? 'يحتاج معالجة وتجميع محلي' : 'رابط ملف مباشر مكتشف من الصفحة/الشبكة')}</div>
        </div>
      </div>
      <div class="detected-actions">
        <button class="primary-tiny" data-detected-download="${i}" ${protectedMedia?'disabled':''}>${stream ? 'معالجة وتنزيل' : 'تنزيل مباشر'}</button>
        <button data-detected-mp3="${i}" ${protectedMedia?'disabled':''}>MP3</button>
      </div>
    </div>`;
  }).join('');
  $$('[data-detected-download]').forEach(b => b.onclick = () => startDetectedAction(Number(b.dataset.detectedDownload), 'download'));
  $$('[data-detected-mp3]').forEach(b => b.onclick = () => startDetectedAction(Number(b.dataset.detectedMp3), 'mp3'));
}

async function loadDetectedMedia(showToast = false) {
  if (!state.activeTabId) return;
  const r = await chrome.runtime.sendMessage({target:'get_detected_media',tabId:state.activeTabId}).catch(()=>null);
  if (r?.ok) {
    state.detectedMedia = r.items || [];
    renderDetectedMedia();
    if (r.blobDetected && !state.detectedMedia.length) toast('اكتُشف Blob محلي؛ ابحث عن مسار الشبكة الأصلي. رابط Blob غير قابل للتنزيل مباشرة.');
    if (showToast) toast(state.detectedMedia.length ? `تم اكتشاف ${state.detectedMedia.length} عنصر وسائط` : 'لم تُكتشف وسائط مباشرة في الصفحة');
  }
}

function filenameHint(item) {
  try {
    const base = decodeURIComponent(new URL(item.url).pathname.split('/').filter(Boolean).pop() || '');
    return base && base.length < 180 ? base : '';
  } catch { return ''; }
}

async function startDetectedAction(index, mode) {
  const item = state.detectedMedia[index];
  if (!item?.url) return toast('عنصر الوسائط لم يعد متاحًا.', 'bad');
  if (item.protected) return toast('هذه الوسائط محمية ولا يدعم Local Toolbox تنزيلها.', 'bad');
  const ctx = state.settings.browserSession === 'auto' ? await getSiteContext(item.url) : {cookies:[],userAgent:navigator.userAgent};
  const common = {
    url:item.url,
    referer:item.pageUrl || state.currentUrl || currentUrl(),
    mediaType:item.kind || 'direct',
    filename:filenameHint(item),
    cookies:ctx.cookies || [],
    userAgent:ctx.userAgent || navigator.userAgent
  };
  if (mode === 'mp3') return startJob('extract_detected_audio',{...common,bitrate:state.settings.defaultAudioBitrate});
  if (item.kind === 'hls' || item.kind === 'dash') return startJob('download_stream',common);
  return startJob('download_detected',common);
}

function currentUrl() {
  return $('#urlInput').value.trim();
}

async function startJob(action, extra = {}) {
  const jobId = crypto.randomUUID();
  const request = { action, ...Object.fromEntries(Object.entries(extra).filter(([k]) => !['cookies','userAgent'].includes(k))) };
  const job = contractJob({ id: jobId, jobId, kind: action, event: 'queued', progress: 0, message: 'في قائمة الانتظار', request, createdAt: Date.now() });
  state.jobs.set(jobId, job); renderJobs();
  toast('بدأت المهمة — يمكنك متابعتها من تبويب المهام');
  const r = await native(action, { jobId, ...extra });
  if (!r?.ok) {
    state.jobs.set(jobId, contractJob({ ...job, event: 'error', state: JobState.FAILED, message: r?.error || 'تعذر بدء المهمة' }));
    renderJobs();
  }
}


async function retryJob(job) {
  const req = job?.request || {};
  const action = req.action || job?.kind || '';
  if (!action) return toast('لا تتوفر معلومات كافية لإعادة هذه المهمة.', 'bad');
  if (action === 'convert_mp3') {
    if (!req.path) return toast('مسار الملف الأصلي غير متوفر.', 'bad');
    return startJob(action, { path:req.path, bitrate:req.bitrate || state.settings.defaultAudioBitrate });
  }
  if (req.url) {
    if (['download_detected','download_stream','extract_detected_audio'].includes(action)) {
      const ctx = state.settings.browserSession === 'auto' ? await getSiteContext(req.url) : {cookies:[],userAgent:navigator.userAgent};
      return startJob(action,{...req,cookies:ctx.cookies||[],userAgent:ctx.userAgent||navigator.userAgent});
    }
    state.currentUrl = req.url;
    $('#urlInput').value = req.url;
    evaluateUrl(req.url);
    const extra = {};
    if (req.quality) extra.quality = req.quality;
    if (req.bitrate) extra.bitrate = req.bitrate;
    if (req.languages) extra.languages = req.languages;
    return startMediaAction(action, extra);
  }
  toast('الرابط الأصلي غير متوفر لإعادة المحاولة.', 'bad');
}

async function dismissJob(jobId) {
  state.jobs.delete(jobId);
  renderJobs();
  await chrome.runtime.sendMessage({target:'dismiss_job', jobId}).catch(()=>null);
}

function stateText(event) {
  return ({ [JobState.QUEUED]:'في قائمة الانتظار', [JobState.ANALYZING]:'جارٍ التحليل', [JobState.DOWNLOADING]:'جارٍ التنزيل', [JobState.PROCESSING]:'جارٍ المعالجة', [JobState.COMPLETED]:'مكتمل', [JobState.FAILED]:'خطأ', [JobState.CANCELLED]:'ملغي', [JobState.INTERRUPTED]:'متوقف وقابل للاستعادة' })[event] || event || 'جاهز';
}

function renderJobs() {
  const el = $('#jobsList');
  const items = [...state.jobs.values()].sort((a,b) => (b.updatedAt||b.createdAt||0)-(a.updatedAt||a.createdAt||0));
  if (!items.length) { el.className='stack-list empty-state'; el.textContent='لا توجد مهام قيد التنفيذ.'; return; }
  el.className='stack-list';
  el.innerHTML = items.map(j => {
    const view = jobPresentation(j);
    const p = view.progress;
    const done = view.terminal;
    const isError = view.failed;
    const isCancelled = view.cancelled;
    const details = j.details || (isError ? j.message : '');
    const status = j.message || stateText(j.state);
    const percentLabel = view.percentLabel;

    const metrics = [];
    if (Number(j.speedBytes) > 0) metrics.push(['السرعة', formatSpeed(j.speedBytes)]);
    else if (Number(j.processingRate) > 0) metrics.push(['المعالجة', `${Number(j.processingRate).toFixed(2)}×`]);

    if (Number(j.downloadedBytes) > 0) {
      const total = Number(j.totalBytes) > 0 ? ` / ${formatBytes(j.totalBytes)}` : '';
      metrics.push(['الحجم', `${formatBytes(j.downloadedBytes)}${total}`]);
    }
    if (Number(j.etaSeconds) > 0 && !done) metrics.push(['المتبقي', formatClock(j.etaSeconds)]);
    if (Number(j.elapsedSeconds) > 0) metrics.push(['المنقضي', formatClock(j.elapsedSeconds)]);

    return `<div class="job-item ${isError ? 'job-error' : ''}">
      <div class="job-head">
        <div class="job-main">
          <div class="job-title">${escapeHtml(kindLabels[j.kind]||j.kind||'مهمة')}</div>
          ${j.stage && !isError ? `<div class="job-stage">${escapeHtml(j.stage)}</div>` : ''}
          <div class="job-state ${isError?'error-text':''}">${escapeHtml(status)}</div>
        </div>
        <div class="job-percent ${isError?'error-text':''}">${escapeHtml(percentLabel)}</div>
      </div>
      ${!isError && !isCancelled ? `<div class="progress-track"><div class="progress-fill" style="width:${p}%"></div></div>` : ''}
      ${metrics.length ? `<div class="job-metrics">${metrics.map(([k,v])=>`<div><span>${escapeHtml(k)}</span><strong dir="ltr">${escapeHtml(v)}</strong></div>`).join('')}</div>` : ''}
      ${details ? `<details class="job-details"><summary>التفاصيل التقنية</summary><pre>${escapeHtml(details)}</pre></details>` : ''}
      <div class="job-actions">
        ${j.path && view.state===JobState.COMPLETED ? `<button class="tiny-btn primary-tiny" data-open="${escapeHtml(j.path)}">فتح مكان الملف</button>`:''}
        ${view.cancellable ? `<button class="tiny-btn" data-cancel="${escapeHtml(j.id||j.jobId)}">إلغاء</button>`:''}
        ${view.retryable ? `<button class="tiny-btn primary-tiny" data-retry="${escapeHtml(j.id||j.jobId)}">إعادة المحاولة</button>`:''}
        ${(isError || isCancelled || view.interrupted) ? `<button class="tiny-btn" data-dismiss="${escapeHtml(j.id||j.jobId)}">إزالة</button>`:''}
      </div>
    </div>`;
  }).join('');
  $$('[data-open]').forEach(b => b.onclick = () => native('open_path',{path:b.dataset.open}));
  $$('[data-cancel]').forEach(b => b.onclick = () => native('cancel_job',{jobId:b.dataset.cancel}));
  $$('[data-retry]').forEach(b => b.onclick = () => retryJob(state.jobs.get(b.dataset.retry)));
  $$('[data-dismiss]').forEach(b => b.onclick = () => dismissJob(b.dataset.dismiss));
}
function renderHistory() {
  const el = $('#historyList');
  if (!state.history.length) { el.className='stack-list empty-state'; el.textContent='لا توجد عمليات مكتملة بعد.'; return; }
  el.className='stack-list';
  el.innerHTML = state.history.slice(0,20).map(h => `<div class="history-item">
    <div class="history-head"><div><div class="history-title">${escapeHtml(kindLabels[h.kind]||h.kind||'مهمة')}</div><div class="job-state">${new Date(h.completedAt||Date.now()).toLocaleString('ar')}${Number(h.elapsedSeconds)>0 ? ` • ${formatClock(h.elapsedSeconds)}` : ''}</div></div></div>
    <div class="file-path">${escapeHtml(h.path||'')}</div>
    <div class="history-actions"><button class="tiny-btn" data-history-open="${escapeHtml(h.path||'')}">فتح مكان الملف</button></div>
  </div>`).join('');
  $$('[data-history-open]').forEach(b => b.onclick=()=>native('open_path',{path:b.dataset.historyOpen}));
}

function switchTab(name) {
  $$('.tab').forEach(b => b.classList.toggle('active', b.dataset.tab === name));
  $$('[data-panel]').forEach(p => p.classList.toggle('active', p.dataset.panel === name));
}

function openSettings() {
  $('#mainView').classList.add('hidden');
  $('#settingsView').classList.remove('hidden');
}
function closeSettings() {
  $('#settingsView').classList.add('hidden');
  $('#mainView').classList.remove('hidden');
}

$$('.tab').forEach(b => b.addEventListener('click', () => switchTab(b.dataset.tab)));
$('#settingsBtn').onclick = openSettings;
$('#backBtn').onclick = closeSettings;
$('#editDefaults').onclick = openSettings;
$('#editAudioDefault').onclick = openSettings;
$('#openOutput').onclick = () => native('open_path');
$('#openOutputTop').onclick = () => native('open_path');
$('#reconnectBtn').onclick = ping;
$('#refreshJobs').onclick = loadState;
$('#refreshDetected').onclick = () => loadDetectedMedia(true);

$('#useCurrent').onclick = async () => { const u=await refreshActiveTab(); if(u && state.urlEligibility.ok) toast(`تم التقاط رابط ${state.urlEligibility.platform}`,'ok'); else if(u) toast(state.urlEligibility.reason,'bad'); else toast('الصفحة الحالية لا تحتوي رابط HTTP قابلًا للاستخدام','bad'); };
$('#urlInput').addEventListener('input', () => { const e=evaluateUrl(currentUrl()); if(e.ok && e.platform==='YouTube') renderMediaInfo({title:'YouTube',thumbnail:youtubeThumbFromNormalized(e.normalized),site:'YouTube',url:e.normalized,instant:true}); });
$('#inspectUrl').onclick = async () => { await preflight(currentUrl(), null, true); };

$('#downloadVideo').onclick = () => startMediaAction('download_video',{quality:state.settings.defaultVideoQuality});
$('#downloadAudio').onclick = () => startMediaAction('download_audio',{bitrate:state.settings.defaultAudioBitrate});
$('#downloadThumb').onclick = () => startMediaAction('download_thumbnail');
$('#downloadSubs').onclick = () => startMediaAction('download_subtitles',{languages:state.settings.subtitleLanguages});
$('#enqueueBatch').onclick = enqueueBatch;

$('#pickFile').onclick = () => native('pick_file');
$('#convertMp3').onclick = () => { if(!state.selectedFile)return; startJob('convert_mp3',{path:state.selectedFile,bitrate:state.settings.defaultAudioBitrate}); };

$('#chooseOutput').onclick = () => native('pick_output_folder');
$('#checkTools').onclick = () => native('check_tools',{force:true});
$('#saveSettings').onclick = async () => {
  const settings=readSettingsForm();
  const r=await native('save_settings',{settings});
  if(!r?.ok) toast(r?.error||'تعذر حفظ الإعدادات','bad');
};

$$('[data-copy]').forEach(b => b.onclick = async () => { await navigator.clipboard.writeText(b.dataset.copy); toast('تم نسخ الأمر','ok'); });

$('#checkUpdate').onclick = () => checkForUpdates(true);
$('#applyUpdate').onclick = installUpdate;

$('#clearHistory').onclick = async () => {
  await chrome.storage.local.set({history:[]}); state.history=[]; renderHistory(); toast('تم مسح السجل','ok');
};

let activeTabRefreshTimer = null;
function scheduleActiveTabRefresh(delay = 120) {
  clearTimeout(activeTabRefreshTimer);
  activeTabRefreshTimer = setTimeout(() => refreshActiveTab(), delay);
}
chrome.tabs.onActivated.addListener(() => scheduleActiveTabRefresh(80));
chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
  if (!tab?.active) return;
  if (!(changeInfo.url || changeInfo.title || changeInfo.status === 'complete')) return;
  // Do not overwrite a manually pasted URL while the user is editing it.
  const typed = currentUrl();
  if (typed && state.currentUrl && typed !== state.currentUrl && !changeInfo.url) return;
  scheduleActiveTabRefresh(changeInfo.url ? 40 : 140);
});

chrome.runtime.onMessage.addListener((m) => {
  if (m?.source !== 'local-toolbox') return;
  if (m.event === 'detected_media_updated') {
    if (!state.activeTabId || Number(m.tabId) === Number(state.activeTabId)) {
      state.detectedMedia = m.items || [];
      renderDetectedMedia();
    }
  } else if (m.event === 'pong') {
    state.helperVersion = m.version || '';
    state.capabilities = m.capabilities || {};
    setConnection(true, 'Local Helper متصل');
    native('get_settings'); native('check_tools');
  } else if (m.event === 'disconnected') {
    setConnection(false, 'الاتصال بالـ Local Helper متوقف'); updateCapabilities();
  } else if (m.event === 'settings') {
    state.settings = { ...state.settings, ...(m.settings||{}) }; updateSettingsUI(); maybeAutoCheckUpdate();
  } else if (m.event === 'settings_saved') {
    state.settings = { ...state.settings, ...(m.settings||{}) }; updateSettingsUI(); closeSettings(); toast('تم حفظ الإعدادات','ok'); native('check_tools');
  } else if (m.event === 'tools_status') {
    state.tools = m.tools || null; renderTools();
  } else if (m.event === 'file_selected') {
    state.selectedFile = m.path || '';
    $('#selectedFileName').textContent = state.selectedFile.split(/[\\/]/).pop() || 'ملف';
    $('#selectedFilePath').textContent = state.selectedFile;
    $('#selectedFile').classList.remove('hidden'); updateCapabilities();
  } else if (m.event === 'folder_selected') {
    $('#settingsOutputDir').textContent = m.path || state.settings.outputDir;
  } else if (m.event === 'path_opened') {
    toast(m.message || 'تم فتح مكان الملف','ok');
  } else if (m.event === 'media_info') {
    $('#inspectUrl').textContent='تحقق';
    const pending = m.id ? state.pendingPreflights.get(m.id) : null;
    if (pending) {
      state.pendingPreflights.delete(m.id);
      state.validatedUrl = pending.url;
      state.siteContext = pending.ctx;
    } else if (m.info?.url) {
      state.validatedUrl = classifyMediaUrl(m.info.url).normalized || m.info.url;
    }
    renderMediaInfo(m.info); updateCapabilities(); toast('تم التحقق من رابط الوسائط','ok');
    if (pending?.onSuccess) Promise.resolve(pending.onSuccess()).catch(()=>{});
  } else if (m.event === 'update_status') {
    state.updateBusy = false;
    $('#checkUpdate').disabled = false; $('#checkUpdate').textContent = 'فحص الآن';
    $('#applyUpdate').disabled = false;
    $('#updateProgressWrap').classList.add('hidden');
    renderUpdateStatus(m.update || null, m.message || '');
    chrome.storage.local.set({ lastUpdateCheck: Date.now() }).catch(()=>{});
    if (m.update?.available && state.settings.autoInstallUpdates) setTimeout(installUpdate, 250);
  } else if (m.event === 'update_progress') {
    state.updateBusy = true;
    setUpdateProgress(m.progress, m.stage, m.message);
  } else if (m.event === 'update_error') {
    state.updateBusy = false;
    $('#checkUpdate').disabled = false; $('#checkUpdate').textContent = 'فحص الآن';
    $('#applyUpdate').disabled = false;
    $('#updateProgressWrap').classList.add('hidden');
    const box = $('#updateStatusBox'); box.className='status-box update-status-box bad'; box.textContent=m.message||'تعذر فحص أو تثبيت التحديث.';
    toast(m.message || 'تعذر التحديث','bad');
  } else if (m.event === 'update_restarting') {
    state.updateBusy = true;
    setUpdateProgress(99, m.stage || 'إعادة تشغيل', m.message || 'جارٍ إعادة تشغيل Local Toolbox…');
    $('#settingsView').classList.add('update-restarting');
    toast('تم التحقق من التحديث. ستعاد الإضافة تلقائيًا…','ok');
  } else if (m.event === 'update_ready') {
    setUpdateProgress(100, 'اكتمل', `تم تثبيت ${m.version || 'الإصدار الجديد'}. جارٍ إعادة تحميل الواجهة…`);
  } else if (['queued','job_started','progress','complete','error','cancelled','cancel_requested'].includes(m.event) && m.jobId) {
    const old = state.jobs.get(m.jobId) || { id:m.jobId, jobId:m.jobId, kind:m.kind, createdAt:Date.now() };
    const next = contractJob({...old,...m,state:m.state || undefined,id:m.jobId,updatedAt:Date.now()});
    if (m.event === 'error') next.progress = Math.min(Number(old.progress)||0, 95);
    if (m.event === 'complete') next.progress = 100;
    state.jobs.set(m.jobId,next); renderJobs();
    if (m.event==='complete') toast('اكتملت المهمة','ok');
    if (m.event==='error') toast(m.message||'حدث خطأ','bad');
    if (['complete','cancelled'].includes(m.event)) setTimeout(()=>{ state.jobs.delete(m.jobId); renderJobs(); }, 3500);
  } else if (m.event === 'history_updated') {
    state.history = m.history || []; renderHistory();
  } else if (m.event === 'error') {
    $('#inspectUrl').textContent='تحقق';
    if (m.id && state.pendingPreflights.has(m.id)) state.pendingPreflights.delete(m.id);
    toast(m.message || 'حدث خطأ','bad');
  }
});

(async function init(){
  $('#currentVersionBadge').textContent = chrome.runtime.getManifest().version;
  updateSettingsUI(); renderUpdateStatus(); renderTools(); await loadState(); await refreshActiveTab(); await ping();
})();
