# MyAudio · 个人在线音频播放服务

基于 **Go (Gin) + SQLite** 的轻量级私人音频流媒体服务。单二进制部署、无外部依赖、内存占用低，支持手机/电脑浏览器在线播放与管理本地 MP3、WAV、FLAC、OGG、M4A 音频。

## 特性

- **目录扫描入库**：递归扫描本地音频目录，自动解析标题/艺术家/专辑与内嵌封面；支持启动扫描、定时扫描、页面手动扫描，按路径+修改时间增量更新
- **在线流播放**：支持 HTTP Range 请求，可拖动进度条、断点续播；手机锁屏/蓝牙线控（Media Session API）
- **浏览与搜索**：封面网格 / 列表两种视图，按标题/艺术家/专辑关键词即时搜索，多种排序与分页
- **播放列表**：创建/重命名/删除播放列表，曲目加入/移除，连续播放、列表循环、随机播放三种模式
- **管理**：删除库记录，可选同时删除磁盘文件；自动清理已失效的曲目记录
- **安全**：单用户密码登录（bcrypt 哈希）、会话 Cookie、按 IP 登录限速（连续失败 5 次锁定 10 分钟）、安全响应头
- **易部署**：纯 Go 实现（SQLite 驱动无 CGO），前端通过 `embed` 编译进二进制，单个文件即可运行，附 systemd 单元

## 技术栈

| 层 | 选型 |
|---|---|
| HTTP 框架 | Gin |
| 数据库 | SQLite（`modernc.org/sqlite` 纯 Go 驱动，WAL 模式，无 CGO） |
| 标签解析 | `dhowden/tag`（ID3v1/v2、FLAC、OGG 等，提取内嵌封面） |
| 密码 | `golang.org/x/crypto/bcrypt` |
| 前端 | 原生 HTML/CSS/JS，`embed.FS` 内嵌，无构建步骤、无 CDN 依赖 |
| 配置 | YAML（`gopkg.in/yaml.v3`） |

## 目录结构

```
MyAduio/
├── cmd/myaudio/main.go        # 入口：装配模块、启动服务、优雅退出
├── internal/
│   ├── config/                # 配置加载（首次启动生成随机密码）
│   ├── model/                 # 实体与查询对象
│   ├── store/                 # SQLite 迁移、CRUD、搜索
│   ├── scanner/               # 目录扫描、标签/封面解析、WAV 时长、增量同步
│   ├── auth/                  # bcrypt 登录、Session、限速中间件
│   └── api/                   # Gin 路由与 Handler（流播放/封面/播放列表）
├── web/                       # 前端静态资源（embed 进二进制）
│   ├── embed.go
│   ├── index.html  login.html  app.js  style.css
├── deploy/myaudio.service     # systemd 单元
├── config.example.yaml        # 配置示例
└── go.mod / go.sum
```

## 快速开始

**默认参数速览：**

| 项目 | 默认值 |
|---|---|
| 服务监听端口（myaudio） | `8083`，默认绑定 `127.0.0.1:8083`（仅本机/反代后端） |
| 对外访问端口（nginx SSL 反代） | `18083`，转发到 `127.0.0.1:8083` |
| 音频目录 | `./data/music` |
| 数据目录（SQLite + 封面） | `./data` |
| 初始密码 | 无配置文件时自动生成 10 位随机密码（打印在日志）；有 `password` 配置则使用该值 |
| 定时扫描间隔 | `5m` |
| 会话有效期 | `168h`（7 天） |
| 登录保护 | 连续失败 5 次锁定 10 分钟 |

### 1. 编译

