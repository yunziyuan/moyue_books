// ===== 服务器地址配置（移动端 / 远程客户端）=====
// App 作为远程客户端连接用户飞牛上的 moyue-books 实例，
// 服务器地址由用户在首次启动时填写，持久化在 localStorage，运行时动态读取。
// 支持保存多个地址并在它们之间切换：
//   - mb_server_url  : 当前生效的服务器地址
//   - mb_servers     : 已保存地址列表（JSON 数组，最新/常用在前）
import { nativeFetch } from '@/utils/nativeHttp'

const SERVER_URL_KEY = 'mb_server_url'
const SERVER_LIST_KEY = 'mb_servers'

// 读取当前生效的服务器地址（不含 /api 后缀，形如 http://192.168.1.10:18080）
export function getServerUrl() {
  try {
    return localStorage.getItem(SERVER_URL_KEY) || ''
  } catch {
    return ''
  }
}

// 读取已保存的全部服务器地址（去重，且保证当前地址在列，最新在前）
export function getServerList() {
  let list = []
  try {
    const raw = localStorage.getItem(SERVER_LIST_KEY)
    if (raw) list = JSON.parse(raw)
  } catch {}
  if (!Array.isArray(list)) list = []
  const current = getServerUrl()
  if (current && !list.includes(current)) list.push(current)
  const seen = new Set()
  const result = []
  for (const item of list) {
    const u = normalizeServerUrl(item)
    if (u && !seen.has(u)) {
      seen.add(u)
      result.push(u)
    }
  }
  return result
}

// 规范化用户输入：去空格、补全协议、去尾部斜杠
export function normalizeServerUrl(raw) {
  let u = (raw || '').trim().replace(/\/+$/, '')
  if (!u) return ''
  if (!/^https?:\/\//i.test(u)) u = 'http://' + u
  return u
}

// 保存地址并设为当前生效（去重写入列表，置顶）；返回规范化后的地址
export function saveServer(raw) {
  const u = normalizeServerUrl(raw)
  if (!u) return ''
  try {
    const list = getServerList().filter((x) => x !== u)
    list.unshift(u)
    localStorage.setItem(SERVER_LIST_KEY, JSON.stringify(list))
    localStorage.setItem(SERVER_URL_KEY, u)
  } catch {}
  return u
}

// 在已保存地址间切换当前生效地址（不新增）；返回规范化后的地址
export function setCurrentServer(raw) {
  const u = normalizeServerUrl(raw)
  if (!u) return ''
  try {
    const list = getServerList()
    if (!list.includes(u)) list.unshift(u)
    localStorage.setItem(SERVER_LIST_KEY, JSON.stringify(list))
    localStorage.setItem(SERVER_URL_KEY, u)
  } catch {}
  return u
}

// 删除一个已保存地址；若删除的正是当前地址，则自动回退到列表首项（可能为空）
export function removeServer(raw) {
  const u = normalizeServerUrl(raw)
  if (!u) return
  try {
    const list = getServerList().filter((x) => x !== u)
    localStorage.setItem(SERVER_LIST_KEY, JSON.stringify(list))
    if (getServerUrl() === u) {
      localStorage.setItem(SERVER_URL_KEY, list[0] || '')
    }
  } catch {}
}

export function clearServerUrl() {
  try {
    localStorage.removeItem(SERVER_URL_KEY)
    localStorage.removeItem(SERVER_LIST_KEY)
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
