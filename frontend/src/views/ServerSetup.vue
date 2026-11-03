<template>
  <div
    class="setup-page"
    :style="keyboardOffset ? { paddingBottom: keyboardOffset + 'px' } : null"
  >
    <div class="bg-glow bg-glow-1"></div>
    <div class="bg-glow bg-glow-2"></div>

    <div class="setup-card cardIn">
      <div class="logo">
        <el-icon :size="34"><Reading /></el-icon>
      </div>
      <h1 class="title">连接你的墨阅书屋</h1>
      <p class="subtitle">
        本应用通过局域网 / 公网连接你<strong>飞牛 fnOS</strong> 上部署的 moyue-books 服务。<br />
        请填写该服务的访问地址，所有书籍与阅读进度都保存在你的服务器上。
      </p>

      <el-form @submit.prevent="onConnect" label-position="top">
        <el-form-item>
          <el-input
            v-model="serverInput"
            size="large"
            clearable
            placeholder="例如 http://192.168.1.10:18080"
            :prefix-icon="Link"
            @keyup.enter="onConnect"
          >
          </el-input>
        </el-form-item>

        <div v-if="savedList.length" class="saved-list">
          <span class="saved-label">已保存的服务器</span>
          <div class="saved-chips">
            <button
              v-for="item in savedList"
              :key="item"
              type="button"
              class="saved-chip"
              @click="serverInput = item; onConnect()"
            >
              {{ item }}
            </button>
          </div>
        </div>

        <div class="hint">
          可含或不含 <code>/api</code> 后缀；支持 <code>http</code> / <code>https</code>。
          默认端口为 <code>18080</code>（飞牛 fpk 安装）或你的反向代理地址。
        </div>

        <el-button
          type="primary"
          size="large"
          class="connect-btn"
          :loading="testing"
          @click="onConnect"
        >
          {{ savedServer ? '保存并重新连接' : '连接并进入' }}
        </el-button>

        <el-alert
          v-if="error"
          class="err"
          type="error"
          :closable="false"
          show-icon
          :title="error"
        />
        <el-alert
          v-if="ok"
          class="err"
          type="success"
          :closable="false"
          show-icon
          :title="ok"
        />
      </el-form>

      <div class="foot">墨阅图书 · 个人图书管理与阅读</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Link, Reading } from '@element-plus/icons-vue'
import { getServerUrl, getServerList, saveServer, normalizeServerUrl, probeServer } from '@/utils/serverConfig'
import { clearToken, setUser } from '@/api/auth'

const serverInput = ref('')
const savedServer = ref('')
const savedList = ref([])
const testing = ref(false)
const error = ref('')
const ok = ref('')

// 移动端键盘高度：用 visualViewport 计算，撑开底部空间让输入框/按钮可滚到键盘上方
const keyboardOffset = ref(0)
let viewportHandler = null

const updateKeyboard = () => {
  if (!window.visualViewport) return
  const vv = window.visualViewport
  const keyboard = window.innerHeight - vv.height - vv.offsetTop
  keyboardOffset.value = keyboard > 0 ? keyboard : 0
}

onMounted(() => {
  savedServer.value = getServerUrl()
  savedList.value = getServerList().filter((u) => u !== savedServer.value)
  serverInput.value = savedServer.value
  if (window.visualViewport) {
    viewportHandler = () => requestAnimationFrame(updateKeyboard)
    window.visualViewport.addEventListener('resize', viewportHandler)
    window.visualViewport.addEventListener('scroll', viewportHandler)
    updateKeyboard()
  }
})

onUnmounted(() => {
  if (window.visualViewport && viewportHandler) {
    window.visualViewport.removeEventListener('resize', viewportHandler)
    window.visualViewport.removeEventListener('scroll', viewportHandler)
  }
})

async function onConnect() {
  error.value = ''
  ok.value = ''
  const raw = normalizeServerUrl(serverInput.value)
  if (!raw) {
    error.value = '请填写服务器地址'
    return
  }
  testing.value = true
  try {
    const info = await probeServer(raw)
    // 连通成功：保存地址，并清除旧 token（不同服务器 = 不同账号）
    saveServer(raw)
    clearToken()
    setUser(null)
    ok.value = `已连接${info?.app_name ? '：' + info.app_name : ''}，正在进入…`
    // 必须真正整页重载：仅修改 hash（如 location.replace('#/login')）不会重载页面，
    // config.js 中一次性算好的 API_BASE 不会按新地址重建，登录请求会打到本地 WebView 而非后端。
    // 先落地登录页 hash，再 reload 让 API_BASE 以新服务器地址重新计算。
    setTimeout(() => {
      if (!window.location.hash.startsWith('#/login')) {
        window.location.hash = '#/login'
      }
      window.location.reload()
    }, 600)
  } catch (e) {
    if (e?.name === 'AbortError') {
      error.value = '连接超时，请确认地址与网络可达（同一局域网）'
    } else {
      error.value = `无法连接：${e?.message || e}`
    }
  } finally {
    testing.value = false
  }
}
</script>