要求 Go 1.22+。

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o myaudio ./cmd/myaudio
```

产物 `myaudio` 为静态链接单二进制（约 25MB），可直接拷贝到目标 Linux 机器运行。

### 2. 配置

复制示例配置：

```bash
cp config.example.yaml config.yaml
```

```yaml
listen: "127.0.0.1:8083"     # 监听地址，默认端口 8083；反代后端用回环地址
music_dir: "./data/music"    # 音频目录（递归扫描）
data_dir: "./data"           # SQLite 与封面缓存目录
password: "change-me-please" # 访问密码；首次启动后自动转为 password_hash
scan_interval: 5m            # 定时扫描间隔，0 关闭
session_ttl: 168h            # 会话有效期（7 天）
max_upload_mb: 100           # 预留：网页上传单文件上限
```

配置项说明：

| 字段 | 默认值 | 说明 |
|---|---|---|
| `listen` | `127.0.0.1:8083` | 监听地址，默认端口 `8083`。`127.0.0.1:8083` 仅本机/反代访问（推荐）；`:8083`（等价 `0.0.0.0:8083`）监听所有网卡，**无反代且需手机局域网直连时用此项**。对外经 nginx SSL 反代时，访问端口仍是 `18083` |
| `music_dir` | `./data/music` | 音频目录，递归扫描，支持 `.mp3/.wav/.flac/.ogg/.m4a` |
| `data_dir` | `./data` | SQLite 数据库与封面缓存目录 |
| `password` / `password_hash` | — | 访问密码，二选一；`password_hash`（bcrypt）优先级更高 |
| `scan_interval` | `5m` | 定时扫描间隔，设为 `0` 关闭定时（仍可在页面手动扫描） |
| `session_ttl` | `168h`（7 天） | 登录会话有效期 |
| `max_upload_mb` | `100` | 预留字段，供后续网页上传功能使用 |

### 3. 初始化密码与首次登录

密码有两种初始化方式：

**方式 A：不提供 `config.yaml` 直接运行（自动生成随机密码）**

程序首次启动会自动生成一份默认配置和一个 **10 位随机初始密码**，打印在终端标准输出（systemd 部署时在 journal 日志中），形如：

```
[config] 未发现配置文件，已生成默认配置: config.yaml
[config] 初始访问密码: aB3xK9pQ2Z （请尽快修改 config.yaml 中的 password 后重启）
```

**方式 B：在 `config.yaml` 中自行填写 `password`**

使用你设置的明文密码。

> 无论哪种方式，明文 `password` 在服务首次启动后都会自动转为 bcrypt 哈希写入 `password_hash`，并清空明文字段（配置文件权限 `0600`）。之后登录校验只使用哈希。

**查看 / 修改 / 重置密码：**

```bash
# 查看自动生成的初始密码（直接运行时看终端；systemd 看日志）
journalctl -u myaudio | grep "初始访问密码"

# 修改密码：编辑 config.yaml，填入新的明文 password 并删掉旧的 password_hash，重启即可
#   password: "新密码"
#   # password_hash: ...   ← 删除此行，让程序重新生成哈希
sudo systemctl restart myaudio
```

### 4. 运行

`-config` 指定配置文件路径，缺省为当前目录的 `config.yaml`。提供前台调试与后台常驻两种方式。

#### 4.0 一键脚本（推荐）

项目根目录的 `run.sh` 封装了编译与常用启停操作，无需手动维护 PID：

```bash
chmod +x run.sh          # 首次使用赋予执行权限

