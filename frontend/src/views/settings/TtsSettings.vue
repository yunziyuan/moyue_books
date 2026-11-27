<template>
  <div class="settings-section tts-settings">
    <div class="section-title">
      <el-icon><Service /></el-icon>
      <div>
        <h3>语音合成服务</h3>
        <p>配置云端朗读引擎密钥，用于阅读朗读与有声书生成</p>
      </div>
    </div>

    <el-alert class="tts-notice" type="info" :closable="false" show-icon>
      密钥仅保存在当前账号的服务端配置中，不会下发到前端页面。留空表示不修改；点击「清除」后保存可删除已保存的密钥。讯飞与 DeepSeek 为按量计费服务。
    </el-alert>

    <div class="pref-group">
      <div class="pref-label"><span>讯飞在线 TTS</span><span class="group-hint">WebSocket · 返回 MP3</span></div>
      <el-form label-position="top" :model="form" @submit.prevent>
        <el-form-item v-for="f in xfFields" :key="f.key" :label="f.label">
          <el-input
            v-model="form[f.key]"
            :type="f.secret ? 'password' : 'text'"
            :show-password="f.secret"
            :placeholder="fieldPlaceholder(f)"
            autocomplete="off"
            @input="onFieldInput(f.key)"
          />
          <div v-if="f.secret && isConfigured(f.key) && !touched[f.key]" class="field-clear">
            <el-button link type="primary" @click="clearField(f.key)">清除已保存的密钥</el-button>
          </div>
        </el-form-item>
      </el-form>
      <div class="tts-actions">
        <el-button :loading="testing === 'xunfei'" @click="saveAndTest('xunfei')">保存并试合成</el-button>
        <span class="action-hint">使用「晓燕」合成一句短文本，成功即表示密钥可用</span>
      </div>
    </div>

    <div class="pref-group">
      <div class="pref-label"><span>DeepSeek TTS</span><span class="group-hint">HTTP · 返回 WAV</span></div>
      <el-form label-position="top" :model="form" @submit.prevent>
        <el-form-item v-for="f in dsFields" :key="f.key" :label="f.label">
          <el-input
            v-model="form[f.key]"
            :type="f.secret ? 'password' : 'text'"
            :show-password="f.secret"
            :placeholder="fieldPlaceholder(f)"
            autocomplete="off"
            @input="onFieldInput(f.key)"
          />
          <div v-if="f.secret && isConfigured(f.key) && !touched[f.key]" class="field-clear">
            <el-button link type="primary" @click="clearField(f.key)">清除已保存的密钥</el-button>
          </div>
        </el-form-item>
      </el-form>
      <div class="tts-actions">
        <el-button :loading="testing === 'deepseek'" @click="saveAndTest('deepseek')">保存并试合成</el-button>
        <span class="action-hint">API 地址填域名或带 /v1 均可，最终请求 /v1/tools/tts；该引擎不支持语速调节</span>
      </div>
    </div>

    <div class="save-row">
      <el-button type="primary" :loading="saving" :disabled="!hasChanges" @click="confirmSave">保存设置</el-button>
      <span v-if="hasChanges" class="dirty-hint">有未保存的修改，点击保存后确认</span>
      <span v-else class="dirty-hint">无未保存的修改</span>
    </div>
    <el-alert
      v-if="saveFeedback.text"
      class="save-alert"
      :type="saveFeedback.type || 'success'"
      :closable="false"
      show-icon
    >{{ saveFeedback.text }}</el-alert>

    <el-dialog v-model="confirmVisible" title="确认保存语音合成设置" width="440px" append-to-body>
      <p class="confirm-intro">以下修改将保存到当前账号，并立即对朗读和有声书生效：</p>
      <ul class="change-list">
        <li v-for="c in pendingChanges" :key="c.key">
          {{ c.label }}：
          <span :class="c.clear ? 'change-clear' : 'change-save'">{{ c.clear ? '清除已保存的值' : (c.secret ? '更新密钥' : '更新配置') }}</span>
        </li>
      </ul>
      <p class="confirm-note">密钥不会明文回显，也不会下发到前端页面。</p>
      <template #footer>
        <el-button @click="confirmVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="doSave">确认保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Service } from '@element-plus/icons-vue'
import axios from 'axios'
import { API_BASE } from '@/config'
import { bookApi } from '@/api/book'

const xfFields = [
  { key: 'xf_app_id', label: 'AppID', secret: false, placeholder: '讯飞控制台 APPID' },
  { key: 'xf_api_key', label: 'APIKey', secret: true, placeholder: '用于 HMAC 签名鉴权' },
  { key: 'xf_api_secret', label: 'APISecret', secret: true, placeholder: '用于 HMAC 签名鉴权' },
  { key: 'xf_api_password', label: 'APIPassword（可选）', secret: true, placeholder: '与 APIKey/APISecret 二选一' }
]
const dsFields = [
  { key: 'deepseek_api_key', label: 'API Key', secret: true, placeholder: 'sk-xxxxxxxx' },
  { key: 'deepseek_base_url', label: 'API 地址（可选）', secret: false, placeholder: '默认 https://api.deepseek.com（可省略 /v1）' }
]
const allFields = [...xfFields, ...dsFields]

const form = reactive(Object.fromEntries(allFields.map((f) => [f.key, ''])))
const touched = reactive(Object.fromEntries(allFields.map((f) => [f.key, false])))
const configured = ref({})
const saving = ref(false)
const testing = ref('')
const confirmVisible = ref(false)
const saveFeedback = ref({ type: '', text: '' })

