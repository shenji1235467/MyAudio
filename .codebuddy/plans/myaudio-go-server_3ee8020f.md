---
name: myaudio-go-server
overview: 搭建一个基于 Go (Gin) 的个人在线音频播放服务：单二进制部署、SQLite 存储、嵌入式 Web 界面（手机适配），支持 MP3/WAV 等音频的目录扫描入库、元数据/封面解析、列表搜索、在线流媒体播放（Range 断点续播）、播放列表与删除管理，并带简单密码登录保护。
design:
  architecture:
    framework: html
  styleKeywords:
    - 玻璃拟态
    - 深色主题
    - 霓虹渐变
    - 响应式
    - 微动效
  fontSystem:
    fontFamily: Noto Sans
    heading:
      size: 22px
      weight: 700
    subheading:
      size: 16px
      weight: 600
    body:
      size: 14px
      weight: 400
  colorSystem:
    primary:
      - "#7C5CFF"
      - "#00D4FF"
      - "#FF5C8A"
    background:
      - "#0B0E17"
      - "#141927"
      - "#1C2333"
    text:
      - "#F2F4F8"
      - "#9AA3B5"
      - "#FFFFFF"
    functional:
      - "#22C55E"
      - "#EF4444"
      - "#F59E0B"
      - "#3B82F6"
todos:
  - id: scaffold-project
    content: 初始化 Go 模块，创建目录结构、config 配置加载与示例配置文件
    status: completed
  - id: store-model
    content: 实现 SQLite 存储层：建表迁移、Track/Playlist 数据模型与 CRUD、搜索查询
    status: completed
  - id: scanner
    content: 实现目录扫描器：音频文件发现、标签与封面解析、增量入库、定时与手动触发
    status: completed
  - id: auth
    content: 实现认证模块：密码登录、Session Cookie、登录限速与路由保护中间件
    status: completed
  - id: api-layer
    content: 实现 Gin API 层：曲目列表/搜索/删除、Range 流播放、封面端点、播放列表 CRUD
    status: completed
  - id: web-ui
    content: 实现嵌入式前端单页应用：音频库网格、搜索、播放条、播放列表面板、响应式深色设计
    status: completed
  - id: deploy-docs
    content: 编写 main 入口与优雅退出、systemd 服务文件、README 部署说明并编译验证
    status: completed
---

## 用户需求
基于 Linux 搭建个人在线音频播放服务（Go/Gin + SQLite + 嵌入式前端）：本地音频目录扫描入库、元数据/封面解析、在线流播放（Range）、搜索、播放列表、删除管理、单用户密码登录，单二进制 + systemd 部署。

## 执行阶段

### 阶段一：HTML 静态原型（当前，待用户确认）
在 `prototype/` 目录产出纯静态原型页面（无业务 JS，Tailwind CDN + 自定义 CSS），供用户确认设计风格与布局：
- `prototype/pages/library.html` — 音频库主页：顶部导航（Logo/搜索/视图切换/扫描）、封面卡片网格（手机 2 列/桌面 6 列）、筛选芯片、底部毛玻璃播放条（进度/上下曲/播放模式）
- `prototype/pages/playlist.html` — 播放列表面板：右侧抽屉、播放列表列表、曲目行增删、播放全部
- `prototype/pages/login.html` — 登录页：居中毛玻璃卡片、密码输入、免登录选项
- `prototype/css/style.css` — 深色玻璃拟态样式：色板 #0B0E17/#7C5CFF/#00D4FF/#FF5C8A、Noto Sans、hover 上浮、呼吸光晕、微动效

### 阶段二：完整实现（原型确认后）
沿用原技术方案（Go 1.22+ / Gin / modernc.org/sqlite 纯 Go / dhowden/tag / embed.FS / Session + bcrypt / systemd）。前端以确认后的原型为基准实现真实交互（fetch API、播放器逻辑、搜索过滤、播放列表操作）。

## 实现要点（阶段二）
- 分层：Handler → Service → Store；`tracks` / `playlists` / `playlist_tracks` 三表
- 流播放 `http.ServeContent` 支持 Range；封面端点带缓存
- 扫描器：启动扫描 + ticker 定时 + API 手动触发，路径+mtime 增量，并发受限解析标签
- SQLite WAL；目录结构：`cmd/myaudio`、`internal/{config,model,store,scanner,auth,api}`、`web/`（embed 原型确认后的界面）、`config.example.yaml`、`deploy/myaudio.service`、`README.md`

## 设计风格
深色玻璃拟态 + 霓虹渐变，手机优先响应式；封面网格为主视觉，底部悬浮播放条为交互核心；hover 上浮、进度光晕、播放封面呼吸光晕等微动效。