./run.sh build           # 编译生成单二进制 myaudio
./run.sh start           # 后台启动（自动编译缺失的二进制，日志写入 myaudio.log）
./run.sh status          # 查看 PID、内存占用、运行时长与监听端口
./run.sh health          # 调用 /healthz 健康检查
./run.sh logs            # 实时查看日志（Ctrl+C 退出），可加行数：./run.sh logs 200
./run.sh restart         # 重启（修改 config.yaml 后生效）
./run.sh stop            # 优雅停止（SIGTERM，最多等待 10s）
./run.sh force-stop      # 强制终止（SIGKILL，非必要不使用）
./run.sh foreground      # 前台运行（调试用，Ctrl+C 停止）
./run.sh help            # 查看全部命令
```

脚本通过 PID 文件（默认 `myaudio.pid`）跟踪进程，能识别过期 PID、避免重复启动。多实例可用环境变量隔离：

```bash
CONFIG=/opt/myaudio/test.yaml ./run.sh start   # PID/日志按配置名派生，互不干扰
```

以下 4.1、4.2 为等价的手动命令，便于理解脚本内部行为或在无脚本环境使用。

#### 4.1 前台运行（调试用）

```bash
./myaudio -config config.yaml
```

日志直接输出到终端，`Ctrl+C` 发送 `SIGINT` 即可停止。适合首次启动确认初始密码、观察扫描过程。

#### 4.2 后台常驻运行（无 systemd 时）

```bash
# 启动：nohup 后台运行，日志追加到 myaudio.log
nohup ./myaudio -config config.yaml >> myaudio.log 2>&1 &

# 记录进程号，便于后续停止
echo $! > myaudio.pid
```

常用管理命令：

```bash
# 查看运行状态（PID 与监听端口）
ps -p "$(cat myaudio.pid)" -o pid,rss,cmd
ss -ltnp | grep 8083

# 查看实时日志
tail -f myaudio.log

# 停止：SIGTERM 触发优雅退出（等待在途请求，最多 8 秒）
kill "$(cat myaudio.pid)"

# 若未记录 PID，可用 pkill 按进程名停止
pkill -f 'myaudio -config'

# 重启（先停后启）
kill "$(cat myaudio.pid)" 2>/dev/null; sleep 1
nohup ./myaudio -config config.yaml >> myaudio.log 2>&1 & echo $! > myaudio.pid
```

> **信号说明**：程序捕获 `SIGINT`（`Ctrl+C`）与 `SIGTERM`（`kill` 默认信号）后停止接收新连接、等待进行中的请求结束再退出，最长等待 8 秒；`kill -9`（`SIGKILL`）会强制立即终止，非必要不使用。

#### 4.3 验证与访问

```bash
# 健康检查（无需登录，返回 {"status":"ok"} 即正常）
curl http://127.0.0.1:8083/healthz
```

- **直连（无反代）**：浏览器访问 `http://<服务器IP>:8083`，需将 `listen` 改为 `:8083` 监听所有网卡。
- **nginx SSL 反代（本项目部署方式）**：myaudio 仅监听回环 `127.0.0.1:8083`，对外由 nginx 在 `18083` 提供 HTTPS，浏览器访问 `https://<域名或服务器IP>:18083`，输入密码登录。

将音频文件放入 `music_dir`，点击页面右上角扫描按钮（或等待定时扫描）即可入库。

> **反代架构说明（推荐）**：服务默认绑定 `127.0.0.1:8083`，不直接暴露公网；nginx 对外监听 `18083`（SSL）并 `proxy_pass` 到 `127.0.0.1:8083`。因此防火墙只需放行对外端口 **18083**（`sudo ufw allow 18083/tcp`），后端 8083 无需对外。
>
> **无反代、手机局域网直连**：需把 `listen` 改为 `:8083`（监听所有网卡），手机与服务器同一局域网，访问 `http://<服务器局域网IP>:8083`，并放行 8083 端口。

> 生产环境建议使用下一节的 **systemd 托管**，可获得开机自启、崩溃自动重启与统一的 `systemctl` 启停管理，无需手动维护 PID。

## systemd 部署（推荐）

```bash
# 1. 创建专用用户与目录
sudo useradd -r -s /usr/sbin/nologin myaudio
sudo mkdir -p /opt/myaudio/data/music
sudo cp myaudio config.yaml /opt/myaudio/
sudo chown -R myaudio:myaudio /opt/myaudio

# 2. 安装服务
sudo cp deploy/myaudio.service /etc/systemd/
sudo systemctl daemon-reload
sudo systemctl enable --now myaudio

# 3. 查看状态与日志（首次启动的随机初始密码也在这里）
systemctl status myaudio
journalctl -u myaudio -f
```

