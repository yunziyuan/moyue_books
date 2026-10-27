#!/bin/sh
set -e

# go-fitz 的 nocgo（purego）实现运行期需要 libmupdf 共享库，且 MuPDF 会校验版本字符串：
# 传入的 FZ_VERSION 必须与已安装的 libmupdf 完全一致，否则报 "version mismatch"。
# 这里在容器启动时自动探测已装 mupdf 版本并写入 FZ_VERSION（若已显式设置则尊重之）。
if [ -z "$FZ_VERSION" ]; then
  VER=$(apk info -v mupdf 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
  if [ -n "$VER" ]; then
    export FZ_VERSION="$VER"
  fi
fi

exec /app/moyue-server "$@"
