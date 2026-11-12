#!/usr/bin/env bash
#
# MyAudio 服务管理脚本：封装编译、启动、停止、重启、状态、日志查看与密码重置。
# 用法：./run.sh <命令>
# 可用 CONFIG / PID_FILE / LOG_FILE 环境变量覆盖默认路径，便于多实例隔离。
#
set -euo pipefail

# 切换到脚本所在目录，保证相对路径（二进制、配置）始终有效
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ---- 可配置变量（环境变量优先）----
BIN="$SCRIPT_DIR/myaudio"
CONFIG="${CONFIG:-$SCRIPT_DIR/config.yaml}"
MAIN_PKG="./cmd/myaudio"

# 默认配置沿用 README 中的 myaudio.pid / myaudio.log；
# 自定义配置则按配置名派生独立的 pid/log，避免多实例相互干扰。
CONF_BASENAME="$(basename "$CONFIG" .yaml)"
if [ "$CONF_BASENAME" = "config" ]; then
	PID_FILE="${PID_FILE:-$SCRIPT_DIR/myaudio.pid}"
	LOG_FILE="${LOG_FILE:-$SCRIPT_DIR/myaudio.log}"
else
	PID_FILE="${PID_FILE:-$SCRIPT_DIR/.${CONF_BASENAME}.pid}"
	LOG_FILE="${LOG_FILE:-$SCRIPT_DIR/${CONF_BASENAME}.log}"
fi

# ---- 输出辅助 ----
if [ -t 1 ]; then
	RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; BLUE=$'\033[34m'; NC=$'\033[0m'
else
	RED=""; GREEN=""; YELLOW=""; BLUE=""; NC=""
fi
info() { echo "${BLUE}[run.sh]${NC} $*"; }
ok()   { echo "${GREEN}[  OK  ]${NC} $*"; }
warn() { echo "${YELLOW}[ WARN ]${NC} $*"; }
err()  { echo "${RED}[ FAIL ]${NC} $*" >&2; }

# 解析 config.yaml 的监听地址，输出 LISTEN_HOST / LISTEN_PORT
parse_listen() {
	LISTEN_HOST=""; LISTEN_PORT=""
	[ -f "$CONFIG" ] || return 0
	local line
	line="$(grep -E '^[[:space:]]*listen:' "$CONFIG" | head -1 \
		| sed -E 's/^[[:space:]]*listen:[[:space:]]*//; s/["'"'"']//g' || true)"
	[ -z "$line" ] && return 0
	LISTEN_HOST="${line%:*}"
	LISTEN_PORT="${line##*:}"
}

# 判断进程是否存活（同时校验 PID 文件，防止过期/复用）
is_running() {
	[ -f "$PID_FILE" ] || return 1
	local pid
	pid="$(cat "$PID_FILE" 2>/dev/null || true)"
	[ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

current_pid() { cat "$PID_FILE" 2>/dev/null || true; }

# 解析受管进程 PID：优先 PID 文件；文件缺失时按“进程名 -config 配置路径”匹配孤儿进程。
# 用进程名（myaudio）而非 $BIN 绝对路径，兼容 ./myaudio 等相对路径启动的场景。
resolve_pid() {
	if is_running; then
		current_pid
		return 0
	fi
	pgrep -f "myaudio -config $CONFIG" 2>/dev/null | head -1 || true
}

# 规范化监听地址：剥离 IPv6 方括号，空/0.0.0.0/::/星号 统一为通配符 *
normalize_host() {
	local h="$1"
	h="${h#[}"
	h="${h%]}"
	case "$h" in
		""|"*"|"0.0.0.0"|"::"|"[::]") echo "*" ;;
		*) echo "$h" ;;
	esac
}

# 判断目标 地址:端口 是否与现有监听冲突（0=冲突，1=可绑定）。
# 规则：同端口下，任一方为通配符 *，或两者具体地址相同，即为冲突；
# 不同具体地址（如 nginx 绑公网 IP、服务绑 127.0.0.1）可共存，不算冲突。
# 参数：$1=目标 host（可空，表示通配），$2=目标端口
port_in_use() {
	local want_host want_port
	want_host="$(normalize_host "$1")"
	want_port="$2"
	[ -n "$want_port" ] || return 1

	# ss 列：State Recv-Q Send-Q Local Address:Port Peer ... 第4列为本地地址
	local _state _recv _send addr rest host port
	while read -r _state _recv _send addr rest; do
		[ -n "$addr" ] || continue
		port="${addr##*:}"
		[ "$port" = "$want_port" ] || continue
		host="$(normalize_host "${addr%:*}")"
		if [ "$host" = "*" ] || [ "$want_host" = "*" ] || [ "$host" = "$want_host" ]; then
			return 0
		fi
	done < <(ss -ltn 2>/dev/null | tail -n +2)
	return 1
}

