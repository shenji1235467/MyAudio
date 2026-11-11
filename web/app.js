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

async function api(path, options = {}) {
  const resp = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });
  if (resp.status === 401) { location.href = '/login'; throw new Error('unauthorized'); }
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new Error(data.error || ('HTTP ' + resp.status));
  return data;
}

// 无封面时的 SVG 占位（data URI，零外部依赖）
const COVER_PLACEHOLDER =
  "data:image/svg+xml," + encodeURIComponent(
    `<svg xmlns='http://www.w3.org/2000/svg' width='300' height='300'>
      <defs><linearGradient id='g' x1='0' y1='0' x2='1' y2='1'>
        <stop offset='0' stop-color='#7C5CFF'/><stop offset='1' stop-color='#00D4FF'/>
      </linearGradient></defs>
      <rect width='300' height='300' fill='url(#g)'/>
      <text x='150' y='170' font-size='110' text-anchor='middle'>♪</text></svg>`);

function coverUrl(t) {
  return t.has_cover ? `/api/cover/${t.id}` : COVER_PLACEHOLDER;
}

// 文件名展示行：仅在文件名与标题不同（存在独立文件名且标签标题不一致）时显示
function fileLine(t) {
  const baseName = t.filename || '';
  if (!baseName) return '';
  const titleWithExt = (t.title || '') + '.' + (t.format || '');
  if (baseName === t.title || baseName === titleWithExt) return '';
  return `<div class="card-file" title="${esc(baseName)}">${esc(baseName)}</div>`;
}

// ---------------- 全局状态 ----------------
const state = {
  tracks: [],          // 当前已加载曲目
  total: 0,
  page: 0,
  perPage: 48,
  pages: 1,
  keyword: '',
  order: 'created_at',
  dir: '',              // 当前选中的相对目录（'' = 全部/根目录）
  theme: localStorage.getItem('myaudio_theme') === 'dark' ? 'dark' : 'light',
  view: localStorage.getItem('myaudio_view') || 'grid',
  queue: [],           // 播放队列
  queuePos: -1,
  mode: localStorage.getItem('myaudio_mode') || 'list', // list / loop / random
  currentPlaylistId: null,
  pendingTrackId: null, // “加入播放列表”选择器当前操作的曲目 ID
};

const audio = $('audio');

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
      `共 ${s.track_count} 首 · 总时长 ${fmtDuration(s.total_duration)}` +
      (s.scanning ? ' · 正在扫描…' : '');
  } catch (_) { /* 忽略统计失败 */ }
}