const legacyHint = '后端仍是旧版本：请重新编译并重启后端（本地执行 ./build.sh 或 ./start.sh；Docker 执行 docker compose up -d --build）。'
const isLegacyError = (msg = '') => /shelf_mode 不能为空|请设置 XF_APP_ID/.test(msg)
const setFeedback = (type, text) => { saveFeedback.value = { type, text } }

const hasChanges = computed(() => allFields.some((f) => touched[f.key]))
const pendingChanges = computed(() => allFields.filter((f) => touched[f.key]).map((f) => ({
  ...f,
  clear: !(form[f.key] || '').trim()
})))
const onFieldInput = (key) => {
  touched[key] = true
  saveFeedback.value = { type: '', text: '' }
}
const isConfigured = (key) => !!configured.value[key + '_set']
const fieldPlaceholder = (f) => {
  if (touched[f.key]) return '留空并保存表示清除'
  if (f.secret && isConfigured(f.key)) return '已配置，留空不修改'
  return f.placeholder
}
const clearField = (key) => {
  form[key] = ''
  onFieldInput(key)
}

const load = async () => {
  try {
    const { data } = await axios.get(`${API_BASE}/settings`)
    const creds = data?.tts_credentials || {}
    configured.value = creds
    for (const f of allFields) {
      if (!f.secret) form[f.key] = creds[f.key] || ''
      touched[f.key] = false
    }
  } catch (e) {
    ElMessage.error('加载语音合成设置失败：' + (e?.response?.data?.error || e.message))
  }
}

const save = async (silent = false) => {
  const payload = {}
  for (const f of allFields) {
    if (touched[f.key]) payload[f.key] = (form[f.key] || '').trim()
  }
  if (!Object.keys(payload).length) {
    if (!silent) {
      setFeedback('info', '没有需要保存的修改')
      ElMessage.info('没有需要保存的修改')
    }
    return false
  }
  saving.value = true
  try {
    const { data } = await axios.put(`${API_BASE}/settings`, payload)
    configured.value = data?.tts_credentials || configured.value
    for (const f of allFields) if (touched[f.key]) touched[f.key] = false
    setFeedback('success', '设置已保存，朗读与有声书生成会立即使用新配置')
    if (!silent) ElMessage.success('语音合成设置已保存')
    return true
  } catch (e) {
    const detail = e?.response?.data?.error || e.message || '未知错误'
    const message = '保存失败：' + detail + (isLegacyError(detail) ? ' ' + legacyHint : '')
    setFeedback('error', message)
    ElMessage.error(message)
    return false
  } finally {
    saving.value = false
  }
}

const confirmSave = () => {
  if (!hasChanges.value) {
    setFeedback('info', '没有需要保存的修改')
    ElMessage.info('没有需要保存的修改')
    return
  }
  confirmVisible.value = true
}

const doSave = async () => {
  confirmVisible.value = false
  await save()
}

const saveAndTest = async (engine) => {
  if (!await save(true)) {
    // 未修改配置也允许直接用已保存的密钥试合成
  }
  testing.value = engine
  try {
    const voice = engine === 'xunfei' ? 'xiaoyan' : 'stella'
    const resp = await bookApi.synthesizeTts({
      text: '语音合成测试。',
      engine,
      voice,
      speed_factor: 1,
      media_type: engine === 'deepseek' ? 'wav' : 'mp3'
    })
    if (!resp.data || resp.data.size < 100) throw new Error('返回结果为空')
    const okText = `${engine === 'xunfei' ? '讯飞' : 'DeepSeek'} 试合成成功`
    setFeedback('success', okText)
    ElMessage.success(okText)
  } catch (e) {
    let detail = e?.message || '未知错误'
    const body = e?.response?.data
    if (body instanceof Blob) {
      try {
        detail = JSON.parse(await body.text()).error || detail
      } catch (_) { /* 非 JSON 错误体保持原样 */ }
    } else if (body?.error) {
      detail = body.error
    }
    const message = '试合成失败：' + detail + (isLegacyError(detail) ? ' ' + legacyHint : '')
    setFeedback('error', message)
    ElMessage.error(message)
  } finally {
    testing.value = ''
  }
}

onMounted(load)
</script>

<style scoped>
.section-title {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 20px;
}
.section-title > .el-icon {
  font-size: 26px;
  color: var(--el-color-primary);
  background: color-mix(in srgb, var(--app-primary) 12%, transparent);
  padding: 10px;
  border-radius: 12px;
}
.section-title h3 {
  margin: 0;
  font-size: 18px;
}
.section-title p {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.tts-notice {
  margin-bottom: 24px;
}
.pref-group {
  margin-bottom: 28px;
  padding: 18px 20px 16px;
  border-radius: 16px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
}
.pref-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
  font-size: 14px;
  font-weight: 600;
}
.group-hint {
  font-size: 12px;
  font-weight: 400;
  color: var(--el-text-color-secondary);
}
.tts-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-top: 4px;
}
.action-hint,
.dirty-hint {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.field-clear {
  margin-top: 2px;
  text-align: right;
}
.field-clear .el-button {
  height: auto;
  padding: 0;
  font-size: 12px;
}
.save-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding-top: 4px;
}
.save-alert {
  margin-top: 14px;
}
.confirm-intro,
.confirm-note {
  margin: 0 0 12px;
  font-size: 13px;
  color: var(--el-text-color-regular);
}
.confirm-note {
  margin: 12px 0 0;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.change-list {
  margin: 0;
  padding: 10px 16px;
  border-radius: 10px;
  background: var(--el-fill-color-lighter);
  list-style: none;
  font-size: 13px;
  line-height: 2;
}
.change-save {
  color: var(--el-color-success);
  font-weight: 600;
}
.change-clear {
  color: var(--el-color-danger);
  font-weight: 600;
}
</style>
