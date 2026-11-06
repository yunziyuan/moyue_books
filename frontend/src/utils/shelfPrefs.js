// 书架使用习惯：账号级偏好，持久化在服务端 Setting 表，保证多端一致。
// 本地 localStorage 仅作为首屏占位与离线降级，最终以服务端返回值为准。
import { ref } from 'vue'
import axios from 'axios'
import { API_BASE } from '@/config'
import { getToken } from '@/api/auth'

export const SHELF_MODE_ALL = 'all'
export const SHELF_MODE_SELECTED = 'selected'

const CACHE_KEY = 'moyue_shelf_mode_cache'
const normalize = (mode) => (mode === SHELF_MODE_SELECTED ? SHELF_MODE_SELECTED : SHELF_MODE_ALL)

const readCache = () => {
  try {
    return normalize(localStorage.getItem(CACHE_KEY))
  } catch {
    return SHELF_MODE_ALL
  }
}
const writeCache = (mode) => {
  try {
    localStorage.setItem(CACHE_KEY, mode)
  } catch {
    /* 忽略存储异常 */
  }
}

// 全局单例：各页面共享同一个值，任一处修改立即生效
export const shelfMode = ref(readCache())

let inflight = null
let localWriteSeq = 0

// 从服务端拉取最新偏好；每次调用都取最新值，并发请求合并为一次，失败时用本地缓存降级
export function ensureShelfModeLoaded() {
  if (!getToken()) return Promise.resolve(shelfMode.value)
  if (inflight) return inflight
  const seq = localWriteSeq
  inflight = axios.get(`${API_BASE}/settings`)
    .then(({ data }) => {
      // 拉取期间若本地已保存过新值，以本地为准，避免旧响应覆盖
      if (seq !== localWriteSeq) return shelfMode.value
      shelfMode.value = normalize(data?.shelf_mode)
      writeCache(shelfMode.value)
      return shelfMode.value
    })
    .catch(() => shelfMode.value)
    .finally(() => { inflight = null })
  return inflight
}

// 保存到服务端：失败时回滚，避免本地展示与账号数据不一致
export async function saveShelfMode(mode) {
  const value = normalize(mode)
  const previous = shelfMode.value
  // 保存一开始就使并发中的 GET 响应失效，避免旧值覆盖乐观更新
  localWriteSeq += 1
  shelfMode.value = value
  try {
    await axios.put(`${API_BASE}/settings`, { shelf_mode: value })
    writeCache(value)
    return value
  } catch (e) {
    shelfMode.value = previous
    throw e
  }
}

// 其它标签页保存成功后的同步（storage 事件只在其它文档触发）
window.addEventListener('storage', (e) => {
  if (e.key === CACHE_KEY) shelfMode.value = normalize(e.newValue)
})

// 切回页面 / 设备唤醒时重新拉取，与其它端保持同步
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'visible') ensureShelfModeLoaded()
})
