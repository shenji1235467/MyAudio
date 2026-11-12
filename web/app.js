/* MyAudio 前端逻辑：音频库浏览、播放器、播放列表管理（原生 JS，无依赖） */
'use strict';

// ---------------- 工具 ----------------
const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) =>
  ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

function fmtDuration(sec) {
  sec = Math.floor(sec || 0);
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const mm = h ? String(m).padStart(2, '0') : String(m);
  return (h ? h + ':' : '') + mm + ':' + String(s).padStart(2, '0');
}

function toast(msg, type = '') {
  const el = $('toast');
  el.textContent = msg;
  el.className = 'toast show ' + type;
  clearTimeout(toast._t);
  toast._t = setTimeout(() => { el.className = 'toast'; }, 2600);
}

// 后端错误 -> 当前语言文案。优先匹配错误码（code），其次兼容历史中文字符串。
function translateApiError(data, status) {
  const code = data && data.code;
  const raw = data && data.error ? data.error : '';
  if (code && ERROR_CODE_KEYS[code]) return t(ERROR_CODE_KEYS[code], { msg: data.msg || '' });
  const legacyKey = LEGACY_ERROR_KEYS[raw];
  if (legacyKey) return t(legacyKey);
  // 带动态后缀的中文错误（如“查询失败: xxx”）无法精确匹配时，直接展示原文
  return raw || ('HTTP ' + status);
}

// 后端结构化错误码（Go 端返回 code 字段）
const ERROR_CODE_KEYS = {
  'TRACK_NOT_FOUND': 'err.trackNotFound',
  'FILE_MISSING': 'err.fileMissing',
  'INVALID_ID': 'err.invalidID',
  'INVALID_TRACK_ID': 'err.invalidTrackID',
  'EMPTY_NAME': 'err.emptyName',
  'NAME_EXISTS': 'pl.nameExists',
  'NOT_FOUND': 'pl.notFound',
  'SCAN_IN_PROGRESS': 'scan.inProgress',
  'BAD_REQUEST': 'login.badRequest',
  'UNAUTHORIZED': 'login.unauthorized',
  'INVALID_PASSWORD': 'login.wrongPassword',
  'TOO_MANY_ATTEMPTS': 'login.tooManyAttempts',
  'QUERY_FAILED': 'err.queryFailed',
  'DIR_QUERY_FAILED': 'err.dirQueryFailed',
  'INVALID_POSITION': 'err.invalidPosition',
};

// 兼容改造前后端直接返回的中文字符串
const LEGACY_ERROR_KEYS = {
  '曲目不存在': 'err.trackNotFound',
  '音频文件不存在或已被移动': 'err.fileMissing',
  '无效的 ID': 'err.invalidID',
  '无效的曲目 ID': 'err.invalidTrackID',
  '名称不能为空': 'err.emptyName',
  '播放列表名称已存在': 'pl.nameExists',
  '记录不存在': 'pl.notFound',
  '扫描任务正在进行中': 'scan.inProgress',
  '请求格式错误': 'login.badRequest',
  '未登录或会话已过期': 'login.unauthorized',
  '密码错误': 'login.wrongPassword',
  '失败次数过多，请 10 分钟后再试': 'login.tooManyAttempts',
};

async function api(path, options = {}) {
  const resp = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  if (resp.status === 401) { location.href = '/login'; throw new Error('unauthorized'); }
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new Error(translateApiError(data, resp.status));
  return data;
}

// 无封面时的 SVG 占位（data URI，零外部依赖）。encodeURIComponent 会编码双引号，
// 保证结果 URI 中既不含单引号也不含双引号，可安全用于 <img src> 与回退赋值。
const COVER_PLACEHOLDER =
  'data:image/svg+xml,' + encodeURIComponent(
    `<svg xmlns="http://www.w3.org/2000/svg" width="300" height="300">
      <defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="#7C5CFF"/><stop offset="1" stop-color="#00B8E6"/>
      </linearGradient></defs>
      <rect width="300" height="300" fill="url(#g)"/>
      <text x="150" y="170" font-size="110" text-anchor="middle" fill="#ffffff">♪</text></svg>`);

function coverUrl(t) {
  return t.has_cover ? `/api/cover/${t.id}` : COVER_PLACEHOLDER;
}

// 全局图片加载失败兜底：封面 404/出错时回退到占位图。
// 用事件委托替代内联 onerror，避免 data URI 引号与内联事件字符串冲突。
document.addEventListener('error', (e) => {
  const img = e.target;
  if (img && img.tagName === 'IMG' && img.classList.contains('cover-fallback') && img.src !== COVER_PLACEHOLDER) {
    img.src = COVER_PLACEHOLDER;
  }
}, true); // 捕获阶段：error 事件不冒泡，只能在捕获阶段监听

// 文件名展示行：仅在文件名与标题不同（存在独立文件名且标签标题不一致）时显示
function fileLine(t) {
  const baseName = t.filename || '';
  if (!baseName) return '';
  const titleWithExt = (t.title || '') + '.' + (t.format || '');
  if (baseName === t.title || baseName === titleWithExt) return '';
  return `<div class="card-file" title="${esc(baseName)}">${esc(baseName)}</div>`;
}

// 支持的倍速档位，最高 3x（须在 state 之前定义：state 初始化会调用 normalizeSpeed）
const SPEEDS = [1, 1.25, 1.5, 2, 2.5, 3];
function normalizeSpeed(v) {
  return SPEEDS.includes(v) ? v : 1;
}
function speedLabel(v) {
  return v + 'x';
}

// 播放模式白名单（同样须前置：state 初始化会调用 normalizeMode）
const MODE_ORDER = ['list', 'loop', 'random'];
function normalizeMode(mode) {
  return MODE_ORDER.includes(mode) ? mode : 'list';
}

// ---------------- 全局状态 ----------------
const state = {
  tracks: [],          // 当前已加载曲目
  total: 0,
  page: 0,
  perPage: 48,
  pages: 1,
  keyword: '',
  // 默认按文件名排序；用户切换后持久化，跨刷新保持（白名单校验防止脏值）
  order: ['created_at', 'filename', 'title', 'artist', 'album'].includes(localStorage.getItem('myaudio_order'))
    ? localStorage.getItem('myaudio_order') : 'filename',
  dir: localStorage.getItem('myaudio_dir') || '', // 当前选中的相对目录（'' = 全部/根目录），跨刷新记忆
  theme: localStorage.getItem('myaudio_theme') === 'dark' ? 'dark' : 'light',
  view: localStorage.getItem('myaudio_view') || 'grid',
  queue: [],           // 播放队列
  queuePos: -1,
  mode: normalizeMode(localStorage.getItem('myaudio_mode')), // list / loop / random
  speed: normalizeSpeed(parseFloat(localStorage.getItem('myaudio_speed'))), // 播放倍速
  currentPlaylistId: null,
  pendingTrackId: null, // “加入播放列表”选择器当前操作的曲目 ID
};

