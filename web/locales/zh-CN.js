/* 简体中文词条 */
(function () {
  'use strict';
  window.I18N = window.I18N || {};
  window.I18N._locales = window.I18N._locales || {};
  window.I18N._locales['zh-CN'] = {
    // 通用
    'app.title': 'MyAudio · 私人音频库',
    'common.loading': '加载中…',
    'common.cancel': '取消',
    'common.confirm': '确定',
    'common.close': '关闭',
    'common.delete': '删除',
    'common.rootDir': '根目录',
    'common.cover': '封面',

    // 顶部导航
    'nav.searchPlaceholder': '搜索标题 / 艺术家 / 专辑…',
    'nav.gridView': '网格视图',
    'nav.listView': '列表视图',
    'nav.toggleTheme': '切换明亮 / 暗色主题',
    'nav.rescan': '重新扫描音频目录',
    'nav.playlists': '播放列表',
    'nav.logout': '退出登录',
    'nav.switchLang': 'Switch to English',
    'nav.folders': '目录',

    // 播放器
    'player.notPlaying': '未在播放',
    'player.selectTrack': '选择一首曲目开始',
    'player.prev': '上一首',
    'player.playPause': '播放/暂停',
    'player.next': '下一首',
    'player.mode': '播放模式',
    'player.speed': '播放速度',
    'player.sleep': '定时停止播放',
    'player.sleepTimer': '定时',
    'player.minutes': '{n} 分钟',
    'player.playbackFailed': '播放失败: {msg}',
    'player.audioLoadFailed': '音频加载失败，文件可能已被移动',
    'player.libraryEmpty': '音频库为空',
    'player.speedLabel': '播放速度 {speed}',
    'player.sleepEnded': '定时结束，已停止播放',
    'player.sleepCancelled': '已取消定时',
    'player.sleepInMinutes': '将在 {minutes} 分钟后停止播放',
    'player.resumeAt': '从 {time} 继续',

    // 播放模式
    'mode.list': '列表循环关闭',
    'mode.loop': '列表循环',
    'mode.random': '随机播放',

    // 音频库
    'library.title': '音频库',
    'library.stats': '共 {count} 首 · 总时长 {duration}',
    'library.scanning': ' · 正在扫描…',
    'library.orderLatest': '最新',
    'library.orderFilename': '文件名',
    'library.orderTitle': '标题',
    'library.orderArtist': '艺术家',
    'library.orderAlbum': '专辑',
    'library.emptyTitle': '暂无音频',
    'library.emptyHint': '请将 MP3 / WAV / FLAC 等文件放入音频目录，然后点击右上角扫描按钮',
    'library.loadMore': '加载更多',
    'library.unknownArtist': '未知艺术家',
    'library.addToList': '加入列表',
    'library.coverOf': '{title}封面',

    // 删除确认
    'track.deleteConfirm1': '确定删除该曲目记录？\n点击“确定”仅移除记录；如需同时删除磁盘文件，请在后续确认中选择。',
    'track.deleteConfirm2': '是否同时删除服务器上的音频文件？\n（确定=删除文件，取消=仅移除库记录）',
    'track.deleted': '已删除',

    // 主题
    'theme.toLight': '切换到明亮主题',
    'theme.toDark': '切换到暗色主题',

    // 扫描
    'scan.started': '扫描已启动…',
    'scan.done': '扫描完成',
    'scan.timeout': '扫描耗时较长，请稍后手动刷新',
    'scan.inProgress': '扫描任务正在进行中',

    // 退出登录
    'logout.confirm': '确定退出登录？',

    // 播放列表抽屉
    'pl.title': '播放列表',
    'pl.new': '新建播放列表',
    'pl.mine': '我的列表',
    'pl.tracksSection': '列表曲目',
    'pl.trackCount': '{count} 首',
    'pl.deleteList': '删除列表',
    'pl.empty': '还没有播放列表',
    'pl.deleteConfirm': '删除该播放列表？（曲目本身不会被删除）',
    'pl.deleted': '列表已删除',
    'pl.detailTitle': '列表曲目 · {count} 首',
    'pl.playAll': '播放全部',
    'pl.removeFromList': '从列表移除',
    'pl.needCreateFirst': '请先新建一个播放列表',
    'pl.addedTo': '已加入「{name}」',
    'pl.createdAndAdded': '已创建并加入「{name}」',
    'pl.createdButAddFailed': '列表已创建，但加入失败: {msg}',
    'pl.created': '列表已创建',
    'pl.nameEmpty': '名称不能为空',
    'pl.nameExists': '播放列表名称已存在',
    'pl.notFound': '记录不存在',

    // 模态框
    'modal.newPlaylist': '新建播放列表',
    'modal.newPlaylistAndAdd': '新建播放列表并加入',
    'modal.nameLabel': '列表名称',
    'modal.namePlaceholder': '例如：深夜驾驶',
    'modal.addToPlaylist': '加入播放列表',
    'modal.newAndAdd': '新建播放列表并加入',

    // 登录页
    'login.title': 'MyAudio · 登录',
    'login.subtitle': '私人在线音频播放服务',
    'login.passwordLabel': '访问密码',
    'login.passwordPlaceholder': '请输入访问密码',
    'login.submit': '进入音频库',
    'login.wrongPassword': '密码错误，请重试',
    'login.foot': '登录状态通过会话 Cookie 保持<br>连续失败 5 次将锁定 10 分钟',
    'login.enterPassword': '请输入密码',
    'login.signingIn': '登录中…',
    'login.failedHttp': '登录失败（HTTP {status}）',
    'login.networkError': '网络错误，请检查服务是否可用',
    'login.tooManyAttempts': '失败次数过多，请 10 分钟后再试',
    'login.badRequest': '请求格式错误',
    'login.unauthorized': '未登录或会话已过期',

    // 后端错误码映射
    'err.queryFailed': '查询失败: {msg}',
    'err.trackNotFound': '曲目不存在',
    'err.fileMissing': '音频文件不存在或已被移动',
    'err.invalidID': '无效的 ID',
    'err.invalidTrackID': '无效的曲目 ID',
    'err.emptyName': '名称不能为空',
    'err.dirQueryFailed': '查询目录失败: {msg}',
    'err.invalidPosition': '播放位置无效',
  };
})();
