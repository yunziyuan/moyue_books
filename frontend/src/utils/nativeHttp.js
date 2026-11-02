import { Capacitor, registerPlugin } from '@capacitor/core'

const isNative = Capacitor.isNativePlatform()
const HttpPlugin = isNative ? registerPlugin('HttpPlugin') : null

// 解决 Android WebView 对「从 http://localhost 到私有 IP(192.168.x.x)」请求的 PNA 拦截：
// 原生层(Java HttpURLConnection)发请求，完全绕开 WebView 的 CORS/PNA 限制。
// web/桌面环境同源无此限制，仍走原生 fetch/axios。

function base64ToBytes(b64) {
  const bin = atob(b64)
  const len = bin.length
  const bytes = new Uint8Array(len)
  for (let i = 0; i < len; i++) bytes[i] = bin.charCodeAt(i)
  return bytes
}

function base64ToBlob(b64, type = 'application/octet-stream') {
  return new Blob([base64ToBytes(b64)], { type })
}

// fetch 兼容封装：返回值提供 .ok / .status / .json() / .text() / .blob() / .arrayBuffer()
export async function nativeFetch(url, init = {}) {
  if (!isNative) return fetch(url, init)

  const method = (init.method || 'GET').toUpperCase()
  const headers = init.headers || {}
  const data = init.body ?? null
  const res = await HttpPlugin.request({
    method,
    url,
    headers,
    data,
    responseType: 'blob',
    timeout: init.timeout || 0
  })
  const blob = typeof res.data === 'string' ? base64ToBlob(res.data) : new Blob()
  return {
    ok: res.status >= 200 && res.status < 300,
    status: res.status,
    statusText: '',
    headers: new Headers(res.headers || {}),
    url,
    json: async () => JSON.parse(await blob.text()),
    text: async () => blob.text(),
    blob: async () => blob,
    arrayBuffer: async () => blob.arrayBuffer(),
    body: null
  }
}

// 供 axios 使用的原生适配器：所有 /api 请求在原生端走 HttpPlugin
export function capacitorAxiosAdapter(config) {
  const method = (config.method || 'get').toUpperCase()
  let url = (config.baseURL || '') + (config.url || '')
  if (config.params) {
    const q = new URLSearchParams()
    Object.entries(config.params).forEach(([k, v]) => {
      if (v != null) q.append(k, String(v))
    })
    const s = q.toString()
    if (s) url += (url.includes('?') ? '&' : '?') + s
  }
  const headers = {}
  const h = config.headers
  if (h) {
    if (typeof h.forEach === 'function') {
      h.forEach((v, k) => { if (v != null) headers[k] = String(v) })
    } else {
      Object.entries(h).forEach(([k, v]) => { if (v != null) headers[k] = String(v) })
    }
  }
  let data = config.data
  if (
    data != null &&
    typeof data === 'object' &&
    !(data instanceof FormData) &&
    !(data instanceof Blob) &&
    !(data instanceof ArrayBuffer) &&
    !(data instanceof URLSearchParams) &&
    typeof data !== 'string'
  ) {
    try { data = JSON.stringify(data) } catch { /* ignore */ }
  }
  if (data != null && !headers['Content-Type'] && !headers['content-type']) {
    headers['Content-Type'] = 'application/json'
  }
  const responseType = config.responseType || 'json'
  return HttpPlugin.request({
    method,
    url,
    headers,
    data,
    responseType,
    timeout: config.timeout || 0
  })
    .then((res) => {
      let d = res.data
      if (responseType === 'json' && typeof d === 'string') {
        try { d = JSON.parse(d) } catch { /* keep string */ }
      }
      if ((responseType === 'blob' || responseType === 'arraybuffer') && typeof d === 'string') {
        d = base64ToBytes(d).buffer
      }
      const response = {
        data: d,
        status: res.status,
        statusText: '',
        headers: res.headers || {},
        config,
        request: {}
      }
      if (res.status >= 200 && res.status < 300) return response
      const error = new Error(`Request failed with status code ${res.status}`)
      error.isAxiosError = true
      error.config = config
      error.response = response
      return Promise.reject(error)
    })
    .catch((err) => {
      if (err.isAxiosError) throw err
      const error = new Error(err?.message || 'Network Error')
      error.isAxiosError = true
      error.config = config
      throw error
    })
}