const audio = $('audio');

// ---------------- 服务器端记忆播放位置（跨设备/浏览器同步） ----------------
// 进度保存在后端 SQLite，任意位置登录后都能恢复；前端维护一份内存缓存避免频繁请求。
const POS_SAVE_GAP = 3;        // 播放中最多每 3 秒同步一次，避免频繁请求
const POS_MIN_RESUME = 5;      // 距开头不足 5 秒视为从头播放，不提示续播
const posCache = new Map();    // trackId(Number) -> 秒（Number）
let positionsLoaded = false;   // 是否已从服务器拉取过全量进度
let lastTrackId = 0;           // 服务器记录的“上次播放”曲目 ID（列表首条）

// 启动时拉取全部播放位置，填充内存缓存与上次播放曲目
async function loadPositions() {
  try {
    const list = await api('/api/positions');
    posCache.clear();
    (list || []).forEach((p) => {
      if (p && p.track_id && Number.isFinite(p.position)) posCache.set(Number(p.track_id), p.position);
    });
    lastTrackId = list && list.length ? Number(list[0].track_id) : 0;
    positionsLoaded = true;
  } catch (_) {
    positionsLoaded = false; // 拉取失败时退化为不记忆，不阻断主界面
  }
}
function getSavedPosition(trackId) {
  const pos = posCache.get(Number(trackId));
  return Number.isFinite(pos) && pos > 0 ? pos : 0;
}
// 保存到服务器并同步内存缓存（失败静默：播放体验优先，下次定时保存会重试）
async function savePosition(trackId, sec) {
  if (!trackId || !Number.isFinite(sec) || sec <= 0) return;
  posCache.set(Number(trackId), Math.floor(sec));
  lastTrackId = Number(trackId);
  try {
    await api(`/api/positions/${trackId}`, {
      method: 'PUT',
      body: JSON.stringify({ position: Math.floor(sec) }),
    });
  } catch (_) { /* 网络/权限异常忽略，不打断播放 */ }
}
// 清除服务器上的某曲目进度（完整播完或删除曲目时）
async function clearPosition(trackId) {
  if (!trackId) return;
  posCache.delete(Number(trackId));
  try {
    await api(`/api/positions/${trackId}`, { method: 'DELETE' });
  } catch (_) { /* 忽略 */ }
}
// 待元数据就绪后跳转的续播位置（秒）；0 表示从头播放
let pendingResume = 0;
let lastSavedAt = 0;

// ---------------- 音频库加载与渲染 ----------------
async function loadTracks(reset = true) {
  if (reset) {
    state.page = 0;
    state.tracks = [];
  }
  const params = new URLSearchParams({
    keyword: state.keyword,
    order: state.order,
    dir: state.dir,
    page: String(state.page + 1),
    per_page: String(state.perPage),
  });
  const data = await api('/api/tracks?' + params);
  state.page += 1;
  state.pages = data.pages;
  state.total = data.total;
  state.tracks = reset ? data.items : state.tracks.concat(data.items);
  renderTracks();
  $('loadMoreBtn').style.display = state.page < state.pages ? 'inline-flex' : 'none';
  refreshStats();
}

async function refreshStats() {
  try {
    const s = await api('/api/stats');
    $('statsText').textContent =
      t('library.stats', { count: s.track_count, duration: fmtDuration(s.total_duration) }) +
      (s.scanning ? t('library.scanning') : '');
  } catch (_) { /* 忽略统计失败 */ }
}

function renderTracks() {
  const grid = $('trackGrid');
  $('emptyState').style.display = state.tracks.length === 0 ? 'block' : 'none';
  grid.className = state.view === 'list' ? 'track-grid list-view' : 'track-grid';

  const currentId = state.queuePos >= 0 ? state.queue[state.queuePos]?.id : null;
  grid.innerHTML = state.tracks.map((tr) => {
    const active = tr.id === currentId ? ' active' : '';
    if (state.view === 'list') {
      return `
      <div class="track-card${active}" data-id="${tr.id}">
        <div class="cover-wrap">
          <img class="cover-img cover-fallback" src="${coverUrl(tr)}" alt="${t('common.cover')}">
        </div>
        <div class="card-body">
          <div class="card-title">${esc(tr.title)}</div>
          <div class="card-artist">${esc(tr.artist || t('library.unknownArtist'))}${tr.album ? ' · ' + esc(tr.album) : ''}</div>
          ${fileLine(tr)}
        </div>
        <div class="card-actions">
          <button class="mini-btn" data-act="playlist" data-id="${tr.id}" title="${t('library.addToList')}" aria-label="${t('library.addToList')}">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>
            <span class="mini-btn-text">${t('library.addToList')}</span>
          </button>
          <button class="mini-btn danger" data-act="delete" data-id="${tr.id}" title="${t('common.delete')}" aria-label="${t('common.delete')}">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14"/></svg>
            <span class="mini-btn-text">${t('common.delete')}</span>
          </button>
        </div>
      </div>`;
    }
    return `
    <div class="track-card${active}" data-id="${tr.id}">
      <div class="cover-wrap">
        <img class="cover-img cover-fallback" src="${coverUrl(tr)}" alt="${esc(t('library.coverOf', { title: tr.title }))}">
        <div class="play-overlay"><div class="play-btn-lg">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5.5v13l11-6.5z"/></svg>
        </div></div>
        <span class="badge fmt">${esc(tr.format)}</span>
        <span class="badge dur">${fmtDuration(tr.duration)}</span>
      </div>
      <div class="card-body">
        <div class="card-title">${esc(tr.title)}</div>
        <div class="card-artist">${esc(tr.artist || t('library.unknownArtist'))}</div>
        ${fileLine(tr)}
        <div class="card-actions">
          <button class="mini-btn" data-act="playlist" data-id="${tr.id}">${t('library.addToList')}</button>
          <button class="mini-btn danger" data-act="delete" data-id="${tr.id}">${t('common.delete')}</button>
        </div>
      </div>
    </div>`;
  }).join('');
}

// 事件委托：卡片点击播放 / 加列表 / 删除
$('trackGrid').addEventListener('click', async (e) => {
  const btn = e.target.closest('[data-act]');
  if (btn) {
    e.stopPropagation();
    const id = Number(btn.dataset.id);
    if (btn.dataset.act === 'delete') return deleteTrack(id);
    if (btn.dataset.act === 'playlist') return quickAddToPlaylist(id);
  }
  const card = e.target.closest('.track-card');
  if (!card) return;
  const id = Number(card.dataset.id);
  // 以当前筛选结果作为播放队列
  state.queue = state.tracks.slice();
  playIndex(state.queue.findIndex((t) => t.id === id));
});

