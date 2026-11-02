// ===== 服务器地址配置（移动端 / 远程客户端）=====
// App 作为远程客户端连接用户飞牛上的 moyue-books 实例，
// 服务器地址由用户在首次启动时填写，持久化在 localStorage，运行时动态读取。
import { nativeFetch } from '@/utils/nativeHttp'

const SERVER_URL_KEY = 'mb_server_url'

// 读取已保存的服务器地址（不含 /api 后缀，形如 http://192.168.1.10:18080）
export function getServerUrl() {
  try {
    return localStorage.getItem(SERVER_URL_KEY) || ''
  } catch {
    return ''
  }
}

// 规范化用户输入：去空格、补全协议、去尾部斜杠
export function normalizeServerUrl(raw) {
  let u = (raw || '').trim().replace(/\/+$/, '')
  if (!u) return ''
  if (!/^https?:\/\//i.test(u)) u = 'http://' + u
  return u
}

// 保存服务器地址（自动规范化）
export function setServerUrl(raw) {
  const u = normalizeServerUrl(raw)
  try {
    if (u) localStorage.setItem(SERVER_URL_KEY, u)
    else localStorage.removeItem(SERVER_URL_KEY)
  } catch {}
  return u
}

export function clearServerUrl() {
  try {
    localStorage.removeItem(SERVER_URL_KEY)
  } catch {}
}

// 探测某地址是否可达且为 moyue-books 服务：
// 调用公开接口 /api/config/public，成功返回 true，否则抛错（含状态码/网络错误）
export async function probeServer(baseUrl, timeoutMs = 8000) {
  const u = normalizeServerUrl(baseUrl)
  if (!u) throw new Error('地址为空')
  const apiBase = u.endsWith('/api') ? u : u + '/api'
  try {
    const res = await nativeFetch(`${apiBase}/config/public`, {
      method: 'GET',
      timeout: timeoutMs
    })
    if (!res.ok) throw new Error(`服务返回 ${res.status}`)
    return await res.json()
  } catch (e) {
    if (e instanceof TypeError) {
      throw new Error('网络不可达：请确认手机与飞牛在同一局域网，且飞牛防火墙/端口已放行该服务')
    }
    throw e
  }
}
