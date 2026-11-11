#!/usr/bin/env bash
#
# MyAudio 发布打包脚本：交叉编译单二进制并组装可直接部署的 release 压缩包。
#
# 用法：
#   ./scripts/release.sh                 # 默认 VERSION=dev，目标 linux/amd64
#   VERSION=v1.0.0 ./scripts/release.sh
#   VERSION=v1.0.0 TARGETS="linux/amd64 linux/arm64" ./scripts/release.sh
#
# 产物：dist/myaudio_<version>_<os>_<arch>.tar.gz（及同名 .sha256）
# 包内为运行时文件，不含源码、Git 元数据、数据库与音频数据。
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

VERSION="${VERSION:-dev}"
TARGETS="${TARGETS:-linux/amd64}"
OUT_DIR="$ROOT_DIR/dist"
STAGE_DIR="$OUT_DIR/stage"

# 兼容官方 tar 包安装到 /usr/local/go 的环境
if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi
command -v go >/dev/null 2>&1 || { echo "[FAIL] 未找到 Go 工具链" >&2; exit 1; }

# 随包分发的运行时文件（相对仓库根目录）
RUNTIME_FILES=(
  "config.example.yaml"
  "run.sh"
  "README.md"
  "deploy/myaudio.service"
)

echo "[release] MyAudio $VERSION"
echo "[release] 目标平台: $TARGETS"
rm -rf "$STAGE_DIR"
mkdir -p "$OUT_DIR" "$STAGE_DIR"

build_one() {
  local target="$1"
  local goos="${target%/*}"
  local goarch="${target#*/}"
  local pkg_name="myaudio_${VERSION}_${goos}_${goarch}"
  local pkg_dir="$STAGE_DIR/$pkg_name"

  echo "[release] 编译 $goos/$goarch ..."
  mkdir -p "$pkg_dir/deploy"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath \
      -ldflags="-s -w -X main.version=$VERSION" \
      -o "$pkg_dir/myaudio" ./cmd/myaudio

  # 组装运行时文件
  local f
  for f in "${RUNTIME_FILES[@]}"; do
    [ -f "$f" ] || { echo "[FAIL] 缺少随包文件: $f" >&2; exit 1; }
    cp "$f" "$pkg_dir/$f"
  done
  chmod +x "$pkg_dir/myaudio" "$pkg_dir/run.sh"

  # 预建数据目录占位（.gitkeep 保证空目录可随包分发，首次运行即可用）
  mkdir -p "$pkg_dir/data/music" "$pkg_dir/data/covers"
  : > "$pkg_dir/data/.gitkeep"

  # 打包（从 stage 目录进入，使归档根目录为包名）
  local archive="$OUT_DIR/$pkg_name.tar.gz"
  tar -C "$STAGE_DIR" -czf "$archive" "$pkg_name"
  ( cd "$OUT_DIR" && sha256sum "$(basename "$archive")" > "$(basename "$archive").sha256" )

  echo "[  OK  ] $archive"
}

for t in $TARGETS; do
  build_one "$t"
done

rm -rf "$STAGE_DIR"
echo "[release] 全部完成，产物目录: $OUT_DIR"
ls -lh "$OUT_DIR"
