<template>
  <div class="settings-section server-settings">
    <div class="section-title">
      <el-icon><Connection /></el-icon>
      <div>
        <h3>服务器地址</h3>
        <p>本应用连接你飞牛 fnOS 上的 moyue-books 服务，所有书籍与进度都保存在该服务器</p>
      </div>
    </div>

    <!-- 当前生效服务器 -->
    <div class="pref-group">
      <div class="pref-label">
        <span>当前服务器</span>
      </div>
      <el-input :model-value="current" readonly style="width: 100%">
        <template #prepend>地址</template>
      </el-input>
      <p class="setting-tip">连接状态由后端连通性决定；切换服务器会清除本机登录态</p>
    </div>

    <!-- 已保存的服务器列表（可切换 / 删除） -->
    <div class="pref-group" v-if="list.length">
      <div class="pref-label"><span>已保存的服务器</span></div>
      <div class="server-list">
        <div
          v-for="item in list"
          :key="item"
          class="server-item"
          :class="{ active: item === current }"
        >
          <div class="server-meta">
            <span class="server-url">{{ item }}</span>
            <el-tag v-if="item === current" size="small" type="success" effect="light">当前</el-tag>
          </div>
          <div class="server-actions">
            <el-button
              v-if="item !== current"
              size="small"
              type="primary"
              plain
              :loading="testing && switching === item"
              @click="onSwitch(item)"
            >切换</el-button>
            <el-button
              size="small"
              type="danger"
              plain
              :disabled="testing"
              @click="onRemove(item)"
            >删除</el-button>
          </div>
        </div>
      </div>
    </div>

    <!-- 新增 / 修改服务器地址 -->
    <div class="pref-group">
      <div class="pref-label"><span>添加服务器地址</span></div>
      <el-input
        v-model="draft"
        style="width: 100%"
        placeholder="例如 http://192.168.1.10:18080"
        :prefix-icon="Link"
        @keyup.enter="onSave"
      >
      </el-input>
      <div class="actions">
        <el-button type="primary" :loading="testing && !switching" @click="onSave">保存并连接</el-button>
        <el-button type="danger" plain :disabled="testing" @click="onClear">清除全部并退出</el-button>
      </div>
      <el-alert v-if="error" class="err" type="error" :closable="false" show-icon :title="error" />
      <el-alert v-if="ok" class="err" type="success" :closable="false" show-icon :title="ok" />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Connection, Link } from '@element-plus/icons-vue'
import {
  getServerUrl,
  getServerList,
  saveServer,
  setCurrentServer,
  removeServer,
  normalizeServerUrl,
  probeServer,
  clearServerUrl
} from '@/utils/serverConfig'
import { clearToken, setUser } from '@/api/auth'

const current = ref('')
const list = ref([])
const draft = ref('')
const testing = ref(false)
const switching = ref('') // 正在切换的地址（用于按钮 loading 区分）
const error = ref('')
const ok = ref('')

function refresh() {
  current.value = getServerUrl()
  list.value = getServerList()
}

onMounted(refresh)

// 新增地址：探测可达后保存并切为当前
async function onSave() {
  error.value = ''
  ok.value = ''
  const raw = normalizeServerUrl(draft.value)
  if (!raw) {
    error.value = '请填写服务器地址'
    return
  }
  testing.value = true
  switching.value = ''
  try {
    const info = await probeServer(raw)
    saveServer(raw)
    clearToken()
    setUser(null)
    ok.value = `已连接${info?.app_name ? '：' + info.app_name : ''}，正在重新加载…`
    setTimeout(() => location.reload(), 600)
  } catch (e) {
    error.value = e?.name === 'AbortError' ? '连接超时，请确认地址与网络可达' : `无法连接：${e?.message || e}`
  } finally {
    testing.value = false
    switching.value = ''
  }
}

// 在已保存地址间切换：重新探测可达后切换当前
async function onSwitch(url) {
  error.value = ''
  ok.value = ''
  if (url === current.value) return
  testing.value = true
  switching.value = url
  try {
    const info = await probeServer(url)
    setCurrentServer(url)
    clearToken()
    setUser(null)
    ok.value = `已切换到${info?.app_name ? '：' + info.app_name : ''}，正在重新加载…`
    setTimeout(() => location.reload(), 600)
  } catch (e) {
    error.value = e?.name === 'AbortError' ? '连接超时，无法切换到该服务器' : `无法连接：${e?.message || e}`
  } finally {
    testing.value = false
    switching.value = ''
  }
}

function onRemove(url) {
  removeServer(url)
  refresh()
}

function onClear() {
  clearServerUrl()
  clearToken()
  setUser(null)
  location.reload()
}
</script>

<style scoped>
.server-settings { text-align: left; }
.pref-group {
  margin-bottom: 22px;
}
.pref-label {
  margin-bottom: 10px;
  font-size: 13px;
  font-weight: 600;
  color: #475569;
}
.setting-tip,
.server-tip {
  margin: 8px 0 0;
  font-size: 12px;
  color: #94a3b8;
  line-height: 1.6;
}
.server-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.server-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.7);
  border: 1px solid rgba(148, 163, 184, 0.25);
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.server-item.active {
  border-color: var(--app-primary);
  box-shadow: 0 4px 14px color-mix(in srgb, var(--app-primary) 20%, transparent);
}
.server-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.server-url {
  font-size: 13px;
  color: #1e293b;
  word-break: break-all;
}
.server-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}
.actions {
  display: flex;
  gap: 12px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.err { margin-top: 14px; }
</style>