# 找出与目标 地址:端口 冲突的监听进程，输出 "进程名 PID" 列表
port_owners() {
	local want_host want_port
	want_host="$(normalize_host "$1")"
	want_port="$2"
	# ss -p 列：前4列同上，进程信息在第6列起的 users:(...)
	local _state _recv _send addr _peer rest host port
	while read -r _state _recv _send addr _peer rest; do
		[ -n "$addr" ] || continue
		port="${addr##*:}"
		[ "$port" = "$want_port" ] || continue
		host="$(normalize_host "${addr%:*}")"
		if [ "$host" = "*" ] || [ "$want_host" = "*" ] || [ "$host" = "$want_host" ]; then
			echo "$rest" | grep -oE 'users:\(\("[^"]+",pid=[0-9]+' \
				| sed -E 's/users:\(\("//; s/",pid=/ /'
		fi
	done < <(ss -ltnp 2>/dev/null | tail -n +2) | sort -u
}

# 启动前统一校验：存活进程 / 端口占用，任一冲突即中止。
# 返回 0 表示可继续启动；非 0 表示已报告冲突并应退出。
guard_start() {
	if is_running; then
		warn "服务已在运行（PID $(current_pid)），如需重启请用：$0 restart"
		exit 0
	fi
	# PID 文件存在但进程已死，清理过期文件
	[ -f "$PID_FILE" ] && ! is_running && rm -f "$PID_FILE"

	parse_listen
	if port_in_use "$LISTEN_HOST" "$LISTEN_PORT"; then
		err "端口 ${LISTEN_HOST:-*}:${LISTEN_PORT} 已被占用，无法启动"
		local owners; owners="$(port_owners "$LISTEN_HOST" "$LISTEN_PORT")"
		if [ -n "$owners" ]; then
			echo "  占用进程：" >&2
			echo "$owners" | sed 's/^/    - /' >&2
		fi
		echo >&2
		echo "  处理建议：" >&2
		echo "    1) 若为旧实例：$0 stop  或  $0 force-stop" >&2
		echo "    2) 若为其他程序（如 nginx）：修改 config.yaml 的 listen 换端口，或改用 127.0.0.1 绑定" >&2
		echo "    3) 确认后重新启动：$0 start" >&2
		exit 1
	fi
}

# 启动后轮询等待进程存活，捕获“启动即崩溃”（如端口绑定失败）
# 参数：$1=刚启动的 PID
wait_healthy() {
	local pid="$1"
	local i
	for i in $(seq 1 10); do
		# 进程已退出 → 启动失败
		if ! kill -0 "$pid" 2>/dev/null; then
			rm -f "$PID_FILE"
			err "进程启动后立即退出，最近日志："
			tail -n 15 "$LOG_FILE" 2>/dev/null | sed 's/^/    /' >&2
			exit 1
		fi
		# 端口已监听 → 启动成功
		parse_listen
		if port_in_use "$LISTEN_HOST" "$LISTEN_PORT"; then
			return 0
		fi
		sleep 0.3
	done
	# 进程仍在但端口迟迟未监听，视为异常
	warn "进程存活但 10 次轮询内未检测到端口 ${LISTEN_PORT} 监听，请查看日志：$LOG_FILE"
}

# ---- 命令实现 ----
do_build() {
	info "编译 $MAIN_PKG ..."
	if ! command -v go >/dev/null 2>&1; then
		# 兼容官方 tar 包安装到 /usr/local/go 的环境
		[ -x /usr/local/go/bin/go ] && export PATH="/usr/local/go/bin:$PATH"
	fi
	if ! command -v go >/dev/null 2>&1; then
		err "未找到 Go 工具链，请安装 Go 1.22+ 或将其加入 PATH"
		exit 1
	fi
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN" "$MAIN_PKG"
	ok "编译完成：$BIN ($(du -h "$BIN" | cut -f1))"
}

do_start() {
	if [ ! -x "$BIN" ]; then
		warn "二进制不存在，先执行编译 ..."
		do_build
	fi
	if [ ! -f "$CONFIG" ]; then
		err "配置文件不存在：$CONFIG（可复制 config.example.yaml 为 config.yaml）"
		exit 1
	fi
	# 统一预检：存活进程、过期 PID、端口占用
	guard_start
	info "启动服务（配置：$CONFIG，日志：$LOG_FILE）"
	nohup "$BIN" -config "$CONFIG" >> "$LOG_FILE" 2>&1 &
	local pid="$!"
	echo "$pid" > "$PID_FILE"
	wait_healthy "$pid"
	ok "服务已启动（PID $pid）"
	[ -n "$LISTEN_PORT" ] && info "监听端口：$LISTEN_PORT，健康检查：$0 health"
}

