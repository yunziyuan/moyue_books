#!/usr/bin/env bash
#
# moyue-books 安卓 APK 一键打包脚本
#
# 用法：
#   ./build_apk.sh                 # 打 debug 包（默认，无需签名，可直接安装测试）
#   ./build_apk.sh release         # 打 release 包（需 keystore.properties，见下）
#   ./build_apk.sh install         # 构建 debug 并 adb 安装到已连接设备
#   ./build_apk.sh release install # 构建 release 并安装
#   ./build_apk.sh -h              # 查看帮助
#
# 依赖：JDK 21+（Capacitor 7 要求）、Android SDK（platform + build-tools）、本地 local.properties
#   - local.properties 不存在时，脚本会自动探测 macOS 默认 SDK 路径并写入
#
# release 签名：在 android/ 目录放置 keystore.properties（不入库），内容：
#   storeFile=release.keystore
#   storePassword=你的密钥库密码
#   keyAlias=moyue
#   keyPassword=你的密钥密码
# 首次可用 keytool 生成：
#   keytool -genkeypair -v -keystore release.keystore -alias moyue \
#           -keyalg RSA -keysize 2048 -validity 10000
#
set -euo pipefail

# 进入脚本所在目录（android 工程根），保证 gradlew / local.properties 相对路径正确
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# ---------- 参数解析 ----------
BUILD_TYPE="debug"
INSTALL=0
for arg in "$@"; do
  case "$arg" in
    debug)   BUILD_TYPE="debug" ;;
    release) BUILD_TYPE="release" ;;
    install) INSTALL=1 ;;
    -h|--help)
      grep '^#' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "❌ 未知参数：$arg（可用 debug / release / install）" >&2; exit 1 ;;
  esac
done
if [ "$BUILD_TYPE" = "release" ]; then TASK="assembleRelease"; else TASK="assembleDebug"; fi
echo "📦 构建类型：$BUILD_TYPE"

# ---------- 1. 定位 JDK 并设置 JAVA_HOME ----------
# 注意：macOS 的 /usr/bin/java 只是个 stub，真实 JDK 常不在 PATH。优先探测常见位置，
# 找到后导出 JAVA_HOME 供 gradlew 使用，否则会卡在“Unable to locate a Java Runtime”。
JDK_CANDIDATES=(
  "${JAVA_HOME:-}"
  "$(/usr/libexec/java_home 2>/dev/null || true)"
  /opt/homebrew/opt/openjdk@17
  /opt/homebrew/opt/openjdk
  /Library/Java/JavaVirtualMachines/*/Contents/Home
  "$HOME/Library/Java/JavaVirtualMachines/*/Contents/Home"
)
JAVA_BIN=""
for cand in "${JDK_CANDIDATES[@]}"; do
  [ -z "$cand" ] && continue
  for d in $cand; do   # 不加引号以允许通配展开
    if [ -x "$d/bin/java" ]; then JAVA_BIN="$d/bin/java"; break 2; fi
  done
done
if [ -z "$JAVA_BIN" ] && command -v java >/dev/null 2>&1; then
  JAVA_BIN="java"   # 退而求其次（若是 stub，下面的版本检查会暴露）
fi
if [ -z "$JAVA_BIN" ]; then
  echo "❌ 未找到 JDK。请安装 JDK 21+（Capacitor 7 要求）（brew install openjdk@17）或设置 JAVA_HOME。" >&2
  exit 1
fi
JAVA_VER="$("$JAVA_BIN" -version 2>&1 | head -1 | sed -E 's/.*version "([0-9]+).*/\1/')"
if [ -z "$JAVA_VER" ] || [ "$JAVA_VER" -lt 17 ]; then
  echo "❌ 需要 JDK 21+（Capacitor 7 要求），当前：${JAVA_VER:-未知}（请确认 JAVA_HOME 指向 JDK 21+（Capacitor 7 要求））。" >&2
  exit 1
fi
export JAVA_HOME="$(cd "$(dirname "$JAVA_BIN")/.." && pwd)"
export PATH="$JAVA_HOME/bin:$PATH"
echo "✅ JDK：$JAVA_HOME (v$JAVA_VER)"

# ---------- 2. 检查 / 探测 Android SDK ----------
if [ ! -f local.properties ]; then
  CANDIDATE="$HOME/Library/Android/sdk"   # macOS 默认位置
  if [ -d "$CANDIDATE" ]; then
    printf 'sdk.dir=%s\n' "$CANDIDATE" > local.properties
    echo "✅ 已自动写入 local.properties：sdk.dir=$CANDIDATE"
  else
    echo "❌ 未找到 local.properties，且未探测到 Android SDK（$CANDIDATE 不存在）。" >&2
    echo "   请安装 Android SDK 后手动创建 local.properties，写入：" >&2
    echo "   sdk.dir=/你的/Android/Sdk" >&2
    exit 1
  fi
else
  echo "✅ 已存在 local.properties"
fi

# ---------- 3. 确保 gradlew 可执行 ----------
if [ ! -x gradlew ]; then
  if [ -f gradlew ]; then
    chmod +x gradlew
    echo "✅ 已为 gradlew 添加可执行权限"
  else
    echo "❌ 未找到 gradlew。请先在本机执行 'gradle wrapper' 生成，或用 Android Studio 打开一次。" >&2
    exit 1
  fi
fi

# ---------- 4. release 签名检查 ----------
if [ "$BUILD_TYPE" = "release" ]; then
  if [ ! -f keystore.properties ]; then
    echo "❌ release 构建需要 keystore.properties（android/ 目录下，不入库）。" >&2
    echo "   生成密钥：" >&2
    echo "   keytool -genkeypair -v -keystore release.keystore -alias moyue -keyalg RSA -keysize 2048 -validity 10000" >&2
    echo "   再创建 keystore.properties，填写：" >&2
    echo "   storeFile=release.keystore" >&2
    echo "   storePassword=你的密钥库密码" >&2
    echo "   keyAlias=moyue" >&2
    echo "   keyPassword=你的密钥密码" >&2
    exit 1
  fi
  echo "✅ 已检测到 keystore.properties"
fi

# ---------- 5. 构建 ----------
echo "🚀 开始构建：$TASK"
./gradlew "$TASK" --no-daemon

# ---------- 6. 定位产物 ----------
APK_DIR="app/build/outputs/apk/$BUILD_TYPE"
APK="$(ls -1 "$APK_DIR"/*.apk 2>/dev/null | head -1 || true)"
if [ -z "$APK" ]; then
  echo "❌ 未找到生成的 APK（预期目录：$APK_DIR）" >&2
  exit 1
fi
echo "✅ 构建完成：$APK"
echo "   体积：$(du -h "$APK" | cut -f1)"

# ---------- 7. 可选安装 ----------
if [ "$INSTALL" -eq 1 ]; then
  if ! command -v adb >/dev/null 2>&1; then
    echo "⚠️ 未找到 adb，跳过安装。请安装 Android SDK platform-tools 并加入 PATH。" >&2
  else
    echo "📲 安装到已连接设备…"
    adb install -r "$APK"
    echo "✅ 已安装：$APK"
  fi
fi

echo "🎉 完成。APK：$APK"