async function deleteTrack(id) {
  if (!confirm(t('track.deleteConfirm1'))) return;
  const deleteFile = confirm(t('track.deleteConfirm2'));
  try {
    await api(`/api/tracks/${id}?delete_file=${deleteFile}`, { method: 'DELETE' });
    toast(t('track.deleted'), 'success');
    posCache.delete(Number(id));   // 服务器外键级联已删除进度，这里仅同步本地缓存
    state.tracks = state.tracks.filter((t) => t.id !== id);
    renderTracks();
    refreshStats();
  } catch (err) {
    toast(err.message, 'error');
  }
}

// ---------------- 播放器 ----------------
// 加载某曲目到播放器并填充元信息；resume=true 时尝试恢复到上次播放位置
function loadTrack(tr, resume) {
  audio.src = `/api/stream/${tr.id}`;
  $('npTitle').textContent = tr.title;
  $('npArtist').textContent = tr.artist || t('library.unknownArtist');
  $('npCover').src = coverUrl(tr);
  $('npCover').classList.add('now-playing-cover');
  updateActiveCard(tr.id);
  // 计算续播位置：仅当距开头足够远、且不超过曲目总时长时才跳转（时长在 loadedmetadata 后校验）
  const saved = resume ? getSavedPosition(tr.id) : 0;
  pendingResume = saved >= POS_MIN_RESUME ? saved : 0;
  hideResumeBadge();
}

function playIndex(i) {
  if (i < 0 || i >= state.queue.length) return;
  state.queuePos = i;
  const tr = state.queue[i];
  loadTrack(tr, true);
  audio.play().catch((err) => toast(t('player.playbackFailed', { msg: err.message }), 'error'));
}

// 仅更新当前播放卡片的高亮，避免切歌时全量重建网格导致封面重新请求、滚动抖动
function updateActiveCard(activeId) {
  $('trackGrid').querySelectorAll('.track-card').forEach((card) => {
    card.classList.toggle('active', Number(card.dataset.id) === activeId);
  });
}

function togglePlay() {
  if (state.queuePos < 0) {
    if (state.tracks.length) {
      state.queue = state.tracks.slice();
      playIndex(0);
    } else {
      toast(t('player.libraryEmpty'));
    }
    return;
  }
  audio.paused ? audio.play() : audio.pause();
}

function playNext(auto = false) {
  if (!state.queue.length) return;
  let next;
  if (state.mode === 'random') {
    next = Math.floor(Math.random() * state.queue.length);
  } else if (state.mode === 'loop') {
    next = (state.queuePos + 1) % state.queue.length;
  } else {
    next = state.queuePos + 1;
    if (next >= state.queue.length) {
      if (auto) return; // 列表模式播完即停
      next = 0;
    }
  }
  playIndex(next);
}

function playPrev() {
  if (!state.queue.length) return;
  // 播放超过 3 秒时先回到本曲开头（符合常见播放器习惯）
  if (audio.currentTime > 3) { audio.currentTime = 0; return; }
  playIndex((state.queuePos - 1 + state.queue.length) % state.queue.length);
}

const MODE_ICONS = {
  list: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 6h12M4 12h12M4 18h7"/><circle cx="18" cy="16" r="3"/></svg>',
  loop: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M17 2l4 4-4 4"/><path d="M3 11v-1a4 4 0 0 1 4-4h14"/><path d="M7 22l-4-4 4-4"/><path d="M21 13v1a4 4 0 0 1-4 4H3"/></svg>',
  random: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M16 3h5v5"/><path d="M4 20L21 3"/><path d="M21 16v5h-5"/><path d="M15 15l6 6"/><path d="M4 4l5 5"/></svg>',
};
const MODE_LABEL_KEY = { list: 'mode.list', loop: 'mode.loop', random: 'mode.random' };
const modeLabel = (mode) => t(MODE_LABEL_KEY[mode]);

function setMode(mode, silent = false) {
  state.mode = normalizeMode(mode);
  localStorage.setItem('myaudio_mode', state.mode);
  $('modeIcon').outerHTML = MODE_ICONS[state.mode].replace('<svg ', '<svg id="modeIcon" ');
  $('modeBtn').title = modeLabel(state.mode);
  if (!silent) toast(modeLabel(state.mode));
}

$('playBtn').addEventListener('click', togglePlay);
$('nextBtn').addEventListener('click', () => playNext(false));
$('prevBtn').addEventListener('click', playPrev);
$('modeBtn').addEventListener('click', () => {
  setMode(MODE_ORDER[(MODE_ORDER.indexOf(state.mode) + 1) % MODE_ORDER.length]);
});

// ---------------- 播放倍速（下拉选择，最高 3x，持久化并对当前/后续曲目生效） ----------------
const speedSelect = $('speedSelect');
function setSpeed(v, silent = false) {
  state.speed = normalizeSpeed(v);
  audio.playbackRate = state.speed;
  localStorage.setItem('myaudio_speed', String(state.speed));
  speedSelect.value = String(state.speed);
  speedSelect.classList.toggle('active', state.speed !== 1);
  if (!silent) toast(t('player.speedLabel', { speed: speedLabel(state.speed) }));
}
speedSelect.addEventListener('change', () => setSpeed(parseFloat(speedSelect.value)));

// ---------------- 定时停止播放（原生下拉，样式同倍速；基于目标时间戳，刷新/切标签不漂移） ----------------
const SLEEP_UNTIL_KEY = 'myaudio_sleep_until';
const SLEEP_MIN_KEY = 'myaudio_sleep_min';
const sleepSelect = $('sleepSelect');
// 依据当前语言生成某档位（value=分钟数，0=定时入口）的 option 文案
function sleepOptionLabel(value) {
  const n = Number(value);
  return n === 0 ? t('player.sleepTimer') : t('player.minutes', { n: String(n) });
}
let sleepUntil = 0;          // 停止时刻的绝对时间戳（ms），0 表示未启用
let sleepTimerId = null;     // 倒计时定时器