安装后的日常启停管理：

```bash
sudo systemctl start myaudio      # 启动
sudo systemctl stop myaudio       # 停止（SIGTERM 优雅退出，TimeoutStopSec=10s）
sudo systemctl restart myaudio    # 重启（修改 config.yaml 后生效）
sudo systemctl status myaudio     # 查看运行状态、PID 与最近日志
sudo systemctl enable myaudio     # 设置开机自启
sudo systemctl disable myaudio    # 取消开机自启
journalctl -u myaudio -f          # 实时跟踪日志
journalctl -u myaudio --since "1 hour ago"   # 查看最近一小时日志
```

服务单元已配置：崩溃自动重启（`Restart=on-failure`）、128MB 内存上限、`NoNewPrivileges`、`ProtectSystem` 等安全加固，仅对数据目录开放写权限。

> **systemd 下的初始密码**：若 `/opt/myaudio/config.yaml` 未设置 `password`/`password_hash`，首次启动生成的随机密码可通过 `journalctl -u myaudio | grep "初始访问密码"` 查看。
>
> **放行端口**：myaudio 默认只监听回环 `127.0.0.1:8083`，本身不对外，无需放行 8083。经 nginx SSL 反代时，对外入口是 **18083**，仅需放行该端口：
>
> ```bash
> sudo ufw allow 18083/tcp     # ufw（nginx 对外 HTTPS 入口）
> # 或
> sudo firewall-cmd --add-port=18083/tcp --permanent && sudo firewall-cmd --reload   # firewalld
> ```
>
> 若不使用反代、改为 `:8083` 全网卡直连，则放行 8083 端口即可。

## API 一览

除 `/healthz`、`/login`、`/api/auth/login` 与静态资源外，均需登录会话。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/login` | 密码登录，下发会话 Cookie |
| POST | `/api/auth/logout` | 退出登录 |
| GET | `/api/tracks` | 曲目分页/搜索（`keyword`、`order`、`page`、`per_page`） |
| GET | `/api/tracks/:id` | 曲目详情 |
| DELETE | `/api/tracks/:id?delete_file=true` | 删除曲目（可选删文件） |
| GET | `/api/stream/:id` | 流式播放（支持 Range / 206） |
| GET | `/api/cover/:id` | 内嵌封面（带缓存） |
| GET | `/api/stats` | 库统计与扫描状态 |
| POST | `/api/scan` | 手动触发扫描（防重入） |
| GET/POST | `/api/playlists` | 播放列表列表 / 新建 |
| PUT/DELETE | `/api/playlists/:id` | 重命名 / 删除 |
| GET | `/api/playlists/:id/tracks` | 列表内曲目 |
| POST/DELETE | `/api/playlists/:id/tracks/:trackID` | 加入 / 移除曲目 |

## 设计说明

- **数据模型**：`tracks`、`playlists`、`playlist_tracks` 三表，外键级联删除，播放列表用 `position` 维护顺序
- **增量扫描**：以文件绝对路径唯一约束 + 修改时间判断变更，未变更文件跳过标签解析；扫描并发度限制为 `min(CPU, 4)`，控制 fd 与内存
- **封面存储**：内嵌封面以音频路径的 SHA1 命名落盘到 `data/covers`，通过 `/api/cover/:id` 提供并设置浏览器缓存
- **WAV 时长**：直接解析 RIFF 文件头得到精确时长；其他格式按平均码率估算
- **优雅退出**：捕获 `SIGINT/SIGTERM`，停止接收新连接并在 8 秒内完成在途请求

## 后续可扩展点

- 网页上传（`MaxUploadMB` 配置已预留）
- 多用户与权限
- 专辑/艺术家聚合视图、分类标签
- HTTPS（建议前置 Nginx/Caddy 反代）

## 许可证

MIT
