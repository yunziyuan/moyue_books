#!/usr/bin/env bash
# =============================================================================
# moyue-books 安卓 App 一键构建（全局入口）
#
# 串联：前端构建 → Capacitor 同步 → Gradle 打包（debug / release / install）
# 实际 Gradle 打包逻辑见 frontend/android/build_apk.sh（含 JDK/SDK 探测、签名、安装）。
#
# 用法：
#   ./build-android.sh                 # 打 debug 包（默认）
#   ./build-android.sh release         # 打 release 包（需 frontend/android/keystore.properties）
#   ./build-android.sh install         # 构建 debug 并 adb 安装到已连接设备
#   ./build-android.sh release install # 构建 release 并安装
#   ./build-android.sh -h              # 查看帮助（转发给 build_apk.sh）
# =============================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
FRONTEND="$ROOT/frontend"

# 转发 -h 给底层脚本（其余参数原样传递）
if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then
  (cd "$FRONTEND/android" && ./build_apk.sh -h)
  exit 0
fi

echo "==> 1/3 构建前端（移动端纯构建，base 为相对路径）"
(cd "$FRONTEND" && npm run build)

echo "==> 2/3 同步前端产物到 Android 工程（Capacitor）"
(cd "$FRONTEND" && npx cap sync android)

echo "==> 3/3 构建 Android APK"
(cd "$FRONTEND/android" && ./build_apk.sh "$@")

echo "🎉 完成。"
