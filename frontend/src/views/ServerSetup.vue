<template>
  <div class="setup-page">
    <div class="bg-glow bg-glow-1"></div>
    <div class="bg-glow bg-glow-2"></div>

    <div class="setup-card cardIn">
      <div class="logo">
        <el-icon :size="34"><Reading /></el-icon>
      </div>
      <h1 class="title">连接你的墨阅书架</h1>
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
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Link, Reading } from '@element-plus/icons-vue'
import { getServerUrl, setServerUrl, normalizeServerUrl, probeServer } from '@/utils/serverConfig'
import { clearToken, setUser } from '@/api/auth'

const serverInput = ref('')
const savedServer = ref('')
const testing = ref(false)
const error = ref('')
const ok = ref('')

onMounted(() => {
  savedServer.value = getServerUrl()
  serverInput.value = savedServer.value
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
    setServerUrl(raw)
    clearToken()
    setUser(null)
    ok.value = `已连接${info?.app_name ? '：' + info.app_name : ''}，正在进入…`
    setTimeout(() => location.reload(), 600)
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
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  box-sizing: border-box;
  overflow: hidden;
}
.bg-glow {
  position: fixed;
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
  width: 100%;
  max-width: 440px;
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
.err {
  margin-top: 16px;
  text-align: left;
}
.foot {
  margin-top: 22px;
  font-size: 11px;
  color: #cbd5e1;
}
</style>