function sleepRemainingSec() {
  return sleepUntil ? Math.max(0, Math.round((sleepUntil - Date.now()) / 1000)) : 0;
}
function fmtCountdown(sec) {
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  const mm = h ? String(m).padStart(2, '0') : String(m);
  return (h ? h + ':' : '') + mm + ':' + String(s).padStart(2, '0');
}
function stopSleepTimer() {
  if (sleepTimerId) { clearInterval(sleepTimerId); sleepTimerId = null; }
}
// 还原所有 option 文案为当前语言下的初始值（消除倒计时改写）
function resetSleepOptionText() {
  Array.from(sleepSelect.options).forEach((o) => { o.textContent = sleepOptionLabel(o.value); });
}
// 到点：暂停播放并清理定时状态
function fireSleep() {
  stopSleepTimer();
  sleepUntil = 0;
  localStorage.removeItem(SLEEP_UNTIL_KEY);
  localStorage.removeItem(SLEEP_MIN_KEY);
  if (!audio.paused) audio.pause();
  sleepSelect.value = '0';
  resetSleepOptionText();
  sleepSelect.classList.remove('active');
  toast(t('player.sleepEnded'));
}
// 刷新当前选中项上的倒计时；返回是否仍在计时
function renderSleep() {
  if (!sleepUntil) return false;
  const remain = sleepRemainingSec();
  if (remain <= 0) { fireSleep(); return false; }
  // 仅改写当前选中 option 的展示文本，下拉收起时即显示在控件上
  sleepSelect.selectedOptions[0].textContent = fmtCountdown(remain);
  return true;
}
// 启动/重设定时：minutes 为分钟数；0 表示取消
function setSleep(minutes) {
  stopSleepTimer();
  resetSleepOptionText();
  if (minutes <= 0) {
    sleepUntil = 0;
    localStorage.removeItem(SLEEP_UNTIL_KEY);
    localStorage.removeItem(SLEEP_MIN_KEY);
    sleepSelect.value = '0';
    sleepSelect.classList.remove('active');
    toast(t('player.sleepCancelled'));
    return;
  }
  sleepUntil = Date.now() + minutes * 60 * 1000;
  localStorage.setItem(SLEEP_UNTIL_KEY, String(sleepUntil));
  localStorage.setItem(SLEEP_MIN_KEY, String(minutes));
  sleepSelect.value = String(minutes);
  sleepSelect.classList.add('active');
  renderSleep();
  sleepTimerId = setInterval(() => {
    if (!renderSleep()) stopSleepTimer();
  }, 1000);
  toast(t('player.sleepInMinutes', { minutes: minutes }));
}
sleepSelect.addEventListener('change', () => setSleep(Number(sleepSelect.value)));
// 恢复持久化的定时（仅当尚未到期）；已到期则清理，不打断当前状态
(function restoreSleep() {
  const savedUntil = Number(localStorage.getItem(SLEEP_UNTIL_KEY));
  const savedMin = String(localStorage.getItem(SLEEP_MIN_KEY) || '');
  if (!savedUntil) return;
  if (savedUntil <= Date.now()) {
    localStorage.removeItem(SLEEP_UNTIL_KEY);
    localStorage.removeItem(SLEEP_MIN_KEY);
    return;
  }
  sleepUntil = savedUntil;
  // 恢复到原档位（非法值则回落到“定时”）
  const validMins = Array.from(sleepSelect.options).some((o) => o.value === savedMin);
  if (validMins && savedMin !== '0') {
    sleepSelect.value = savedMin;
  }
  sleepSelect.classList.add('active');
  renderSleep();
  sleepTimerId = setInterval(() => {
    if (!renderSleep()) stopSleepTimer();
  }, 1000);
})();

// 部分浏览器在切歌/加载后会重置 playbackRate，在 play 与元数据就绪时再次确保生效
audio.addEventListener('play', () => {
  audio.playbackRate = state.speed;
  $('playIcon').innerHTML = '<path d="M7 5h4v14H7zM13 5h4v14h-4z"/>';
  hideResumeBadge();   // 用户已开始播放，不再展示续播提示
  lastSavedAt = 0;
});
audio.addEventListener('pause', () => {
  $('playIcon').innerHTML = '<path d="M8 5.5v13l11-6.5z"/>';
  persistCurrentPosition();   // 暂停即落盘，保证关闭页面前进度已保存
});
audio.addEventListener('ended', () => {
  const tr = state.queue[state.queuePos];
  if (tr) { posCache.delete(Number(tr.id)); clearPosition(tr.id); }   // 完整播完，清除服务器记忆位置
  playNext(true);
});
audio.addEventListener('loadedmetadata', () => {
  // 部分浏览器加载新源后会重置 playbackRate，这里重新确保倍速生效，并刷新总时长
  audio.playbackRate = state.speed;
  $('durTime').textContent = fmtDuration(audio.duration);
  // 元数据就绪后执行续播跳转（此时才能用总时长校验，避免跳到越界位置）
  if (pendingResume > 0 && Number.isFinite(audio.duration)) {
    if (pendingResume < audio.duration - 2) audio.currentTime = pendingResume;
    pendingResume = 0;
  }
  updateMediaSession();
});
audio.addEventListener('timeupdate', () => {
  updateProgress();
  // 节流落盘：每 POS_SAVE_GAP 秒保存一次当前曲目进度
  if (audio.currentTime - lastSavedAt >= POS_SAVE_GAP) {
    lastSavedAt = audio.currentTime;
    persistCurrentPosition();
  }
});
audio.addEventListener('error', () => {
  if (audio.src) toast(t('player.audioLoadFailed'), 'error');
});