function renderTracks() {
  const grid = $('trackGrid');
  $('emptyState').style.display = state.tracks.length === 0 ? 'block' : 'none';
  grid.className = state.view === 'list' ? 'track-grid list-view' : 'track-grid';

  const currentId = state.queuePos >= 0 ? state.queue[state.queuePos]?.id : null;
  grid.innerHTML = state.tracks.map((t) => {
    const active = t.id === currentId ? ' active' : '';
    if (state.view === 'list') {
      return `
      <div class="track-card${active}" data-id="${t.id}">
        <div class="cover-wrap">
          <img class="cover-img" src="${coverUrl(t)}" alt="封面" onerror="this.src='${COVER_PLACEHOLDER}'">
        </div>
        <div class="card-body">
          <div class="card-title">${esc(t.title)}</div>
          <div class="card-artist">${esc(t.artist || '未知艺术家')}${t.album ? ' · ' + esc(t.album) : ''}</div>
          ${fileLine(t)}
        </div>
        <span class="badge dur" style="position:static">${fmtDuration(t.duration)}</span>
        <div class="card-actions" style="flex:0 0 auto">
          <button class="mini-btn" data-act="playlist" data-id="${t.id}">加列表</button>
          <button class="mini-btn danger" data-act="delete" data-id="${t.id}">删除</button>
        </div>
      </div>`;
    }
    return `
    <div class="track-card${active}" data-id="${t.id}">
      <div class="cover-wrap">
        <img class="cover-img" src="${coverUrl(t)}" alt="${esc(t.title)}封面" onerror="this.src='${COVER_PLACEHOLDER}'">
        <div class="play-overlay"><div class="play-btn-lg">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5.5v13l11-6.5z"/></svg>
        </div></div>
        <span class="badge fmt">${esc(t.format)}</span>
        <span class="badge dur">${fmtDuration(t.duration)}</span>
      </div>
      <div class="card-body">
        <div class="card-title">${esc(t.title)}</div>
        <div class="card-artist">${esc(t.artist || '未知艺术家')}</div>
        ${fileLine(t)}
        <div class="card-actions">
          <button class="mini-btn" data-act="playlist" data-id="${t.id}">加入列表</button>
          <button class="mini-btn danger" data-act="delete" data-id="${t.id}">删除</button>
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
  if (!confirm('确定删除该曲目记录？\n点击“确定”仅移除记录；如需同时删除磁盘文件，请在后续确认中选择。')) return;
  const deleteFile = confirm('是否同时删除服务器上的音频文件？\n（确定=删除文件，取消=仅移除库记录）');
  try {
    await api(`/api/tracks/${id}?delete_file=${deleteFile}`, { method: 'DELETE' });
    toast('已删除', 'success');
    state.tracks = state.tracks.filter((t) => t.id !== id);
    renderTracks();
    refreshStats();
  } catch (err) {
    toast(err.message, 'error');
  }
}

// ---------------- 播放器 ----------------
function playIndex(i) {
  if (i < 0 || i >= state.queue.length) return;
  state.queuePos = i;
  const t = state.queue[i];
  audio.src = `/api/stream/${t.id}`;
  audio.play().catch((err) => toast('播放失败: ' + err.message, 'error'));
  $('npTitle').textContent = t.title;
  $('npArtist').textContent = t.artist || '未知艺术家';
  $('npCover').src = coverUrl(t);
  $('npCover').classList.add('now-playing-cover');
  renderTracks();
}

function togglePlay() {
  if (state.queuePos < 0) {
    if (state.tracks.length) {
      state.queue = state.tracks.slice();
      playIndex(0);
    } else {
      toast('音频库为空');
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
const MODE_LABEL = { list: '列表循环关闭', loop: '列表循环', random: '随机播放' };

function setMode(mode) {
  state.mode = mode;
  localStorage.setItem('myaudio_mode', mode);
  $('modeIcon').outerHTML = MODE_ICONS[mode].replace('<svg ', '<svg id="modeIcon" ');
  $('modeBtn').title = MODE_LABEL[mode];
  toast(MODE_LABEL[mode]);
}

$('playBtn').addEventListener('click', togglePlay);
$('nextBtn').addEventListener('click', () => playNext(false));
$('prevBtn').addEventListener('click', playPrev);
$('modeBtn').addEventListener('click', () => {
  const order = ['list', 'loop', 'random'];
  setMode(order[(order.indexOf(state.mode) + 1) % 3]);
});

audio.addEventListener('play', () => {
  $('playIcon').innerHTML = '<path d="M7 5h4v14H7zM13 5h4v14h-4z"/>';
});
audio.addEventListener('pause', () => {
  $('playIcon').innerHTML = '<path d="M8 5.5v13l11-6.5z"/>';
});
audio.addEventListener('ended', () => playNext(true));
audio.addEventListener('loadedmetadata', () => { $('durTime').textContent = fmtDuration(audio.duration); });
audio.addEventListener('timeupdate', updateProgress);
audio.addEventListener('error', () => {
  if (audio.src) toast('音频加载失败，文件可能已被移动', 'error');
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
if ('mediaSession' in navigator) {
  audio.addEventListener('loadedmetadata', () => {
    const t = state.queue[state.queuePos];
    if (!t) return;
    navigator.mediaSession.metadata = new MediaMetadata({
      title: t.title, artist: t.artist || '未知艺术家', album: t.album || '',
      artwork: [{ src: coverUrl(t), sizes: '300x300', type: 'image/jpeg' }],
    });
  });
  navigator.mediaSession.setActionHandler('play', () => audio.play());
  navigator.mediaSession.setActionHandler('pause', () => audio.pause());
  navigator.mediaSession.setActionHandler('previoustrack', playPrev);
  navigator.mediaSession.setActionHandler('nexttrack', () => playNext(false));
}

// ---------------- 目录导航 ----------------
async function loadDirs() {
  let dirs;
  try {
    dirs = await api('/api/dirs');
  } catch (err) {
    return; // 目录加载失败不阻断主界面
  }
  const bar = $('dirBar');
  if (!dirs.length) { bar.innerHTML = ''; return; }
  bar.innerHTML = dirs.map((d) => `
    <span class="dir-chip ${d.dir === state.dir ? 'active' : ''}" data-dir="${esc(d.dir)}" title="${esc(d.dir || '根目录')}">
      <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>
      ${esc(d.name)}<span class="dir-count">${d.count}</span>
    </span>`).join('');
}

$('dirBar').addEventListener('click', (e) => {
  const chip = e.target.closest('.dir-chip');
  if (!chip) return;
  state.dir = chip.dataset.dir || '';
  loadDirs();          // 更新高亮
  loadTracks(true);    // 按新目录重新加载
});

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
  loadTracks(true);
});

function setView(view) {
  state.view = view;
  localStorage.setItem('myaudio_view', view);
  $('gridViewBtn').classList.toggle('active', view === 'grid');
  $('listViewBtn').classList.toggle('active', view === 'list');
  renderTracks();
}
$('gridViewBtn').addEventListener('click', () => setView('grid'));
$('listViewBtn').addEventListener('click', () => setView('list'));
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
  $('themeBtn').title = theme === 'dark' ? '切换到明亮主题' : '切换到暗色主题';
  const meta = document.getElementById('metaThemeColor');
  if (meta) meta.setAttribute('content', THEME_COLOR[theme]);
}
$('themeBtn').addEventListener('click', () => {
  setTheme(state.theme === 'dark' ? 'light' : 'dark');
});

// 手动扫描
$('scanBtn').addEventListener('click', async () => {
  try {
    await api('/api/scan', { method: 'POST' });
    toast('扫描已启动…', 'success');
    $('scanIcon').classList.add('spin');
    pollScan();
  } catch (err) {
    toast(err.message, 'error');
  }
});
async function pollScan() {
  for (let i = 0; i < 30; i++) {
    await new Promise((r) => setTimeout(r, 2000));
    const s = await api('/api/stats').catch(() => null);
    if (s && !s.scanning) {
      $('scanIcon').classList.remove('spin');
      toast('扫描完成', 'success');
      loadTracks(true);
      loadDirs();
      loadPlaylists();
      return;
    }
  }
  $('scanIcon').classList.remove('spin');
}

// 退出登录
$('logoutBtn').addEventListener('click', async () => {
  if (!confirm('确定退出登录？')) return;
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
        <div class="pl-count">${p.track_count} 首</div>
      </div>
      <button class="row-remove" data-pl-del="${p.id}" title="删除列表">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14"/></svg>
      </button>
    </div>`).join('')
    : '<p class="text-secondary" style="font-size:13px;padding:8px 4px">还没有播放列表</p>';

  if (state.currentPlaylistId) renderPlaylistDetail(state.currentPlaylistId);
}

$('playlistList').addEventListener('click', async (e) => {
  const del = e.target.closest('[data-pl-del]');
  if (del) {
    e.stopPropagation();
    const id = Number(del.dataset.plDel);
    if (!confirm('删除该播放列表？（曲目本身不会被删除）')) return;
    await api('/api/playlists/' + id, { method: 'DELETE' });
    if (state.currentPlaylistId === id) {
      state.currentPlaylistId = null;
      $('playlistDetail').innerHTML = '';
      $('plDetailTitle').style.display = 'none';
    }
    toast('列表已删除', 'success');
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
  $('plDetailTitle').textContent = `列表曲目 · ${tracks.length} 首`;
  $('playlistDetail').innerHTML = `
    <div class="pl-actions">
      <button class="btn-primary" id="playAllBtn" ${tracks.length ? '' : 'disabled'}>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5.5v13l11-6.5z"/></svg>播放全部
      </button>
    </div>` +
    tracks.map((t, i) => `
    <div class="track-row" data-play-track="${t.id}">
      <span class="row-index">${i + 1}</span>
      <img class="row-cover" src="${coverUrl(t)}" alt="封面" onerror="this.src='${COVER_PLACEHOLDER}'">
      <div class="row-meta">
        <div class="row-title">${esc(t.title)}</div>
        <div class="row-artist">${esc(t.artist || '未知艺术家')}</div>
      </div>
      <span class="row-dur">${fmtDuration(t.duration)}</span>
      <button class="row-remove" data-remove-track="${t.id}" title="从列表移除">
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
      playIndex(tracks.findIndex((t) => t.id === id));
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
    toast('请先新建一个播放列表');
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
        <div class="pl-count">${p.track_count} 首</div>
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
    toast(`已加入「${name}」`, 'success');
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

// ---------------- 模态框（新建/重命名） ----------------
let modalMode = 'create'; // create
function openModal() {
  modalMode = 'create';
  $('modalTitle').textContent = state.pendingTrackId ? '新建播放列表并加入' : '新建播放列表';
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
  if (!name) return toast('名称不能为空', 'error');
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
        toast(`已创建并加入「${name}」`, 'success');
      } catch (err) {
        toast('列表已创建，但加入失败: ' + err.message, 'error');
      }
    } else {
      toast('列表已创建', 'success');
    }
    loadPlaylists();
  } catch (err) {
    toast(err.message, 'error');
  }
});
$('modalInput').addEventListener('keydown', (e) => { if (e.key === 'Enter') $('modalConfirm').click(); });

// ---------------- 初始化 ----------------
setTheme(state.theme);
setView(state.view);
setMode(state.mode);
loadDirs().catch(() => {});
loadTracks(true).catch((err) => toast(err.message, 'error'));
refreshStats();
setInterval(refreshStats, 30000);