do_stop() {
	local pid; pid="$(resolve_pid)"
	if [ -z "$pid" ]; then
		warn "服务未运行"
		rm -f "$PID_FILE"
		return 0
	fi
	is_running || warn "PID 文件缺失，按进程特征定位到孤儿实例（PID $pid）"
	info "发送 SIGTERM 优雅停止（PID $pid）..."
	kill "$pid" 2>/dev/null || true
	# 等待优雅退出，最多 10 秒
	for _ in $(seq 1 10); do
		kill -0 "$pid" 2>/dev/null || break
		sleep 1
	done
	if kill -0 "$pid" 2>/dev/null; then
		warn "优雅退出超时（>10s），可强制终止：$0 force-stop"
		exit 1
	fi
	rm -f "$PID_FILE"
	ok "服务已停止"
}

do_force_stop() {
	local pid; pid="$(resolve_pid)"
	if [ -z "$pid" ]; then
		warn "服务未运行"
		rm -f "$PID_FILE"
		return 0
	fi
	is_running || warn "PID 文件缺失，按进程特征定位到孤儿实例（PID $pid）"
	warn "发送 SIGKILL 强制终止（PID $pid）..."
	kill -9 "$pid" 2>/dev/null || true
	rm -f "$PID_FILE"
	ok "已强制终止"
}

do_restart() {
	info "重启服务 ..."
	if is_running; then
		do_stop
	fi
	do_start
}

# 生成 16 位 URL 安全随机密码（仅依赖 coreutils）
random_password() {
	local pw
	pw="$(head -c 18 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=[:space:]')"
	printf '%s' "${pw:0:16}"
}

# 重置访问密码：替换配置中的 password / password_hash，运行中的实例自动重启生效。
# 用法：do_reset_password [新密码]；不传则自动生成随机密码。
do_reset_password() {
	[ -f "$CONFIG" ] || { err "配置文件不存在：$CONFIG（先执行一次 $0 start 生成配置）"; exit 1; }

	local pw="${1:-}"
	if [ -z "$pw" ]; then
		pw="$(random_password)"
		info "未指定新密码，已生成随机密码"
	fi
	if [[ "$pw" == *$'\n'* || "$pw" == *$'\r'* ]]; then
		err "密码不能包含换行符"
		exit 1
	fi

	# YAML 单引号标量：内部单引号成对转义，其余字符（空格、\、"）均为字面量。
	local sq="'" dq="''"
	local quoted="${sq}${pw//$sq/$dq}${sq}"
	local tmp
	tmp="$(mktemp "${CONFIG}.XXXXXX")"
	chmod 600 "$tmp"
	# 通过环境变量传值，避免 awk -v 解析密码中的反斜杠转义；其余配置行与注释原样保留。
	if ! NEW_PASSWORD="$quoted" awk '
		/^[[:space:]]*password[[:space:]]*:/ || /^[[:space:]]*password_hash[[:space:]]*:/ {
			if (!inserted) { print "password: " ENVIRON["NEW_PASSWORD"]; inserted = 1 }
			next
		}
		{ print }
		END { if (!inserted) print "password: " ENVIRON["NEW_PASSWORD"] }
	' "$CONFIG" > "$tmp"; then
		rm -f "$tmp"
		err "写入配置失败，原配置未改动"
		exit 1
	fi
	# 备份后原子替换，保留 0600 权限，防止密码泄露且便于回滚。
	cp -p "$CONFIG" "${CONFIG}.bak"
	chmod 600 "${CONFIG}.bak"
	mv -f "$tmp" "$CONFIG"
	chmod 600 "$CONFIG"
	ok "新密码已写入 $CONFIG（备份：${CONFIG}.bak）"
	ok "访问密码：$pw （仅本次显示，请妥善保存）"

	# 重启后程序会把明文 password 转为 bcrypt 哈希写回配置，并清空内存中的旧会话。
	local pid; pid="$(resolve_pid)"
	if [ -n "$pid" ]; then
		info "服务正在运行，重启使新密码生效（旧登录会话将失效）..."
		do_stop
		do_start
	else
		info "服务未运行，新密码将在下次 $0 start 时生效"
	fi
}

