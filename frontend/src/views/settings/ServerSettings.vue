<template>
  <div class="settings-section server-settings">
    <div class="section-title">
      <el-icon><Connection /></el-icon>
      <div>
        <h3>服务器地址</h3>
        <p>本应用连接你飞牛 fnOS 上的 moyue-books 服务，所有书籍与进度都保存在该服务器</p>
      </div>
    </div>

    <div class="pref-group">
      <div class="pref-label">
        <span>当前服务器</span>
      </div>
      <el-input :model-value="current" readonly style="width: 100%">
        <template #prepend>地址</template>
      </el-input>
      <p class="setting-tip">连接状态由后端连通性决定；若更换服务器，本机登录态将被清除</p>
    </div>

    <div class="pref-group">
      <div class="pref-label"><span>修改服务器地址</span></div>
      <el-input
        v-model="draft"
        style="width: 100%"
        placeholder="例如 http://192.168.1.10:18080"
        :prefix-icon="Link"
        @keyup.enter="onSave"
      >
      </el-input>
      <div class="actions">
        <el-button type="primary" :loading="testing" @click="onSave">保存并重新连接</el-button>
        <el-button type="danger" plain @click="onClear">清除并退出到设置页</el-button>
      </div>
      <el-alert v-if="error" class="err" type="error" :closable="false" show-icon :title="error" />
      <el-alert v-if="ok" class="err" type="success" :closable="false" show-icon :title="ok" />
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Connection, Link } from '@element-plus/icons-vue'
import { getServerUrl, setServerUrl, normalizeServerUrl, probeServer, clearServerUrl } from '@/utils/serverConfig'
import { clearToken, setUser } from '@/api/auth'

const current = ref('')
const draft = ref('')
const testing = ref(false)
const error = ref('')
const ok = ref('')

onMounted(() => {
  current.value = getServerUrl()
  draft.value = current.value
})

async function onSave() {
  error.value = ''
  ok.value = ''
  const raw = normalizeServerUrl(draft.value)
  if (!raw) {
    error.value = '请填写服务器地址'
    return
  }
  testing.value = true
  try {
    const info = await probeServer(raw)
    setServerUrl(raw)
    clearToken()
    setUser(null)
    ok.value = `已连接${info?.app_name ? '：' + info.app_name : ''}，正在重新加载…`
    setTimeout(() => location.reload(), 600)
  } catch (e) {
    error.value = e?.name === 'AbortError' ? '连接超时，请确认地址与网络可达' : `无法连接：${e?.message || e}`
  } finally {
    testing.value = false
  }
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
.actions {
  display: flex;
  gap: 12px;
  margin-top: 14px;
  flex-wrap: wrap;
}
.err { margin-top: 14px; }
</style>