<style scoped>
.setup-page {
  position: relative;
  width: 100%;
  height: 100vh;
  height: 100dvh;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 24px;
  padding-top: max(24px, env(safe-area-inset-top));
  box-sizing: border-box;
  overflow: hidden; /* 固定一屏，禁止上下滑动 */
}
.bg-glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  pointer-events: none;
  z-index: 0;
}
.bg-glow-1 {
  width: 420px; height: 420px;
  background: radial-gradient(circle, color-mix(in srgb, var(--app-primary) 18%, transparent), transparent 70%);
  top: -120px; left: -120px;
}
.bg-glow-2 {
  width: 380px; height: 380px;
  background: radial-gradient(circle, rgba(34, 211, 238, 0.14), transparent 70%);
  bottom: -100px; right: -100px;
}
.setup-card {
  position: relative;
  z-index: 1;
  margin: auto; /* 有空间时垂直居中；键盘弹出时随底部留白整体上移 */
  width: 100%;
  max-width: 440px;
  box-sizing: border-box; /* 关键：padding 计入宽度，否则 100%+66px 溢出屏幕 */
  padding: 36px 32px 28px;
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.82);
  border: 1px solid rgba(255, 255, 255, 0.75);
  box-shadow: 0 20px 60px rgba(31, 38, 135, 0.12);
  backdrop-filter: blur(12px);
  text-align: center;
}
.logo {
  width: 64px; height: 64px;
  margin: 0 auto 16px;
  border-radius: 18px;
  display: flex; align-items: center; justify-content: center;
  color: #fff;
  background: linear-gradient(135deg, var(--app-primary), color-mix(in srgb, var(--app-primary) 55%, #8b5cf6));
  box-shadow: 0 10px 24px color-mix(in srgb, var(--app-primary) 35%, transparent);
}
.title {
  margin: 0 0 10px;
  font-size: 22px;
  font-weight: 800;
  color: #0f172a;
}
.subtitle {
  margin: 0 0 24px;
  font-size: 13px;
  line-height: 1.7;
  color: #64748b;
}
.hint {
  text-align: left;
  font-size: 12px;
  color: #94a3b8;
  margin: -6px 2px 16px;
  line-height: 1.6;
}
.hint code {
  background: rgba(100, 116, 139, 0.12);
  padding: 1px 6px;
  border-radius: 6px;
  font-size: 11px;
}
.connect-btn {
  width: 100%;
  font-weight: 700;
  letter-spacing: 1px;
}
.saved-list {
  margin-bottom: 18px;
  text-align: left;
}
.saved-label {
  display: block;
  font-size: 12px;
  color: #94a3b8;
  margin-bottom: 8px;
}
.saved-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.saved-chip {
  max-width: 100%;
  padding: 7px 12px;
  border-radius: 999px;
  border: 1px solid rgba(99, 102, 241, 0.3);
  background: rgba(99, 102, 241, 0.08);
  color: #4f46e5;
  font-size: 12px;
  cursor: pointer;
  transition: background 0.2s ease, transform 0.15s ease;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.saved-chip:active {
  transform: scale(0.96);
  background: rgba(99, 102, 241, 0.16);
}
.err {
  margin-top: 16px;
  text-align: left;
}
.foot {
  margin-top: 22px;
  font-size: 11px;
  color: #cbd5e1;
}

/* 矮屏（或键盘弹出后剩余空间小）：收紧间距，保证整卡不超出固定一屏 */
@media (max-height: 760px) {
  .setup-page { padding-top: max(12px, env(safe-area-inset-top)); padding-bottom: 12px; }
  .setup-card { padding: 22px 22px 18px; border-radius: 18px; }
  .logo { width: 52px; height: 52px; margin-bottom: 10px; }
  .title { font-size: 19px; margin-bottom: 6px; }
  .subtitle { margin-bottom: 14px; font-size: 12px; }
  .hint { margin-bottom: 12px; }
  .saved-list { margin-bottom: 12px; }
  .foot { margin-top: 14px; }
}
</style>