do_status() {
	parse_listen
	if is_running; then
		local pid; pid="$(current_pid)"
		ok "服务运行中（PID $pid）"
		ps -o pid,ppid,rss,etime,cmd -p "$pid" || true
		if [ -n "$LISTEN_PORT" ]; then
			if port_in_use "$LISTEN_HOST" "$LISTEN_PORT"; then
				ok "端口 ${LISTEN_HOST:-*}:$LISTEN_PORT 正在监听"
			else
				warn "未检测到端口 ${LISTEN_HOST:-*}:$LISTEN_PORT 监听"
			fi
		fi
		return 0
	fi

	# PID 文件丢失/失效：检查端口是否仍被占用（孤儿进程或其他程序）
	if [ -n "$LISTEN_PORT" ] && port_in_use "$LISTEN_HOST" "$LISTEN_PORT"; then
		warn "未找到受管进程（PID 文件缺失），但端口 $LISTEN_PORT 仍被占用："
		local owners; owners="$(port_owners "$LISTEN_HOST" "$LISTEN_PORT")"
		[ -n "$owners" ] && echo "$owners" | sed 's/^/    - /'
		echo "    若为残留实例，可执行：pkill -f 'myaudio -config' 后重新 $0 start"
		exit 3
	fi
	warn "服务未运行"
	exit 3
}

do_health() {
	parse_listen
	local host="${LISTEN_HOST:-}"
	case "$host" in ""|0.0.0.0|::) host="127.0.0.1";; esac
	local port="${LISTEN_PORT:-8083}"
	local url="http://${host}:${port}/healthz"
	info "健康检查：$url"
	if curl -fsS --max-time 3 "$url"; then
		echo
		ok "服务健康"
	else
		err "健康检查失败"
		exit 1
	fi
}

do_logs() {
	[ -f "$LOG_FILE" ] || { err "日志文件不存在：$LOG_FILE"; exit 1; }
	local lines="${1:-100}"
	info "实时日志（Ctrl+C 退出）：$LOG_FILE"
	tail -n "$lines" -f "$LOG_FILE"
}

do_foreground() {
	[ -x "$BIN" ] || { warn "二进制不存在，先执行编译 ..."; do_build; }
	[ -f "$CONFIG" ] || { err "配置文件不存在：$CONFIG"; exit 1; }
	# 前台模式同样预检，避免端口冲突导致进程 fatal 退出
	guard_start
	info "前台运行（Ctrl+C 停止）：$BIN -config $CONFIG"
	exec "$BIN" -config "$CONFIG"
}

usage() {
	cat <<EOF
${BLUE}MyAudio 服务管理脚本${NC}

用法：${GREEN}./run.sh${NC} <命令>

命令：
  build        编译生成单二进制 $BIN
  start        后台启动（nohup，日志写入 $LOG_FILE；启动前预检端口占用，
               启动后轮询确认端口监听，失败自动打印日志并清理 PID）
  stop         优雅停止（SIGTERM，最多等待 10s；PID 文件丢失时自动定位孤儿进程）
  force-stop   强制终止（SIGKILL，非必要不使用；同样支持孤儿进程回收）
  restart      重启（先 stop 再 start）
  reset-password [新密码]
               重置访问密码（不传则生成 16 位随机密码），自动重启运行中的服务使其生效；
               仅替换配置里的 password / password_hash，其余配置不变，并生成 .bak 备份
  passwd       reset-password 的别名
  status       查看运行状态、PID、内存与监听端口；PID 丢失但端口被占用时给出诊断
  health       调用 /healthz 健康检查
  logs [N]     查看实时日志（默认显示最后 100 行）
  foreground   前台运行（调试用，Ctrl+C 停止；同样做端口占用预检）
  help         显示本帮助

环境变量（可选，用于多实例隔离）：
  CONFIG       配置文件路径（默认 $CONFIG）
  PID_FILE     PID 文件路径（默认随配置名派生）
  LOG_FILE     日志文件路径（默认随配置名派生）

示例：
  ./run.sh build && ./run.sh start
  ./run.sh restart
  ./run.sh reset-password                     # 随机新密码
  ./run.sh reset-password 'MyNewP@ss'         # 指定新密码
  CONFIG=/opt/myaudio/test.yaml ./run.sh start
EOF
}

# ---- 入口 ----
case "${1:-help}" in
	build)      do_build ;;
	start)      do_start ;;
	stop)       do_stop ;;
	force-stop) do_force_stop ;;
	restart)    do_restart ;;
	reset-password|passwd) shift; do_reset_password "${1:-}" ;;
	status)     do_status ;;
	health)     do_health ;;
	logs)       shift; do_logs "${1:-100}" ;;
	fg|foreground) do_foreground ;;
	help|-h|--help) usage ;;
	*) err "未知命令：$1"; echo; usage; exit 1 ;;
esac