// 计算当前应保存的进度秒；临近结尾或无有效时长时返回 null（不记忆）
function currentSavablePosition() {
  const tr = state.queue[state.queuePos];
  if (!tr || !Number.isFinite(audio.currentTime)) return null;
  if (audio.duration && audio.currentTime >= audio.duration - 2) return null;
  return { trackId: tr.id, sec: audio.currentTime };
}
// 常规保存：节流场景下走异步 PUT
function persistCurrentPosition() {
  const p = currentSavablePosition();
  if (p) savePosition(p.trackId, p.sec);
}
// 页面卸载/切后台：fetch 可能被浏览器中断，优先用 sendBeacon 保证请求发出
function persistOnUnload() {
  const p = currentSavablePosition();
  if (!p) return;
  const sec = Math.floor(p.sec);
  posCache.set(Number(p.trackId), sec);
  lastTrackId = Number(p.trackId);
  const payload = JSON.stringify({ position: sec });
  let delivered = false;
  if (navigator.sendBeacon) {
    try { delivered = navigator.sendBeacon(`/api/positions/${p.trackId}`, new Blob([payload], { type: 'application/json' })); } catch (_) { delivered = false; }
  }
  if (!delivered) {
    // sendBeacon 不可用或被拒：退化为 keepalive fetch（页面关闭后仍尽量完成）
    try {
      fetch(`/api/positions/${p.trackId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: payload,
        keepalive: true,
        credentials: 'same-origin',
      }).catch(() => {});
    } catch (_) { /* 尽力而为 */ }
  }
}
// 页面关闭 / 刷新 / 切后台前再保存一次，最大限度避免进度丢失
window.addEventListener('pagehide', persistOnUnload);
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'hidden') persistOnUnload();
});

// ---------------- 续播徽标 ----------------
function hideResumeBadge() { $('resumeBadge').style.display = 'none'; }
function renderResumeBadge() {
  const tr = state.queue[state.queuePos];
  const badge = $('resumeBadge');
  if (!tr) { hideResumeBadge(); return; }
  const saved = getSavedPosition(tr.id);
  if (saved < POS_MIN_RESUME) { hideResumeBadge(); return; }
  $('resumeBadgeText').textContent = t('player.resumeAt', { time: fmtDuration(saved) });
  badge.style.display = 'inline-flex';
}
// 点击徽标：从上次位置继续播放
$('resumeBadge').addEventListener('click', () => {
  const tr = state.queue[state.queuePos];
  if (!tr) return;
  const saved = getSavedPosition(tr.id);
  if (saved >= POS_MIN_RESUME) {
    if (Number.isFinite(audio.duration) && saved < audio.duration - 2) audio.currentTime = saved;
    else pendingResume = saved;   // 元数据未就绪时暂存，loadedmetadata 中跳转
  }
  hideResumeBadge();
  audio.play().catch((err) => toast(t('player.playbackFailed', { msg: err.message }), 'error'));
});

// 进度条（点击/拖动跳转）
function updateProgress() {
  if (!audio.duration) return;
  const pct = (audio.currentTime / audio.duration) * 100;
  $('progressFill').style.width = pct + '%';
  $('progressThumb').style.left = pct + '%';
  $('mobileFill').style.width = pct + '%';
  $('curTime').textContent = fmtDuration(audio.currentTime);
}
function seekFromEvent(e) {
  const track = $('progressTrack');
  const rect = track.getBoundingClientRect();
  const ratio = Math.min(1, Math.max(0, (e.clientX - rect.left) / rect.width));
  if (audio.duration) audio.currentTime = ratio * audio.duration;
}
let dragging = false;
$('progressTrack').addEventListener('pointerdown', (e) => {
  dragging = true; seekFromEvent(e);
});
window.addEventListener('pointermove', (e) => dragging && seekFromEvent(e));
window.addEventListener('pointerup', () => { dragging = false; });

// 媒体会话 API：手机锁屏/蓝牙线控
function updateMediaSession() {
  if (!('mediaSession' in navigator)) return;
  const tr = state.queue[state.queuePos];
  if (!tr) return;
  const src = coverUrl(tr);
  // 占位图为 SVG，真实封面为图片接口；按来源声明正确 MIME，避免写死 image/jpeg
  const type = src.startsWith('data:') ? 'image/svg+xml' : 'image/jpeg';
  navigator.mediaSession.metadata = new MediaMetadata({
    title: tr.title, artist: tr.artist || t('library.unknownArtist'), album: tr.album || '',
    artwork: [{ src, sizes: '300x300', type }],
  });
}
if ('mediaSession' in navigator) {
  navigator.mediaSession.setActionHandler('play', () => audio.play());
  navigator.mediaSession.setActionHandler('pause', () => audio.pause());
  navigator.mediaSession.setActionHandler('previoustrack', playPrev);
  navigator.mediaSession.setActionHandler('nexttrack', () => playNext(false));
}

// ---------------- 目录侧边栏（由后端扁平目录构建树） ----------------
const DIR_EXPAND_KEY = 'myaudio_dir_expanded';
let rawDirs = [];       // 后端返回的原始扁平目录（缓存，供语言切换时重新构建树）
let dirTreeData = [];   // 最近一次构建的树

// 读取/写入展开的目录路径集合
function loadExpandedSet() {
  try { return new Set(JSON.parse(localStorage.getItem(DIR_EXPAND_KEY) || '[]')); }
  catch (_) { return new Set(); }
}
function saveExpandedSet(set) {
  localStorage.setItem(DIR_EXPAND_KEY, JSON.stringify([...set]));
}
let expandedDirs = loadExpandedSet();

// 由“含曲目的扁平相对路径”补齐中间层级，构建完整目录树
function buildDirTree(dirs) {
  const root = { path: '', name: t('common.rootDir'), count: 0, children: [], hasTracks: false };
  const nodeMap = new Map([['', root]]);
  let rootCount = 0;

  dirs.forEach((d) => {
    if (d.dir === '') { rootCount = d.count; return; }
    const parts = d.dir.split('/');
    let acc = '';
    parts.forEach((part) => {
      const parent = acc;
      acc = acc ? acc + '/' + part : part;
      if (!nodeMap.has(acc)) {
        const node = { path: acc, name: part, count: 0, children: [], hasTracks: false };
        nodeMap.get(parent).children.push(node);
        nodeMap.set(acc, node);
      }
    });
    const leaf = nodeMap.get(d.dir);
    leaf.count = d.count;       // 后端 count 已含该目录及所有子目录
    leaf.hasTracks = true;
  });

  // 中间层级（无直接曲目）的 count = 各直接子节点 count 之和
  const fillCount = (node) => {
    if (!node.children.length) return node.count;
    node.children.forEach(fillCount);
    if (!node.hasTracks) node.count = node.children.reduce((s, c) => s + c.count, 0);
    return node.count;
  };
  fillCount(root);
  root.count = rootCount || root.count;
  // 每层按名称不区分大小写排序，保证稳定直观
  const sortNode = (node) => {
    node.children.sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' }));
    node.children.forEach(sortNode);
  };
  sortNode(root);
  return [root];
}

const FOLDER_SVG = '<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>';
const CARET_SVG = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M9 6l6 6-6 6"/></svg>';

function renderTreeNode(node, depth) {
  const hasChildren = node.children.length > 0;
  const isExpanded = expandedDirs.has(node.path);
  const isActive = node.path === state.dir;
  const caret = hasChildren
    ? `<span class="tree-caret" data-caret="${esc(node.path)}">${CARET_SVG}</span>`
    : `<span class="tree-caret placeholder"></span>`;
  const title = node.path === '' ? t('common.rootDir') : node.path;
  let html = `
  <div class="tree-item${isActive ? ' active' : ''}${isExpanded ? ' expanded' : ''}"
       style="--depth:${depth}" data-dir="${esc(node.path)}" title="${esc(title)}"
       role="treeitem" aria-expanded="${hasChildren ? isExpanded : 'false'}">
    ${caret}
    <span class="tree-folder">${FOLDER_SVG}</span>
    <span class="tree-name">${esc(node.name)}</span>
    <span class="tree-count">${node.count}</span>
  </div>`;
  if (hasChildren && isExpanded) {
    html += `<div class="tree-children" role="group">${node.children.map((c) => renderTreeNode(c, depth + 1)).join('')}</div>`;
  }
  return html;
}

function renderDirTree() {
  $('dirTree').innerHTML = dirTreeData.map((n) => renderTreeNode(n, 0)).join('');
}

async function loadDirs() {
  let dirs;
  try {
    dirs = await api('/api/dirs');
  } catch (err) {
    return; // 目录加载失败不阻断主界面
  }
  if (!dirs.length) { $('dirTree').innerHTML = ''; dirTreeData = []; rawDirs = []; return; }
  rawDirs = dirs;
  dirTreeData = buildDirTree(dirs);
  // 首次访问（无展开偏好记录）时默认展开全部目录；之后以用户手动折叠/展开的记忆为准
  if (localStorage.getItem(DIR_EXPAND_KEY) === null) {
    const collectPaths = (nodes, acc) => {
      nodes.forEach((n) => {
        if (n.children.length) { acc.add(n.path); collectPaths(n.children, acc); }
      });
      return acc;
    };
    expandedDirs = collectPaths(dirTreeData, new Set());
    saveExpandedSet(expandedDirs);
  }
  renderDirTree();
}

// 选中目录：更新高亮并按新目录加载曲目；折叠/展开箭头独立处理
$('dirTree').addEventListener('click', (e) => {
  const caret = e.target.closest('[data-caret]');
  if (caret) {
    e.stopPropagation();
    const path = caret.dataset.caret;
    if (expandedDirs.has(path)) { expandedDirs.delete(path); } else { expandedDirs.add(path); }
    saveExpandedSet(expandedDirs);
    renderDirTree();
    return;
  }
  const item = e.target.closest('.tree-item');
  if (!item) return;
  state.dir = item.dataset.dir || '';
  localStorage.setItem('myaudio_dir', state.dir);
  renderDirTree();      // 更新蓝色激活高亮
  loadTracks(true);     // 按新目录重新加载
  closeSidebar();       // 手机端选择后自动收起抽屉
});

// ---------------- 侧边栏抽屉（手机端） ----------------
function openSidebar() {
  $('sidebar').classList.add('open');
  $('sidebarMask').classList.add('open');
}
function closeSidebar() {
  $('sidebar').classList.remove('open');
  $('sidebarMask').classList.remove('open');
}
$('sidebarToggle').addEventListener('click', openSidebar);
$('sidebarClose').addEventListener('click', closeSidebar);
$('sidebarMask').addEventListener('click', closeSidebar);

// ---------------- 搜索 / 排序 / 视图 ----------------
let searchTimer;
$('searchInput').addEventListener('input', (e) => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => {
    state.keyword = e.target.value.trim();
    loadTracks(true);
  }, 300);
});

$('orderChips').addEventListener('click', (e) => {
  const chip = e.target.closest('.chip');
  if (!chip) return;
  document.querySelectorAll('#orderChips .chip').forEach((c) => c.classList.remove('active'));
  chip.classList.add('active');
  state.order = chip.dataset.order;
  localStorage.setItem('myaudio_order', state.order);
  loadTracks(true);
});

// 根据 state.order 同步排序芯片的激活态（刷新后恢复用户上次选择，默认“文件名”）
function syncOrderChips() {
  document.querySelectorAll('#orderChips .chip').forEach((c) => {
    c.classList.toggle('active', c.dataset.order === state.order);
  });
}
syncOrderChips();

const VIEW_ICONS = {
  grid: '<rect x="3" y="3" width="8" height="8" rx="2"/><rect x="13" y="3" width="8" height="8" rx="2"/><rect x="3" y="13" width="8" height="8" rx="2"/><rect x="13" y="13" width="8" height="8" rx="2"/>',
  list: '<path d="M4 6h16M4 12h16M4 18h16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>',
};

// 同步两个视图按钮的激活态，以及“更多”菜单里视图项（图标/文案提示“下一步切到哪个视图”）
function syncViewToggle() {
  const view = state.view;
  const next = view === 'grid' ? 'list' : 'grid';
  $('gridViewBtn').classList.toggle('active', view === 'grid');
  $('listViewBtn').classList.toggle('active', view === 'list');
  $('moreViewIcon').outerHTML =
    '<svg id="moreViewIcon" width="18" height="18" viewBox="0 0 24 24" fill="' +
    (next === 'grid' ? 'currentColor' : 'none') + '">' + VIEW_ICONS[next] + '</svg>';
  $('moreViewText').textContent = t(next === 'grid' ? 'nav.gridView' : 'nav.listView');
}

function setView(view) {
  state.view = view;
  localStorage.setItem('myaudio_view', view);
  syncViewToggle();
  renderTracks();
}
$('gridViewBtn').addEventListener('click', () => setView('grid'));
$('listViewBtn').addEventListener('click', () => setView('list'));
$('moreView').addEventListener('click', () => { setView(state.view === 'grid' ? 'list' : 'grid'); closeMoreMenu(); });
$('loadMoreBtn').addEventListener('click', () => loadTracks(false));

// ---------------- 主题（明亮 / 暗色，默认明亮） ----------------
const THEME_ICONS = {
  // 当前为明亮时显示“月亮”，点击切到暗色；当前为暗色时显示“太阳”
  light: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>',
  dark: '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>',
};
const THEME_COLOR = { light: '#F4F6FB', dark: '#0B0E17' };

function setTheme(theme) {
  state.theme = theme;
  localStorage.setItem('myaudio_theme', theme);
  document.documentElement.setAttribute('data-theme', theme);
  $('themeIcon').outerHTML = THEME_ICONS[theme].replace('<svg ', '<svg id="themeIcon" ');
  $('moreThemeIcon').outerHTML = THEME_ICONS[theme]
    .replace('<svg ', '<svg id="moreThemeIcon" width="18" height="18" ');
  $('themeBtn').title = theme === 'dark' ? t('theme.toLight') : t('theme.toDark');
  const meta = document.getElementById('metaThemeColor');
  if (meta) meta.setAttribute('content', THEME_COLOR[theme]);
}
$('themeBtn').addEventListener('click', () => {
  setTheme(state.theme === 'dark' ? 'light' : 'dark');
});
$('moreTheme').addEventListener('click', () => {
  setTheme(state.theme === 'dark' ? 'light' : 'dark');
  closeMoreMenu();
});

// ---------------- 顶栏“更多”菜单（窄屏） ----------------
function openMoreMenu() {
  $('moreMenu').classList.add('open');
  $('moreBtn').setAttribute('aria-expanded', 'true');
}
function closeMoreMenu() {
  $('moreMenu').classList.remove('open');
  $('moreBtn').setAttribute('aria-expanded', 'false');
}
$('moreBtn').addEventListener('click', (e) => {
  e.stopPropagation();
  if ($('moreMenu').classList.contains('open')) closeMoreMenu(); else openMoreMenu();
});
$('moreMenu').addEventListener('click', (e) => e.stopPropagation());
document.addEventListener('click', closeMoreMenu);
document.addEventListener('keydown', (e) => { if (e.key === 'Escape') closeMoreMenu(); });
$('moreLang').addEventListener('click', () => { I18N.toggleLang(); closeMoreMenu(); });
$('moreScan').addEventListener('click', () => { closeMoreMenu(); $('scanBtn').click(); });

// 手动扫描（前端防重入，避免并发轮询叠加）
let scanPolling = false;
$('scanBtn').addEventListener('click', async () => {
  if (scanPolling) return;
  try {
    await api('/api/scan', { method: 'POST' });
    toast(t('scan.started'), 'success');
    $('scanIcon').classList.add('spin');
    $('moreScanIcon').classList.add('spin');
    pollScan();
  } catch (err) {
    toast(err.message, 'error');
  }
});
async function pollScan() {
  scanPolling = true;
  try {
    for (let i = 0; i < 30; i++) {
      await new Promise((r) => setTimeout(r, 2000));
      const s = await api('/api/stats').catch(() => null);
      if (s && !s.scanning) {
        toast(t('scan.done'), 'success');
        loadTracks(true).catch((err) => toast(err.message, 'error'));
        loadDirs().catch(() => {});
        loadPlaylists().catch(() => {});
        return;
      }
    }
    toast(t('scan.timeout'), 'error');
  } finally {
    scanPolling = false;
    $('scanIcon').classList.remove('spin');
    $('moreScanIcon').classList.remove('spin');
  }
}

// 退出登录
$('logoutBtn').addEventListener('click', async () => {
  if (!confirm(t('logout.confirm'))) return;
  await api('/api/auth/logout', { method: 'POST' }).catch(() => {});
  location.href = '/login';
});

// ---------------- 播放列表抽屉 ----------------
const drawer = $('drawer');
function openDrawer() {
  drawer.classList.add('open');
  $('drawerMask').classList.add('open');
  loadPlaylists();
}
function closeDrawer() {
  drawer.classList.remove('open');
  $('drawerMask').classList.remove('open');
}
$('drawerBtn').addEventListener('click', openDrawer);
$('drawerClose').addEventListener('click', closeDrawer);
$('drawerMask').addEventListener('click', closeDrawer);

async function loadPlaylists() {
  const pls = await api('/api/playlists');
  $('playlistList').innerHTML = pls.length ? pls.map((p) => `
    <div class="pl-row ${p.id === state.currentPlaylistId ? 'active' : ''}" data-pl="${p.id}">
      <div class="pl-icon"><svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 21s-7-4.6-9.5-9A5.5 5.5 0 0 1 12 6.2 5.5 5.5 0 0 1 21.5 12C19 16.4 12 21 12 21z"/></svg></div>
      <div style="flex:1;min-width:0">
        <div class="pl-name">${esc(p.name)}</div>
        <div class="pl-count">${t('pl.trackCount', { count: p.track_count })}</div>
      </div>
      <button class="row-remove" data-pl-del="${p.id}" title="${t('pl.deleteList')}">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14"/></svg>
      </button>
    </div>`).join('')
    : `<p class="text-secondary" style="font-size:13px;padding:8px 4px">${t('pl.empty')}</p>`;

  if (state.currentPlaylistId) renderPlaylistDetail(state.currentPlaylistId);
}

$('playlistList').addEventListener('click', async (e) => {
  const del = e.target.closest('[data-pl-del]');
  if (del) {
    e.stopPropagation();
    const id = Number(del.dataset.plDel);
    if (!confirm(t('pl.deleteConfirm'))) return;
    await api('/api/playlists/' + id, { method: 'DELETE' });
    if (state.currentPlaylistId === id) {
      state.currentPlaylistId = null;
      $('playlistDetail').innerHTML = '';
      $('plDetailTitle').style.display = 'none';
    }
    toast(t('pl.deleted'), 'success');
    return loadPlaylists();
  }
  const row = e.target.closest('[data-pl]');
  if (row) {
    state.currentPlaylistId = Number(row.dataset.pl);
    loadPlaylists();
  }
});

async function renderPlaylistDetail(id) {
  const tracks = await api(`/api/playlists/${id}/tracks`);
  $('plDetailTitle').style.display = 'block';
  $('plDetailTitle').textContent = t('pl.detailTitle', { count: tracks.length });
  $('playlistDetail').innerHTML = `
    <div class="pl-actions">
      <button class="btn-primary" id="playAllBtn" ${tracks.length ? '' : 'disabled'}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5.5v13l11-6.5z"/></svg>${t('pl.playAll')}
      </button>
    </div>` +
    tracks.map((tr, i) => `
    <div class="track-row" data-play-track="${tr.id}">
      <span class="row-index">${i + 1}</span>
      <img class="row-cover cover-fallback" src="${coverUrl(tr)}" alt="${t('common.cover')}">
      <div class="row-meta">
        <div class="row-title">${esc(tr.title)}</div>
        <div class="row-artist">${esc(tr.artist || t('library.unknownArtist'))}</div>
      </div>
      <span class="row-dur">${fmtDuration(tr.duration)}</span>
      <button class="row-remove" data-remove-track="${tr.id}" title="${t('pl.removeFromList')}">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>
      </button>
    </div>`).join('');

  $('playAllBtn')?.addEventListener('click', () => {
    state.queue = tracks.slice();
    playIndex(0);
    closeDrawer();
  });
  $('playlistDetail').querySelectorAll('[data-play-track]').forEach((row) => {
    row.addEventListener('click', (e) => {
      if (e.target.closest('[data-remove-track]')) return;
      const id = Number(row.dataset.playTrack);
      state.queue = tracks.slice();
      playIndex(tracks.findIndex((tr) => tr.id === id));
      closeDrawer();
    });
  });
  $('playlistDetail').querySelectorAll('[data-remove-track]').forEach((btn) => {
    btn.addEventListener('click', async (e) => {
      e.stopPropagation();
      const tid = Number(btn.dataset.removeTrack);
      await api(`/api/playlists/${id}/tracks/${tid}`, { method: 'DELETE' });
      renderPlaylistDetail(id);
      loadPlaylists();
    });
  });
}

// 加列表：弹出播放列表选择器，由用户指定加入哪个列表；尚无列表时引导新建
async function quickAddToPlaylist(trackId) {
  state.pendingTrackId = trackId;
  let pls;
  try {
    pls = await api('/api/playlists');
  } catch (err) {
    state.pendingTrackId = null;
    return toast(err.message, 'error');
  }
  if (pls.length === 0) {
    toast(t('pl.needCreateFirst'));
    return openModal(); // 新建成功后会自动把该歌曲加入
  }
  openPicker(pls);
}

// ---------------- 播放列表选择器（加入指定列表） ----------------
function openPicker(pls) {
  $('pickerList').innerHTML = pls.map((p) => `
    <div class="picker-item" data-pick-pl="${p.id}">
      <div class="pl-icon"><svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor"><path d="M12 21s-7-4.6-9.5-9A5.5 5.5 0 0 1 12 6.2 5.5 5.5 0 0 1 21.5 12C19 16.4 12 21 12 21z"/></svg></div>
      <div style="flex:1;min-width:0">
        <div class="pl-name">${esc(p.name)}</div>
        <div class="pl-count">${t('pl.trackCount', { count: p.track_count })}</div>
      </div>
    </div>`).join('');
  $('pickerMask').classList.add('open');
}
function closePicker() { $('pickerMask').classList.remove('open'); }

$('pickerCancel').addEventListener('click', () => { closePicker(); state.pendingTrackId = null; });
$('pickerMask').addEventListener('click', (e) => {
  if (e.target === $('pickerMask')) { closePicker(); state.pendingTrackId = null; }
});
$('pickerList').addEventListener('click', async (e) => {
  const item = e.target.closest('[data-pick-pl]');
  if (!item) return;
  const playlistId = Number(item.dataset.pickPl);
  const trackId = state.pendingTrackId;
  if (!trackId) return;
  try {
    await api(`/api/playlists/${playlistId}/tracks/${trackId}`, { method: 'POST' });
    const name = item.querySelector('.pl-name')?.textContent || '';
    toast(t('pl.addedTo', { name: name }), 'success');
    closePicker();
    loadPlaylists();
  } catch (err) {
    toast(err.message, 'error');
  } finally {
    state.pendingTrackId = null;
  }
});
// 选择器内“新建并加入”：关闭选择器，打开新建模态框（保留 pendingTrackId）
$('pickerNewBtn').addEventListener('click', () => {
  closePicker();
  openModal();
});

// ---------------- 模态框（新建播放列表） ----------------
function openModal() {
  $('modalTitle').textContent = state.pendingTrackId ? t('modal.newPlaylistAndAdd') : t('modal.newPlaylist');
  $('modalInput').value = '';
  $('modalMask').classList.add('open');
  $('modalInput').focus();
}
function closeModal() { $('modalMask').classList.remove('open'); }
$('newPlaylistBtn').addEventListener('click', () => { state.pendingTrackId = null; openModal(); });
$('modalCancel').addEventListener('click', () => { closeModal(); state.pendingTrackId = null; });
$('modalMask').addEventListener('click', (e) => {
  if (e.target === $('modalMask')) { closeModal(); state.pendingTrackId = null; }
});
$('modalConfirm').addEventListener('click', async () => {
  const name = $('modalInput').value.trim();
  if (!name) return toast(t('pl.nameEmpty'), 'error');
  try {
    const pl = await api('/api/playlists', { method: 'POST', body: JSON.stringify({ name }) });
    state.currentPlaylistId = pl.id;
    closeModal();
    // 若处于“加列表”流程，新建后自动把当前歌曲加入该列表
    if (state.pendingTrackId) {
      const trackId = state.pendingTrackId;
      state.pendingTrackId = null;
      try {
        await api(`/api/playlists/${pl.id}/tracks/${trackId}`, { method: 'POST' });
        toast(t('pl.createdAndAdded', { name: name }), 'success');
      } catch (err) {
        toast(t('pl.createdButAddFailed', { msg: err.message }), 'error');
      }
    } else {
      toast(t('pl.created'), 'success');
    }
    loadPlaylists();
  } catch (err) {
    toast(err.message, 'error');
  }
});
$('modalInput').addEventListener('keydown', (e) => { if (e.key === 'Enter') $('modalConfirm').click(); });

// ---------------- 多语言切换 ----------------
$('langBtn').addEventListener('click', () => I18N.toggleLang());

// 语言切换后，重建所有由 JS 动态生成的文案与 DOM
document.addEventListener('langchange', () => {
  // 主题/模式按钮 title（applyDOM 不覆盖这些运行时改写的属性）
  setTheme(state.theme);
  syncViewToggle();
  setMode(state.mode, true);
  // 定时下拉选项（保留当前倒计时显示）
  resetSleepOptionText();
  if (sleepUntil) renderSleep();
  // 动态列表与详情
  renderTracks();
  refreshStats();
  // 目录树用缓存数据按新语言重建（根目录名等文案随语言刷新），避免重复请求
  if (rawDirs.length) { dirTreeData = buildDirTree(rawDirs); renderDirTree(); }
  renderResumeBadge();
  loadPlaylists().catch(() => {});
  // 模态框标题（若打开中）
  if ($('modalMask').classList.contains('open')) openModal();
});

// 刷新后恢复上次播放的曲目与位置（数据来自服务器，任意设备登录均可恢复；不自动播放）
async function restoreLastSession() {
  if (!positionsLoaded || !lastTrackId) return;
  try {
    const tr = await api('/api/tracks/' + lastTrackId);
    state.queue = [tr];
    state.queuePos = 0;
    loadTrack(tr, true);
    renderResumeBadge();
    updateActiveCard(tr.id);
  } catch (_) {
    /* 曲目可能已被删除，静默忽略 */
  }
}

// ---------------- 初始化 ----------------
setTheme(state.theme);
setView(state.view);
setMode(state.mode, true);
setSpeed(state.speed, true);
resetSleepOptionText();
loadDirs().catch((err) => toast(err.message, 'error'));
// 先拉取服务器端播放进度，再加载曲库并恢复上次会话，保证续播位置可用
loadPositions()
  .then(() => loadTracks(true))
  .then(restoreLastSession)
  .catch((err) => toast(err.message, 'error'));
refreshStats();
setInterval(refreshStats, 30000);

// 清理历史版本写在浏览器本地的播放位置键（现已迁移到服务器端，避免残留误导）
try {
  localStorage.removeItem('myaudio_positions');
  localStorage.removeItem('myaudio_last_track');
} catch (_) { /* 忽略 */ }
